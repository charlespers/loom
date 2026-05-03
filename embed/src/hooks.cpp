// embed/src/hooks.cpp — loom_hook_* strong symbols.
//
// Consumers that include <loom/loom_hooks.h> (notably Bedrock via its
// libBedrock/shared/include/bedrock/loom_hooks.h) declare these names as
// weak. When libloom is linked, the strong definitions below override the
// weak declarations and forward each hook into the canonical loom_* C
// ABI in loom.cpp.
//
// The hooks API is intentionally simpler than the full C++ surface — it
// passes only `name` for span begin/end (no handle). To pair them into a
// real duration-bearing span, this file maintains a per-thread stack of
// handles returned from loom_span_begin. loom_hook_span_end pops the top
// and calls loom_span_end. Mismatched begin/end (e.g. early return paths
// that skip end) drop their entry on next end of any kind, leaking at
// most a stack slot per skip, never an active_spans table entry.
#include "loom/loom_c.h"

#include <cstring>
#include <vector>

namespace {

// Per-thread span handle stack. When the hook header's clients nest
// LOOM_HOOK(loom_hook_span_begin, ...) calls, parents resolve naturally
// because the embed lib's own thread_span_stack handles parenting; we
// only need the handle here so loom_hook_span_end can pair.
std::vector<uint64_t>& hook_handle_stack() {
  thread_local std::vector<uint64_t> stack;
  return stack;
}

}  // namespace

extern "C" {

void loom_hook_span_begin(const char* name) {
  if (!name) return;
  uint64_t h = loom_span_begin(name, std::strlen(name), nullptr, 0);
  hook_handle_stack().push_back(h);  // 0 (inactive) is fine — pop will skip it
}

void loom_hook_span_end(const char* /*name*/) {
  auto& s = hook_handle_stack();
  if (s.empty()) return;
  uint64_t h = s.back();
  s.pop_back();
  if (h != 0) loom_span_end(h);
}

void loom_hook_span_emit(const char* name, uint64_t dur_ns) {
  if (!name) return;
  loom_span_emit(name, std::strlen(name), dur_ns, nullptr, 0);
}

void loom_hook_device_span_emit(const char* name, uint64_t dur_ns,
                                const char* backend, const char* queue_id) {
  if (!name) return;
  loom_device_span_emit(
      name,     std::strlen(name),
      dur_ns,
      backend  ? backend  : "", backend  ? std::strlen(backend)  : 0,
      queue_id ? queue_id : "", queue_id ? std::strlen(queue_id) : 0,
      nullptr, 0);
}

void loom_hook_metric_f64(const char* name, double v) {
  if (!name) return;
  loom_metric_f64(name, std::strlen(name), v, nullptr, 0);
}

void loom_hook_error(const char* name, const char* message) {
  if (!name) return;
  loom_error(name, std::strlen(name),
             message ? message : "", message ? std::strlen(message) : 0,
             /*severity=*/1,
             nullptr, 0);
}

void loom_hook_audit(const char* name, const char* /*attrs_json*/, int sync) {
  if (!name) return;
  // attrs_json is parsed in M2-full; for now we record the audit with empty
  // attributes (the chain still progresses, the categorical record stands).
  loom_audit(name, std::strlen(name), nullptr, 0, sync);
}

void loom_hook_lifecycle(const char* marker) {
  if (!marker) return;
  loom_lifecycle(marker, std::strlen(marker), nullptr, 0);
}

}  // extern "C"
