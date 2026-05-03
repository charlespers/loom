// embed/src/loom.cpp — Loom embed library runtime + finalizer.
//
// Single translation unit covering: init/shutdown, the C ABI surface for
// span / metric / audit / lifecycle / error, span pairing with duration,
// per-name span aggregates, the audit SHA-256 hash chain, and the
// finalization step that writes manifest.json + summary.md at the end of
// a run.
//
// The binary attribute encoder/decoder lives in attrs.cpp; SHA-256 lives
// in sha256.cpp; the loom_hook_* shim lives in hooks.cpp. Those three are
// kept separate because they're independent utilities; everything else
// belongs together so the runtime can be read top-to-bottom.
#include "loom/loom_c.h"
#include "loom/version.h"
#include "attrs.h"
#include "sha256.h"
#include "state.h"

#include <algorithm>
#include <atomic>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <ctime>
#include <mutex>
#include <string>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/utsname.h>
#include <time.h>
#include <unistd.h>
#include <vector>

#ifdef __APPLE__
  #include <mach-o/dyld.h>
#endif

namespace loom::detail {

// ─────────────────────────────────────────────────────────────────────────
// State singleton + thread-local span stack
// ─────────────────────────────────────────────────────────────────────────

State& state() {
  static State s;
  return s;
}

std::vector<uint64_t>& thread_span_stack() {
  thread_local std::vector<uint64_t> stack;
  return stack;
}

// ─────────────────────────────────────────────────────────────────────────
// Time, paths, file helpers
// ─────────────────────────────────────────────────────────────────────────

namespace {

uint64_t now_unix_ns() {
  struct timespec ts;
  clock_gettime(CLOCK_REALTIME, &ts);
  return static_cast<uint64_t>(ts.tv_sec) * 1000000000ull +
         static_cast<uint64_t>(ts.tv_nsec);
}

uint64_t now_monotonic_ns() {
  struct timespec ts;
  clock_gettime(CLOCK_MONOTONIC, &ts);
  return static_cast<uint64_t>(ts.tv_sec) * 1000000000ull +
         static_cast<uint64_t>(ts.tv_nsec);
}

void format_iso8601(uint64_t unix_ns, char* buf, size_t cap) {
  time_t   sec = static_cast<time_t>(unix_ns / 1000000000ull);
  uint64_t ms  = (unix_ns / 1000000ull) % 1000ull;
  struct tm tm;
  gmtime_r(&sec, &tm);
  size_t n = std::strftime(buf, cap, "%Y-%m-%dT%H:%M:%S", &tm);
  std::snprintf(buf + n, cap - n, ".%03lluZ",
                static_cast<unsigned long long>(ms));
}

std::string iso8601(uint64_t unix_ns) {
  char buf[40];
  format_iso8601(unix_ns, buf, sizeof(buf));
  return buf;
}

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

std::FILE* open_append(const std::string& path, mode_t mode_bits) {
  std::FILE* f = std::fopen(path.c_str(), "ab");
  if (f) chmod(path.c_str(), mode_bits);
  return f;
}

void capture_process(State& s) {
  s.pid  = static_cast<uint64_t>(getpid());
  s.ppid = static_cast<uint64_t>(getppid());

  char path[4096];
  if (getcwd(path, sizeof(path)) != nullptr) s.cwd = path;

#ifdef __APPLE__
  uint32_t bufsize = sizeof(path);
  if (_NSGetExecutablePath(path, &bufsize) == 0) s.executable = path;
#else
  ssize_t n = readlink("/proc/self/exe", path, sizeof(path) - 1);
  if (n > 0) { path[n] = 0; s.executable = path; }
  std::FILE* f = std::fopen("/proc/self/cmdline", "rb");
  if (f) {
    std::string buf;
    int c;
    while ((c = std::fgetc(f)) != EOF) buf.push_back(static_cast<char>(c));
    std::fclose(f);
    size_t start = 0;
    for (size_t i = 0; i < buf.size(); i++) {
      if (buf[i] == 0) {
        s.argv.emplace_back(buf.data() + start, i - start);
        start = i + 1;
      }
    }
  }
#endif
  if (!s.argv.empty()) {
    s.command = s.argv[0];
    for (size_t i = 1; i < s.argv.size(); i++) {
      s.command += ' ';
      s.command += s.argv[i];
    }
  } else if (!s.executable.empty()) {
    s.command = s.executable;
  }

  struct utsname u;
  if (uname(&u) == 0) {
    s.os_name        = u.sysname;
    s.os_arch        = u.machine;
    s.kernel_release = u.release;
    s.hostname       = u.nodename;
  }
}

// ─────────────────────────────────────────────────────────────────────────
// Event line writer — used by every category.
// ─────────────────────────────────────────────────────────────────────────

void write_event(const char* cat, uint64_t seq, uint64_t ts_unix_ns,
                 const std::string& body) {
  State& s = state();
  std::FILE* f = s.events_file;
  if (!f) return;
  char ts[40];
  format_iso8601(ts_unix_ns, ts, sizeof(ts));
  std::lock_guard<std::mutex> lk(s.write_mutex);
  std::fprintf(f,
               "{\"v\":\"%s\",\"cat\":\"%s\",\"seq\":%llu,\"ts\":\"%s\"%s}\n",
               LOOM_WIRE_SCHEMA, cat,
               static_cast<unsigned long long>(seq), ts,
               body.c_str());
  std::fflush(f);
}

void format_attrs_field(const uint8_t* attrs, size_t attrs_len,
                        std::string& out) {
  std::string inner;
  size_t n = append_attrs_json(attrs, attrs_len, inner);
  if (n == 0) {
    out += ",\"attrs\":{}";
    return;
  }
  out += ",\"attrs\":{";
  out += inner;
  out.push_back('}');
}

}  // namespace

}  // namespace loom::detail

// ─────────────────────────────────────────────────────────────────────────
// Public C ABI — Lifecycle
// ─────────────────────────────────────────────────────────────────────────

extern "C" int loom_init(void) {
  using namespace loom::detail;
  State& s = state();

  if (s.events_file != nullptr ||
      s.active.load(std::memory_order_acquire)) {
    return 0;
  }

  const char* run_id = std::getenv("LOOM_RUN_ID");
  if (!run_id || !*run_id) return 0;

  s.run_id        = run_id;
  s.artifact_dir  = artifact_dir_for(run_id);
  s.start_unix_ns = now_unix_ns();
  capture_process(s);

  mkdir_p(s.artifact_dir);

  s.events_file       = open_append(s.artifact_dir + "/events.jsonl",       0600);
  s.audit_file        = open_append(s.artifact_dir + "/audit.jsonl",         0600);
  s.audit_public_file = open_append(s.artifact_dir + "/audit.public.jsonl",  0644);
  if (!s.events_file || !s.audit_file || !s.audit_public_file) {
    if (s.events_file)       { std::fclose(s.events_file);       s.events_file       = nullptr; }
    if (s.audit_file)        { std::fclose(s.audit_file);        s.audit_file        = nullptr; }
    if (s.audit_public_file) { std::fclose(s.audit_public_file); s.audit_public_file = nullptr; }
    return -1;
  }

  s.seq.store(0, std::memory_order_relaxed);
  s.next_span_id.store(1, std::memory_order_relaxed);
  s.active.store(true, std::memory_order_release);

  // Open with a "run.start" lifecycle event. Carries run_id as an attribute
  // so it's also visible in events.jsonl without consulting manifest.json.
  uint8_t buf[64];
  size_t  off = loom_attrs_begin(buf, sizeof(buf));
  off = loom_attrs_str(buf, sizeof(buf), off, "run_id", 6,
                       s.run_id.data(), s.run_id.size());
  loom_lifecycle("run.start", 9, buf, off);
  return 0;
}

// Forward declaration — finalize_run is defined later in this TU.
namespace loom::detail { void finalize_run(State& s); }

extern "C" void loom_shutdown(void) {
  using namespace loom::detail;
  State& s = state();

  bool was_active = s.active.exchange(false, std::memory_order_acq_rel);
  if (!was_active) return;

  if (!s.shutdown_done.exchange(true, std::memory_order_acq_rel)) {
    // Closing lifecycle: temporarily reactivate so it gets emitted.
    s.active.store(true, std::memory_order_release);
    loom_lifecycle("run.end", 7, nullptr, 0);
    s.active.store(false, std::memory_order_release);
    finalize_run(s);
  }

  std::lock_guard<std::mutex> lk(s.write_mutex);
  if (s.events_file)       { std::fflush(s.events_file);       std::fclose(s.events_file);       s.events_file       = nullptr; }
  if (s.audit_file)        { std::fflush(s.audit_file);        std::fclose(s.audit_file);        s.audit_file        = nullptr; }
  if (s.audit_public_file) { std::fflush(s.audit_public_file); std::fclose(s.audit_public_file); s.audit_public_file = nullptr; }
}

extern "C" int loom_active(void) {
  return loom::detail::state().active.load(std::memory_order_acquire) ? 1 : 0;
}

__attribute__((constructor))
static void loom_auto_init(void)     { loom_init(); }
__attribute__((destructor))
static void loom_auto_shutdown(void) { loom_shutdown(); }

// ─────────────────────────────────────────────────────────────────────────
// Public C ABI — Spans
// ─────────────────────────────────────────────────────────────────────────

extern "C" uint64_t loom_span_begin(const char* name, size_t name_len,
                                    const uint8_t* attrs, size_t attrs_len) {
  using namespace loom::detail;
  State& s = state();
  if (!s.active.load(std::memory_order_acquire)) return 0;

  uint64_t handle   = s.next_span_id.fetch_add(1, std::memory_order_relaxed);
  uint64_t start_ns = now_monotonic_ns();
  uint64_t parent   = 0;
  auto& stack = thread_span_stack();
  if (!stack.empty()) parent = stack.back();
  stack.push_back(handle);

  std::string attrs_json;
  if (attrs && attrs_len) append_attrs_json(attrs, attrs_len, attrs_json);

  std::lock_guard<std::mutex> lk(s.spans_mutex);
  SpanRecord r;
  r.start_ns   = start_ns;
  r.parent_id  = parent;
  r.name       = std::string(name, name_len);
  r.attrs_json = std::move(attrs_json);
  s.active_spans.emplace(handle, std::move(r));
  return handle;
}

extern "C" void loom_span_end(uint64_t handle) {
  using namespace loom::detail;
  if (handle == 0) return;
  State& s = state();
  if (!s.active.load(std::memory_order_acquire)) return;

  uint64_t end_ns = now_monotonic_ns();
  SpanRecord rec;
  bool found = false;
  {
    std::lock_guard<std::mutex> lk(s.spans_mutex);
    auto it = s.active_spans.find(handle);
    if (it != s.active_spans.end()) {
      rec   = std::move(it->second);
      found = true;
      s.active_spans.erase(it);
    }
  }
  if (!found) return;

  uint64_t dur_ns = (end_ns > rec.start_ns) ? (end_ns - rec.start_ns) : 0;

  auto& stack = thread_span_stack();
  for (auto it = stack.rbegin(); it != stack.rend(); ++it) {
    if (*it == handle) {
      stack.erase(std::next(it).base());
      break;
    }
  }

  {
    std::lock_guard<std::mutex> lk(s.span_stats_mutex);
    auto& st = s.span_stats[rec.name];
    st.count++;
    st.total_ns += dur_ns;
    if (dur_ns < st.min_ns) st.min_ns = dur_ns;
    if (dur_ns > st.max_ns) st.max_ns = dur_ns;
    if (st.samples.size() < kMaxSpanSamples) st.samples.push_back(dur_ns);
  }
  s.count_span.fetch_add(1, std::memory_order_relaxed);

  uint64_t seq    = s.seq.fetch_add(1, std::memory_order_relaxed);
  uint64_t now_ns = now_unix_ns();

  std::string body;
  body.reserve(128);
  body += ",\"name\":";
  append_json_string(rec.name, body);
  body += ",\"span_id\":";
  body += std::to_string(handle);
  if (rec.parent_id != 0) {
    body += ",\"parent\":";
    body += std::to_string(rec.parent_id);
  }
  body += ",\"dur_ns\":";
  body += std::to_string(dur_ns);
  if (!rec.attrs_json.empty()) {
    body += ",\"attrs\":{";
    body += rec.attrs_json;
    body.push_back('}');
  } else {
    body += ",\"attrs\":{}";
  }
  write_event("span", seq, now_ns, body);
}

extern "C" void loom_span_annotate(uint64_t handle,
                                   const uint8_t* attrs, size_t attrs_len) {
  using namespace loom::detail;
  if (handle == 0) return;
  State& s = state();
  if (!s.active.load(std::memory_order_acquire)) return;
  if (!attrs || !attrs_len) return;
  std::lock_guard<std::mutex> lk(s.spans_mutex);
  auto it = s.active_spans.find(handle);
  if (it == s.active_spans.end()) return;
  if (!it->second.attrs_json.empty()) it->second.attrs_json.push_back(',');
  append_attrs_json(attrs, attrs_len, it->second.attrs_json);
}

// ─────────────────────────────────────────────────────────────────────────
// Public C ABI — Metrics
// ─────────────────────────────────────────────────────────────────────────

namespace loom::detail {
namespace {
void emit_metric(const char* name, size_t name_len,
                 const char* kind, const char* value_lit,
                 const uint8_t* attrs, size_t attrs_len) {
  State& s = state();
  if (!s.active.load(std::memory_order_acquire)) return;
  uint64_t seq = s.seq.fetch_add(1, std::memory_order_relaxed);
  s.count_metric.fetch_add(1, std::memory_order_relaxed);

  std::string body;
  body.reserve(64 + name_len);
  body += ",\"name\":";
  append_json_string(name, name_len, body);
  body += ",\"value\":";
  body += value_lit;
  body += ",\"kind\":\"";
  body += kind;
  body += '"';
  format_attrs_field(attrs, attrs_len, body);
  write_event("metric", seq, now_unix_ns(), body);
}
}  // namespace
}  // namespace loom::detail

extern "C" void loom_metric_i64(const char* name, size_t name_len, int64_t v,
                                const uint8_t* attrs, size_t attrs_len) {
  char tmp[32];
  std::snprintf(tmp, sizeof(tmp), "%lld", static_cast<long long>(v));
  loom::detail::emit_metric(name, name_len, "i64", tmp, attrs, attrs_len);
}
extern "C" void loom_metric_f64(const char* name, size_t name_len, double v,
                                const uint8_t* attrs, size_t attrs_len) {
  char tmp[32];
  std::snprintf(tmp, sizeof(tmp), "%.10g", v);
  loom::detail::emit_metric(name, name_len, "f64", tmp, attrs, attrs_len);
}
extern "C" void loom_counter_inc(const char* name, size_t name_len, int64_t delta,
                                 const uint8_t* attrs, size_t attrs_len) {
  char tmp[32];
  std::snprintf(tmp, sizeof(tmp), "%lld", static_cast<long long>(delta));
  loom::detail::emit_metric(name, name_len, "counter", tmp, attrs, attrs_len);
}

// ─────────────────────────────────────────────────────────────────────────
// Public C ABI — Errors
// ─────────────────────────────────────────────────────────────────────────

extern "C" void loom_error(const char* name, size_t name_len,
                           const char* msg, size_t msg_len, int sev,
                           const uint8_t* attrs, size_t attrs_len) {
  using namespace loom::detail;
  State& s = state();
  if (!s.active.load(std::memory_order_acquire)) return;
  uint64_t seq   = s.seq.fetch_add(1, std::memory_order_relaxed);
  s.count_error.fetch_add(1, std::memory_order_relaxed);
  uint64_t ts_ns = now_unix_ns();

  const char* sev_str = (sev == 0) ? "warn" : (sev == 2) ? "fatal" : "error";

  std::string body;
  body.reserve(96 + name_len + msg_len);
  body += ",\"name\":";
  append_json_string(name, name_len, body);
  body += ",\"message\":";
  append_json_string(msg ? msg : "", msg_len, body);
  body += ",\"severity\":\"";
  body += sev_str;
  body += '"';
  format_attrs_field(attrs, attrs_len, body);
  write_event("error", seq, ts_ns, body);

  ErrorRecord rec;
  rec.ts_unix_ns = ts_ns;
  rec.name       = std::string(name, name_len);
  rec.message    = std::string(msg ? msg : "", msg_len);
  rec.severity   = sev;
  if (attrs && attrs_len) append_attrs_json(attrs, attrs_len, rec.attrs_json);
  std::lock_guard<std::mutex> lk(s.errors_mutex);
  if (s.errors.size() < 1024) s.errors.push_back(std::move(rec));
}

// ─────────────────────────────────────────────────────────────────────────
// Public C ABI — Audit (with hash chain)
// ─────────────────────────────────────────────────────────────────────────
//
// Each audit event is appended to two files:
//   audit.jsonl         (private, mode 0600, full attrs)
//   audit.public.jsonl  (mode 0644, attrs replaced with empty {} in this
//                        M2-partial cut; M3 introduces field-level @public)
//
// Both files share the same hash chain. For the i'th record the chain
// fields are:
//   chain.prev = hash of record (i-1)'s canonical-JSON line, "0"*64 for i=0
//   chain.this = sha256(prev || canonical_payload)
// where canonical_payload is the record's body fields excluding the chain
// field itself, written in a stable order. A verifier rehashes each
// canonical_payload + previous-hash and checks chain.this matches.

extern "C" void loom_audit(const char* name, size_t name_len,
                           const uint8_t* attrs, size_t attrs_len, int sync) {
  using namespace loom::detail;
  State& s = state();
  if (!s.active.load(std::memory_order_acquire)) return;
  if (!s.audit_file) return;

  uint64_t seq   = s.seq.fetch_add(1, std::memory_order_relaxed);
  s.count_audit.fetch_add(1, std::memory_order_relaxed);
  uint64_t ts_ns = now_unix_ns();
  std::string ts = iso8601(ts_ns);

  // Build canonical payload (the part that gets hashed) and full body
  // (canonical payload + chain fields).
  std::string attrs_json;
  if (attrs && attrs_len) append_attrs_json(attrs, attrs_len, attrs_json);

  std::string canonical;
  canonical.reserve(96 + name_len + attrs_json.size());
  canonical += "\"v\":\"";
  canonical += LOOM_WIRE_SCHEMA;
  canonical += "\",\"cat\":\"audit\",\"seq\":";
  canonical += std::to_string(seq);
  canonical += ",\"ts\":\"";
  canonical += ts;
  canonical += "\",\"name\":";
  append_json_string(name, name_len, canonical);
  if (!attrs_json.empty()) {
    canonical += ",\"attrs\":{";
    canonical += attrs_json;
    canonical.push_back('}');
  } else {
    canonical += ",\"attrs\":{}";
  }

  // Chain step.
  std::string prev_hex;
  std::string this_hex;
  {
    std::lock_guard<std::mutex> lk(s.chain_mutex);
    prev_hex = s.chain.head_hex.empty()
               ? std::string(64, '0')
               : s.chain.head_hex;
    Sha256 h;
    h.update(prev_hex.data(), prev_hex.size());
    h.update(canonical.data(), canonical.size());
    this_hex = h.finalize_hex();
    s.chain.head_hex = this_hex;
    s.chain.count++;
  }

  // Compose private record (full attrs + chain).
  std::string private_line;
  private_line.reserve(canonical.size() + 160);
  private_line.push_back('{');
  private_line += canonical;
  private_line += ",\"chain\":{\"prev\":\"";
  private_line += prev_hex;
  private_line += "\",\"this\":\"";
  private_line += this_hex;
  private_line += "\"}}\n";

  // Public record: structural fields verbatim; attrs filtered to only
  // those keys ending in "@public" (suffix stripped on the way out).
  // The chain is shared — verifiers rehash the private record.
  std::string public_attrs_json;
  if (attrs && attrs_len) {
    append_attrs_json_public(attrs, attrs_len, public_attrs_json);
  }
  std::string public_canonical;
  public_canonical.reserve(96 + name_len + public_attrs_json.size());
  public_canonical += "\"v\":\"";
  public_canonical += LOOM_WIRE_SCHEMA;
  public_canonical += "\",\"cat\":\"audit\",\"seq\":";
  public_canonical += std::to_string(seq);
  public_canonical += ",\"ts\":\"";
  public_canonical += ts;
  public_canonical += "\",\"name\":";
  append_json_string(name, name_len, public_canonical);
  if (!public_attrs_json.empty()) {
    public_canonical += ",\"attrs\":{";
    public_canonical += public_attrs_json;
    public_canonical.push_back('}');
  } else {
    public_canonical += ",\"attrs\":{}";
  }

  std::string public_line;
  public_line.reserve(public_canonical.size() + 160);
  public_line.push_back('{');
  public_line += public_canonical;
  public_line += ",\"chain\":{\"prev\":\"";
  public_line += prev_hex;
  public_line += "\",\"this\":\"";
  public_line += this_hex;
  public_line += "\"}}\n";

  // Write both files + the events.jsonl mirror, all under one lock so a
  // crash mid-write leaves the chain head atomic with disk state.
  {
    std::lock_guard<std::mutex> lk(s.write_mutex);
    if (s.audit_file) {
      std::fputs(private_line.c_str(), s.audit_file);
      std::fflush(s.audit_file);
      if (sync) {
        // Best-effort fsync; ignore failures (e.g. tmpfs).
        ::fsync(fileno(s.audit_file));
      }
    }
    if (s.audit_public_file) {
      std::fputs(public_line.c_str(), s.audit_public_file);
      std::fflush(s.audit_public_file);
      if (sync) ::fsync(fileno(s.audit_public_file));
    }
    if (s.events_file) {
      // events.jsonl mirror — full content, chain elided (for compactness;
      // the audit files are the canonical chain).
      std::fputs(private_line.c_str(), s.events_file);
      std::fflush(s.events_file);
    }
  }
}

// ─────────────────────────────────────────────────────────────────────────
// Public C ABI — Lifecycle
// ─────────────────────────────────────────────────────────────────────────

extern "C" void loom_lifecycle(const char* marker, size_t marker_len,
                               const uint8_t* attrs, size_t attrs_len) {
  using namespace loom::detail;
  State& s = state();
  if (!s.active.load(std::memory_order_acquire)) return;
  uint64_t seq   = s.seq.fetch_add(1, std::memory_order_relaxed);
  s.count_lifecycle.fetch_add(1, std::memory_order_relaxed);
  uint64_t ts_ns = now_unix_ns();

  std::string body;
  body.reserve(48 + marker_len);
  body += ",\"name\":";
  append_json_string(marker, marker_len, body);
  format_attrs_field(attrs, attrs_len, body);
  write_event("lifecycle", seq, ts_ns, body);

  LifecycleRecord rec;
  rec.ts_unix_ns = ts_ns;
  rec.marker     = std::string(marker, marker_len);
  if (attrs && attrs_len) append_attrs_json(attrs, attrs_len, rec.attrs_json);
  std::lock_guard<std::mutex> lk(s.lifecycle_mutex);
  if (s.lifecycles.size() < 4096) s.lifecycles.push_back(std::move(rec));
}

// ─────────────────────────────────────────────────────────────────────────
// Finalization — manifest.json + summary.md
// ─────────────────────────────────────────────────────────────────────────

namespace loom::detail {
namespace {

uint64_t percentile(std::vector<uint64_t> samples, double pct) {
  if (samples.empty()) return 0;
  std::sort(samples.begin(), samples.end());
  size_t idx = static_cast<size_t>((pct / 100.0) * (samples.size() - 1));
  if (idx >= samples.size()) idx = samples.size() - 1;
  return samples[idx];
}

std::string format_duration_ns(uint64_t ns) {
  char buf[32];
  if (ns < 1000ull) {
    std::snprintf(buf, sizeof(buf), "%lluns", static_cast<unsigned long long>(ns));
  } else if (ns < 1000000ull) {
    std::snprintf(buf, sizeof(buf), "%.2fµs", ns / 1e3);
  } else if (ns < 1000000000ull) {
    std::snprintf(buf, sizeof(buf), "%.2fms", ns / 1e6);
  } else {
    std::snprintf(buf, sizeof(buf), "%.2fs", ns / 1e9);
  }
  return buf;
}

uint64_t file_size(const std::string& path) {
  struct stat st{};
  if (::stat(path.c_str(), &st) != 0) return 0;
  return static_cast<uint64_t>(st.st_size);
}

uint64_t file_lines(const std::string& path) {
  std::FILE* f = std::fopen(path.c_str(), "rb");
  if (!f) return 0;
  uint64_t lines = 0;
  int c;
  while ((c = std::fgetc(f)) != EOF) if (c == '\n') lines++;
  std::fclose(f);
  return lines;
}

void write_manifest(State& s, uint64_t end_unix_ns, uint64_t duration_ms) {
  std::string path = s.artifact_dir + "/manifest.json";
  std::FILE* f = std::fopen(path.c_str(), "wb");
  if (!f) return;

  std::string events_path = s.artifact_dir + "/events.jsonl";
  std::string audit_path  = s.artifact_dir + "/audit.jsonl";
  std::string audit_pub   = s.artifact_dir + "/audit.public.jsonl";
  std::string summary     = s.artifact_dir + "/summary.md";

  uint64_t evt_lines = file_lines(events_path);
  uint64_t evt_size  = file_size (events_path);
  uint64_t aud_lines = file_lines(audit_path);
  uint64_t aud_size  = file_size (audit_path);
  uint64_t aup_lines = file_lines(audit_pub);
  uint64_t aup_size  = file_size (audit_pub);
  uint64_t sum_size  = file_size (summary);

  std::string out;
  out.reserve(2048);
  out += "{\n";
  out += "  \"schema\": \"loom.manifest.v1\",\n";
  out += "  \"loom_version\": \"" LOOM_VERSION_STRING "\",\n";
  out += "  \"wire_schema\": \"" LOOM_WIRE_SCHEMA "\",\n";
  out += "  \"run_id\": "; append_json_string(s.run_id, out); out += ",\n";
  out += "  \"started_at\": \""; out += iso8601(s.start_unix_ns); out += "\",\n";
  out += "  \"started_at_unix_ns\": " + std::to_string(s.start_unix_ns) + ",\n";
  out += "  \"ended_at\": \"";   out += iso8601(end_unix_ns);    out += "\",\n";
  out += "  \"ended_at_unix_ns\": " + std::to_string(end_unix_ns) + ",\n";
  out += "  \"duration_ms\": "  + std::to_string(duration_ms)  + ",\n";
  out += "  \"status\": \"completed\",\n";
  out += "  \"process\": {\n";
  out += "    \"pid\": "  + std::to_string(s.pid)  + ",\n";
  out += "    \"ppid\": " + std::to_string(s.ppid) + ",\n";
  out += "    \"executable\": "; append_json_string(s.executable, out); out += ",\n";
  out += "    \"command\": ";    append_json_string(s.command,    out); out += ",\n";
  out += "    \"argv\": [";
  for (size_t i = 0; i < s.argv.size(); i++) {
    if (i) out += ", ";
    append_json_string(s.argv[i], out);
  }
  out += "],\n";
  out += "    \"cwd\": "; append_json_string(s.cwd, out); out += "\n";
  out += "  },\n";
  out += "  \"host\": {\n";
  out += "    \"hostname\": ";       append_json_string(s.hostname,       out); out += ",\n";
  out += "    \"os\": ";              append_json_string(s.os_name,        out); out += ",\n";
  out += "    \"arch\": ";            append_json_string(s.os_arch,        out); out += ",\n";
  out += "    \"kernel_release\": "; append_json_string(s.kernel_release, out); out += "\n";
  out += "  },\n";
  out += "  \"counts\": {\n";
  out += "    \"events_total\": " + std::to_string(s.seq.load()) + ",\n";
  out += "    \"by_category\": {\n";
  out += "      \"span\": "      + std::to_string(s.count_span.load())      + ",\n";
  out += "      \"metric\": "    + std::to_string(s.count_metric.load())    + ",\n";
  out += "      \"audit\": "     + std::to_string(s.count_audit.load())     + ",\n";
  out += "      \"lifecycle\": " + std::to_string(s.count_lifecycle.load()) + ",\n";
  out += "      \"error\": "     + std::to_string(s.count_error.load())     + "\n";
  out += "    }\n";
  out += "  },\n";
  out += "  \"spans\": {\n";
  out += "    \"by_name\": {\n";
  bool first = true;
  for (auto& kv : s.span_stats) {
    auto& st = kv.second;
    uint64_t p50 = percentile(st.samples, 50.0);
    uint64_t p95 = percentile(st.samples, 95.0);
    uint64_t p99 = percentile(st.samples, 99.0);
    if (!first) out += ",\n";
    first = false;
    out += "      ";
    append_json_string(kv.first, out);
    out += ": {";
    out += "\"count\":"   + std::to_string(st.count);
    out += ",\"total_ns\":" + std::to_string(st.total_ns);
    out += ",\"min_ns\":"   + std::to_string(st.min_ns == UINT64_MAX ? 0 : st.min_ns);
    out += ",\"max_ns\":"   + std::to_string(st.max_ns);
    out += ",\"p50_ns\":"   + std::to_string(p50);
    out += ",\"p95_ns\":"   + std::to_string(p95);
    out += ",\"p99_ns\":"   + std::to_string(p99);
    out += "}";
  }
  out += "\n    }\n";
  out += "  },\n";
  out += "  \"audit_chain\": {\n";
  out += "    \"head\": \"" + (s.chain.head_hex.empty()
                              ? std::string(64, '0')
                              : s.chain.head_hex) + "\",\n";
  out += "    \"count\": "  + std::to_string(s.chain.count) + "\n";
  out += "  },\n";
  out += "  \"files\": {\n";
  out += "    \"events.jsonl\":       {\"size_bytes\":" + std::to_string(evt_size) + ",\"lines\":" + std::to_string(evt_lines) + "},\n";
  out += "    \"audit.jsonl\":        {\"size_bytes\":" + std::to_string(aud_size) + ",\"lines\":" + std::to_string(aud_lines) + "},\n";
  out += "    \"audit.public.jsonl\": {\"size_bytes\":" + std::to_string(aup_size) + ",\"lines\":" + std::to_string(aup_lines) + "},\n";
  out += "    \"summary.md\":         {\"size_bytes\":" + std::to_string(sum_size) + "}\n";
  out += "  }\n";
  out += "}\n";

  std::fputs(out.c_str(), f);
  std::fclose(f);
}

void write_summary(State& s, uint64_t end_unix_ns, uint64_t duration_ms) {
  std::string path = s.artifact_dir + "/summary.md";
  std::FILE* f = std::fopen(path.c_str(), "wb");
  if (!f) return;

  std::string out;
  out.reserve(2048);

  out += "# Run `";
  out += s.run_id;
  out += "`\n\n";
  out += "**Status**: ✓ completed in ";
  out += format_duration_ns(static_cast<uint64_t>(duration_ms) * 1000000ull);
  out += "\n\n";
  out += "| | |\n|---|---|\n";
  out += "| Started  | " + iso8601(s.start_unix_ns) + " |\n";
  out += "| Ended    | " + iso8601(end_unix_ns)     + " |\n";
  out += "| Command  | `" + (s.command.empty() ? std::string("(unknown)") : s.command) + "` |\n";
  out += "| PID      | " + std::to_string(s.pid) + " |\n";
  out += "| Host     | `" + s.hostname + "` (" + s.os_name + "/" + s.os_arch + ") |\n";
  out += "| Loom     | v" LOOM_VERSION_STRING ", schema `" LOOM_WIRE_SCHEMA "` |\n\n";

  // Lifecycle outline
  out += "## Lifecycle\n\n";
  if (s.lifecycles.empty()) {
    out += "_(no lifecycle markers)_\n\n";
  } else {
    out += "```\n";
    for (auto& lc : s.lifecycles) {
      char ts[24];
      time_t   sec = static_cast<time_t>(lc.ts_unix_ns / 1000000000ull);
      uint64_t ms  = (lc.ts_unix_ns / 1000000ull) % 1000ull;
      struct tm tm;
      gmtime_r(&sec, &tm);
      std::strftime(ts, sizeof(ts), "%H:%M:%S", &tm);
      char line[64];
      std::snprintf(line, sizeof(line), "%s.%03llu  %s\n",
                    ts, static_cast<unsigned long long>(ms), lc.marker.c_str());
      out += line;
    }
    out += "```\n\n";
  }

  // Event counts
  out += "## Events\n\n";
  out += "| Category | Count |\n|---|---:|\n";
  out += "| span      | " + std::to_string(s.count_span.load())      + " |\n";
  out += "| metric    | " + std::to_string(s.count_metric.load())    + " |\n";
  out += "| audit     | " + std::to_string(s.count_audit.load())     + " |\n";
  out += "| lifecycle | " + std::to_string(s.count_lifecycle.load()) + " |\n";
  out += "| error     | " + std::to_string(s.count_error.load())     + " |\n";
  out += "| **total** | **" + std::to_string(s.seq.load())           + "** |\n\n";

  // Spans by total time
  if (!s.span_stats.empty()) {
    out += "## Spans by total time\n\n";
    out += "| Name | Count | Total | Mean | p50 | p95 | p99 | Max |\n";
    out += "|---|---:|---:|---:|---:|---:|---:|---:|\n";
    std::vector<std::pair<std::string, SpanStats>> rows(s.span_stats.begin(), s.span_stats.end());
    std::sort(rows.begin(), rows.end(),
              [](auto& a, auto& b) { return a.second.total_ns > b.second.total_ns; });
    for (auto& kv : rows) {
      auto& st = kv.second;
      uint64_t mean = st.count ? st.total_ns / st.count : 0;
      uint64_t p50 = percentile(st.samples, 50.0);
      uint64_t p95 = percentile(st.samples, 95.0);
      uint64_t p99 = percentile(st.samples, 99.0);
      out += "| `" + kv.first + "` | "
             + std::to_string(st.count) + " | "
             + format_duration_ns(st.total_ns) + " | "
             + format_duration_ns(mean) + " | "
             + format_duration_ns(p50) + " | "
             + format_duration_ns(p95) + " | "
             + format_duration_ns(p99) + " | "
             + format_duration_ns(st.max_ns) + " |\n";
    }
    out += "\n";
  }

  // Audit chain
  out += "## Audit\n\n";
  if (s.chain.count == 0) {
    out += "_No audit events recorded._\n\n";
  } else {
    out += "Recorded **" + std::to_string(s.chain.count) + "** audit event"
        + (s.chain.count == 1 ? "" : "s") + ", chained.\n\n";
    out += "**Chain head**: `" + s.chain.head_hex + "`\n\n";
    out += "Verify with `loom verify " + s.run_id + "`. The private file (`audit.jsonl`, mode 0600) holds full attributes; the public file (`audit.public.jsonl`) is safe to share.\n\n";
  }

  // Errors
  out += "## Errors\n\n";
  if (s.errors.empty()) {
    out += "_No errors observed._\n\n";
  } else {
    out += "| Time | Severity | Name | Message |\n|---|---|---|---|\n";
    for (auto& e : s.errors) {
      char ts[24];
      time_t sec = static_cast<time_t>(e.ts_unix_ns / 1000000000ull);
      struct tm tm;
      gmtime_r(&sec, &tm);
      std::strftime(ts, sizeof(ts), "%H:%M:%S", &tm);
      const char* sev = (e.severity == 0) ? "warn" :
                        (e.severity == 2) ? "fatal" : "error";
      std::string msg = e.message;
      // Truncate long messages for the table.
      if (msg.size() > 80) msg = msg.substr(0, 77) + "…";
      out += "| " + std::string(ts) + " | " + sev + " | `" + e.name + "` | " + msg + " |\n";
    }
    out += "\n";
  }

  out += "---\n\n";
  out += "_Full event stream in `events.jsonl` · run metadata in `manifest.json`._  \n";
  out += "_Generated by loom v" LOOM_VERSION_STRING "._\n";

  std::fputs(out.c_str(), f);
  std::fclose(f);
}

}  // namespace

void finalize_run(State& s) {
  uint64_t end_ns      = now_unix_ns();
  uint64_t duration_ms = (end_ns > s.start_unix_ns)
                         ? (end_ns - s.start_unix_ns) / 1000000ull
                         : 0;
  // Summary first (manifest references its size).
  write_summary(s, end_ns, duration_ms);
  write_manifest(s, end_ns, duration_ms);
}

}  // namespace loom::detail
