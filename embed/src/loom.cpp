// embed/src/loom.cpp — M2-partial implementation.
//
// Each event call writes one NDJSON line to events.jsonl. The ring buffer,
// the daemon, the audit hash chain, the redaction pipeline, and the proper
// attribute encoder all arrive in M2-full; until then this direct-write
// path is intentionally simple at the cost of a syscall per event. Suitable
// for end-to-end demos and integration smoke tests, NOT for the Jetson
// decode hot path performance budget in spec § 9.
#include "loom/loom_c.h"
#include "state.h"

#include <atomic>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <ctime>
#include <mutex>
#include <string>
#include <sys/stat.h>
#include <sys/types.h>
#include <time.h>

namespace {

// Minimal JSON string escaping. Sufficient for the keys/names Loom emits;
// M2-full's attribute encoder replaces this with a binary-then-canonical-JSON
// path through the daemon.
void json_str(std::FILE* f, const char* s, size_t n) {
  for (size_t i = 0; i < n; i++) {
    unsigned char c = static_cast<unsigned char>(s[i]);
    switch (c) {
      case '"':  std::fputs("\\\"", f); break;
      case '\\': std::fputs("\\\\", f); break;
      case '\n': std::fputs("\\n",  f); break;
      case '\r': std::fputs("\\r",  f); break;
      case '\t': std::fputs("\\t",  f); break;
      default:
        if (c < 0x20) std::fprintf(f, "\\u%04x", c);
        else          std::fputc(static_cast<int>(c), f);
    }
  }
}

void format_iso8601(char* buf, size_t cap) {
  struct timespec ts;
  clock_gettime(CLOCK_REALTIME, &ts);
  struct tm tm;
  gmtime_r(&ts.tv_sec, &tm);
  size_t n = std::strftime(buf, cap, "%Y-%m-%dT%H:%M:%S", &tm);
  std::snprintf(buf + n, cap - n, ".%03ldZ",
                static_cast<long>(ts.tv_nsec / 1000000));
}

// Best-effort recursive mkdir; ignores errors. Caller checks fopen() result.
void mkdir_p(const std::string& path) {
  for (size_t pos = 1; (pos = path.find('/', pos)) != std::string::npos; pos++) {
    auto sub = path.substr(0, pos);
    mkdir(sub.c_str(), 0700);
  }
  mkdir(path.c_str(), 0700);
}

std::string artifact_dir_for(const char* run_id) {
  const char* home_env = std::getenv("LOOM_HOME");
  std::string home;
  if (home_env && *home_env) {
    home = home_env;
  } else {
    const char* h = std::getenv("HOME");
    home = std::string(h ? h : "") + "/.loom";
  }
  return home + "/runs/" + run_id;
}

void emit(const char* cat, const char* name, size_t name_len) {
  if (!loom::detail::g_active.load(std::memory_order_acquire)) return;
  std::FILE* f = loom::detail::g_events_file;
  if (!f) return;

  uint64_t seq = loom::detail::g_seq.fetch_add(1, std::memory_order_relaxed);
  char ts[40];
  format_iso8601(ts, sizeof(ts));

  std::lock_guard<std::mutex> lk(loom::detail::g_write_mutex);
  std::fprintf(f,
               "{\"v\":\"loom.event.v1\",\"cat\":\"%s\",\"seq\":%llu,"
               "\"ts\":\"%s\",\"name\":\"",
               cat, static_cast<unsigned long long>(seq), ts);
  json_str(f, name, name_len);
  std::fputs("\"}\n", f);
  std::fflush(f);  // M2-partial always flushes; M2-full ring buffer batches.
}

}  // namespace

extern "C" int loom_init(void) {
  // Idempotent: a process that links libloom auto-initializes via the
  // constructor below, and may also call loom::init() explicitly. Second
  // and later calls are cheap no-ops.
  if (loom::detail::g_events_file != nullptr ||
      loom::detail::g_active.load(std::memory_order_acquire)) {
    return 0;
  }

  const char* run_id = std::getenv("LOOM_RUN_ID");
  if (!run_id || !*run_id) return 0;  // stay inactive

  auto dir  = artifact_dir_for(run_id);
  mkdir_p(dir);
  auto path = dir + "/events.jsonl";

  std::FILE* f = std::fopen(path.c_str(), "ab");
  if (!f) return -1;

  loom::detail::g_events_file = f;
  loom::detail::g_seq.store(0, std::memory_order_relaxed);
  loom::detail::g_active.store(true, std::memory_order_release);
  return 0;
}

// Auto-init when libloom is linked into a process. A consumer that does
// nothing more than link liblooom.a still gets observed (provided
// LOOM_RUN_ID is set in env). This is what makes bedrock's loom_hooks.h
// path work without bedrock_bench having to call loom::init().
__attribute__((constructor))
static void loom_auto_init(void)     { loom_init(); }
__attribute__((destructor))
static void loom_auto_shutdown(void) { loom_shutdown(); }

extern "C" void loom_shutdown(void) {
  bool was_active = loom::detail::g_active.exchange(false, std::memory_order_acq_rel);
  if (!was_active) return;
  std::lock_guard<std::mutex> lk(loom::detail::g_write_mutex);
  if (loom::detail::g_events_file) {
    std::fflush(loom::detail::g_events_file);
    std::fclose(loom::detail::g_events_file);
    loom::detail::g_events_file = nullptr;
  }
}

extern "C" int loom_active(void) {
  return loom::detail::g_active.load(std::memory_order_acquire) ? 1 : 0;
}

// ── Spans ─────────────────────────────────────────────────────────────────
// M2-partial: span_begin emits the named event; span_end is a no-op. Real
// duration tracking and parent linkage land in M2-full alongside the ring.
extern "C" uint64_t loom_span_begin(const char* name, size_t name_len,
                                    const uint8_t* /*attrs*/, size_t /*attrs_len*/) {
  if (!loom::detail::g_active.load(std::memory_order_acquire)) return 0;
  emit("span", name, name_len);
  return 1;  // sentinel, opaque to caller
}
extern "C" void loom_span_end(uint64_t /*handle*/) { /* paired in M2-full */ }
extern "C" void loom_span_annotate(uint64_t /*h*/, const char* /*key*/, size_t /*kl*/,
                                   const uint8_t* /*attrs*/, size_t /*al*/) {
  /* annotations land with M2-full attribute encoder */
}

// ── Metrics ───────────────────────────────────────────────────────────────
// M2-partial: name is recorded; numeric value is dropped until the attribute
// encoder ships. Producers should still call these so the call graph is
// pinned and the events appear in events.jsonl.
extern "C" void loom_metric_i64(const char* name, size_t name_len, int64_t /*v*/) {
  emit("metric", name, name_len);
}
extern "C" void loom_metric_f64(const char* name, size_t name_len, double /*v*/) {
  emit("metric", name, name_len);
}
extern "C" void loom_counter_inc(const char* name, size_t name_len, int64_t /*delta*/) {
  emit("metric", name, name_len);
}

// ── Errors / audit / lifecycle ───────────────────────────────────────────
extern "C" void loom_error(const char* name, size_t name_len,
                           const char* /*msg*/, size_t /*ml*/,
                           int /*sev*/, const uint8_t* /*attrs*/, size_t /*al*/) {
  emit("error", name, name_len);
}
extern "C" void loom_audit(const char* name, size_t name_len,
                           const uint8_t* /*attrs*/, size_t /*al*/, int /*sync*/) {
  emit("audit", name, name_len);
}
extern "C" void loom_lifecycle(const char* marker, size_t marker_len,
                               const uint8_t* /*attrs*/, size_t /*al*/) {
  emit("lifecycle", marker, marker_len);
}

// ── Attribute encoders ────────────────────────────────────────────────────
// Stubs in M2-partial; the wire format ring header + attribute encoding lands
// in M2-full. The C++ wrappers in <loom/loom.h> still compile and call these,
// so the API surface is pinned.
extern "C" size_t loom_attrs_begin(uint8_t*, size_t)         { return 0; }
extern "C" size_t loom_attrs_i64  (uint8_t*, size_t, size_t,
                                   const char*, size_t, int64_t) { return 0; }
extern "C" size_t loom_attrs_f64  (uint8_t*, size_t, size_t,
                                   const char*, size_t, double)  { return 0; }
extern "C" size_t loom_attrs_str  (uint8_t*, size_t, size_t,
                                   const char*, size_t,
                                   const char*, size_t)         { return 0; }
extern "C" size_t loom_attrs_bool (uint8_t*, size_t, size_t,
                                   const char*, size_t, int)    { return 0; }
