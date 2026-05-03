// embed/src/state.h — shared internal state for the Loom embed library.
//
// One State instance lives for the life of the process. Created lazily on
// first access; populated by loom_init() when LOOM_RUN_ID is set. The state
// holds the open file handles, the seq counter, the active-span table for
// pairing begin/end with durations, the per-name span stats for the run
// summary, the audit hash-chain head, and the run metadata captured at
// init for the manifest.
#pragma once

#include <atomic>
#include <cstdint>
#include <cstdio>
#include <mutex>
#include <string>
#include <unordered_map>
#include <vector>

namespace loom::detail {

// Per-active-span record kept in a global table keyed by handle (== span_id).
// Allocated at span_begin, removed at span_end after duration is computed.
struct SpanRecord {
  uint64_t    start_ns;
  uint64_t    parent_id;
  std::string name;
  std::string attrs_json;   // pre-encoded JSON object string for the begin-event attrs
};

// Per-span-name aggregated stats, written into manifest.spans + summary.md.
// We keep a sample reservoir (capped) to compute percentiles without holding
// every duration sample for a long-running process.
struct SpanStats {
  uint64_t count    = 0;
  uint64_t total_ns = 0;
  uint64_t min_ns   = UINT64_MAX;
  uint64_t max_ns   = 0;
  std::vector<uint64_t> samples;   // capped at kMaxSpanSamples
};
constexpr size_t kMaxSpanSamples = 4096;

// Recorded errors — included verbatim in summary.md so failures aren't
// hidden behind events.jsonl scrolling.
struct ErrorRecord {
  uint64_t    ts_unix_ns;
  std::string name;
  std::string message;
  int         severity;
  std::string attrs_json;
};

// Recorded lifecycle markers — used as the spine of summary.md.
struct LifecycleRecord {
  uint64_t    ts_unix_ns;
  std::string marker;
  std::string attrs_json;
};

// Audit chain head; the on-disk records carry their own prev/this hashes.
struct AuditChainState {
  std::string head_hex;     // sha256 of last record (empty before any audit)
  uint64_t    count = 0;
};

struct State {
  // ── Run identity ─────────────────────────────────────────────────────
  std::string run_id;
  std::string artifact_dir;          // ${LOOM_HOME}/runs/${run_id}
  uint64_t    start_unix_ns = 0;
  uint64_t    pid           = 0;
  uint64_t    ppid          = 0;
  std::string command;               // best-effort proctitle / argv0
  std::vector<std::string> argv;
  std::string cwd;
  std::string executable;
  std::string hostname;
  std::string os_name;
  std::string os_arch;
  std::string kernel_release;

  // ── Reproducibility metadata (env-supplied at init) ──────────────────
  // Keys read from LOOM_MODEL_ID, LOOM_MODEL_HASH, LOOM_PROMPT_VERSION,
  // LOOM_SEED, LOOM_RUN_TAG. Captured once at init; landed verbatim in
  // manifest.json so a regulator can replay a decision later.
  std::string repro_model_id;
  std::string repro_model_hash;
  std::string repro_prompt_version;
  std::string repro_seed;
  std::string repro_tag;

  // ── Open files ───────────────────────────────────────────────────────
  std::FILE* events_file        = nullptr;
  std::FILE* audit_file         = nullptr;
  std::FILE* audit_public_file  = nullptr;
  std::mutex write_mutex;            // serializes formatting & file writes

  // ── Activation flags & counters ──────────────────────────────────────
  std::atomic<bool>     active{false};
  std::atomic<uint64_t> seq{0};
  std::atomic<uint64_t> next_span_id{1};
  std::atomic<uint64_t> count_span{0};
  std::atomic<uint64_t> count_device_span{0};
  std::atomic<uint64_t> count_metric{0};
  std::atomic<uint64_t> count_audit{0};
  std::atomic<uint64_t> count_lifecycle{0};
  std::atomic<uint64_t> count_error{0};

  // ── Active-span table for pairing begin/end ──────────────────────────
  std::mutex spans_mutex;
  std::unordered_map<uint64_t, SpanRecord> active_spans;

  // ── Per-name span stats for summary.md and manifest.json ─────────────
  // Two parallel tables: CPU spans (cat="span", measured by RAII or
  // explicit loom_span_emit) and device spans (cat="device_span",
  // host-resolved from cudaEvent / equivalent). Same lock covers both
  // since the pairs of events that update them are independent.
  std::mutex span_stats_mutex;
  std::unordered_map<std::string, SpanStats> span_stats;
  std::unordered_map<std::string, SpanStats> device_span_stats;

  // ── Errors & lifecycle, kept in memory for summary.md ────────────────
  std::mutex errors_mutex;
  std::vector<ErrorRecord> errors;
  std::mutex lifecycle_mutex;
  std::vector<LifecycleRecord> lifecycles;

  // ── Audit hash chain ─────────────────────────────────────────────────
  std::mutex chain_mutex;
  AuditChainState chain;

  // ── Shutdown reentrancy guard ────────────────────────────────────────
  std::atomic<bool> shutdown_done{false};
};

// Lazy singleton. Constructed on first call; lives for the process. Note
// that the destructor of inline globals runs at process exit AFTER our
// __attribute__((destructor)), which is why finalization is explicit and
// idempotent rather than RAII on this struct.
State& state();

// Thread-local span stack used for parent_span_id lookup. A span's parent
// is the most recent span_begin on the same thread that has not yet had
// span_end called. Cross-thread parents would require explicit linkage,
// which is the M2-full + tracecontext story.
std::vector<uint64_t>& thread_span_stack();

}  // namespace loom::detail
