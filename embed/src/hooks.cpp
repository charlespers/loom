// embed/src/hooks.cpp — loom_hook_* strong symbols.
//
// Consumers that include <loom/loom_hooks.h> (e.g., Bedrock from its
// libBedrock/shared/include/bedrock/loom_hooks.h header) declare these
// names as weak. When libloom is linked, the strong definitions below
// override the weak declarations and forward each hook into the canonical
// loom_* C ABI in loom.cpp.
//
// Keeping the forwarders in a separate file lets a consumer that wants
// only the hook surface link `liblloom_hooks` if we ever split it out;
// for M2-partial it's all in liblloom.a together.
#include "loom/loom_c.h"

#include <cstring>

extern "C" {

void loom_hook_span_begin(const char* name) {
  if (!name) return;
  loom_span_begin(name, std::strlen(name), nullptr, 0);
}

void loom_hook_span_end(const char* /*name*/) {
  // M2-partial: span_end is a no-op; M2-full will pair with span_begin and
  // emit a duration. The name parameter is reserved for that pairing.
}

void loom_hook_metric_f64(const char* name, double v) {
  if (!name) return;
  loom_metric_f64(name, std::strlen(name), v);
}

void loom_hook_error(const char* name, const char* message) {
  if (!name) return;
  loom_error(name, std::strlen(name),
             message ? message : "", message ? std::strlen(message) : 0,
             /*severity=*/1,  // Severity::Error
             nullptr, 0);
}

void loom_hook_audit(const char* name, const char* /*attrs_json*/, int sync) {
  if (!name) return;
  // attrs_json is parsed and threaded into audit attributes in M2-full.
  loom_audit(name, std::strlen(name), nullptr, 0, sync);
}

void loom_hook_lifecycle(const char* marker) {
  if (!marker) return;
  loom_lifecycle(marker, std::strlen(marker), nullptr, 0);
}

}  // extern "C"
