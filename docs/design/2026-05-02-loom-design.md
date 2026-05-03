# Loom — Design Spec

| Field | Value |
|---|---|
| Project | **Loom** — observability harness for local-compute AI systems |
| Spec date | 2026-05-02 |
| Spec status | Approved (brainstorming-phase sign-off; implementation plan pending) |
| Spec owner | Charles Mahoney (`cm6268@princeton.edu`) |
| Repo | `~/Desktop/loom/` (sibling to `WhitePaperBedrockV1/`) |
| License | Apache-2.0 |
| First consumer | Bedrock W4A16 inference runtime (this user's existing repo) |
| Future consumers | A multimodal Jetson SDK and small-team local-AI applications |

---

## 1. Motivation

Frontier AI subscriptions (OpenAI, Anthropic) operate at a structural loss on
the consumer end. The likely steady state is two-tier: enterprises pay for API
credits at full rate; consumers and small businesses live under aggressive rate
limits and run smaller models locally. On top of that, any business with
compliance obligations (healthcare, legal, finance, education) cannot send
inputs to a third-party cloud at all. The center of gravity for everyday AI
work moves to the device.

Local AI is operationally different from cloud AI. The model touches the user's
files. Tools execute locally. There is no per-request log shipped to a vendor
backend that an SRE can query when something fails. **The local machine has to
become a system that can be debugged, audited, and trusted, by the people
using it.**

Loom is the harness that makes that possible. It instruments local-AI workloads
to produce:

1. **Observability** — what the system did, when, with what latency, with what
   inputs and outputs, structured well enough that a developer or a model can
   reason about it.
2. **A compliance trail** — a tamper-evident record of model interactions with
   the local environment (file reads, tool calls, outputs delivered to the
   user) suitable for audit, with a redaction story that lets the operator
   share evidence without leaking content.
3. **A polished surface for humans** — the SDKs being instrumented are
   deliberately terse and agent-optimized; Loom is the opposite. Its source
   files, its CLI, its TUI, its `report.html`, and its README are designed to
   be read by people. It is the showroom that sits in front of the dark factory.

## 2. Aesthetic and design principles

Loom is paired with — and explicitly different from — the dark-factory style
of the SDKs it instruments. Two distinct registers:

- **Dark factory (Bedrock, future SDK):** compressed, agent-readable, fewest
  lines, comments only when they prevent rediscovering a trap. State is in
  `master.md` and `BEDROCK_CONTRACT.md`. Hostile to documentation drift. The
  audience is a coding agent.
- **Showroom (Loom):** expansive, human-readable, considered naming and shape.
  Source files have intentional structure. Comments explain *why* — intent,
  invariants, traps — never *what*. Documentation is the product. The audience
  is a small-team developer or operator.

Both registers share one rule: **comments and prose earn their keep or get
cut.** Verbosity in Loom lives in *naming, structure, and documentation*, not
in narrating obvious code.

Three principles for the user-facing surfaces:

1. **Document feel, not log dump.** Typographic hierarchy, generous whitespace,
   restrained color. Most logger UIs throw color at every severity level; Loom
   uses one accent color per category and lets typography do the work.
2. **Lifecycle as navigation, not noise.** Lifecycle markers are the spine of
   every UI: chapters in `report.html`, collapsible sections in the TUI,
   anchors in shareable URLs.
3. **One cognitive surface per panel.** Spans, metrics, audit, errors,
   lifecycle each get their own view. The user switches; the screen does not
   blur them together.

## 3. Architecture overview

Three components, three processes, one shared file region.

```
  ┌──────────────────────────┐
  │  Consumer (Bedrock /     │   process A
  │  multimodal SDK / app)   │   ─────────
  │                          │
  │  C++ embed lib           │   <300 ns hot path:
  │  - libloom.a (static)   │   atomic seq++,
  │  - loom.h (public API)   │   memcpy into ring slot
  │  - loom_hooks.h (no-op   │
  │    weak symbols, opt-in) │
  └───────────┬──────────────┘
              │  mmap'd MPSC ring
              │  /tmp/loom/<run-id>/ring
              │  + overflow.NNN spill files
              ▼
  ┌──────────────────────────┐
  │  loom-daemon             │   process B (Go)
  │  - drains ring           │   ─────────
  │  - applies redaction     │   one daemon per run-id
  │    pipeline              │   (not one per host)
  │  - writes artifacts      │
  │  - serves a local UDS    │
  │    for CLI subscribers   │   `unix:/tmp/loom/<run-id>/sock`
  └───────────┬──────────────┘
              │  artifacts on disk
              │  ~/.loom/runs/<run-id>/
              │    events.jsonl
              │    audit.jsonl       (0600)
              │    audit.public.jsonl
              │    metrics.parquet   (or .jsonl in v1)
              │    spans.jsonl
              │    summary.md
              │    report.html       (single file, embedded data)
              │    manifest.json     (chain heads, file hashes)
              ▼
  ┌──────────────────────────┐
  │  loom CLI                │   process C (Go) — `loom run | watch | view |
  │  - subprocess launcher   │     report | verify | redact | doctor`
  │  - TUI viewer            │
  │  - report renderer       │
  │  - verifier              │
  └──────────────────────────┘
```

### 3.1 Lifecycle of a run

1. `loom run -- <consumer-command>` allocates a `run-id` (ULID), creates
   `~/.loom/runs/<run-id>/`, mmaps the ring under `/tmp/loom/<run-id>/`, forks
   `loom-daemon` as a child with that run-id, then `exec`s the consumer
   command with `LOOM_RUN_ID` and `LOOM_RING_PATH` set in env.
2. The consumer's embed lib reads those env vars on `loom::init()` and attaches
   to the ring. If the env vars are absent, every `loom::*` call is a no-op.
   So a binary linked against `libloom.a` but launched outside `loom run`
   does no work and pays no cost beyond the unconditional symbol lookups
   resolved at link time.
3. The daemon drains the ring, applies the redaction pipeline, appends to the
   artifact files, and snapshots `manifest.json` every N seconds and on
   `SIGUSR1`.
4. On consumer exit, the CLI sends `SIGTERM` to the daemon. The daemon
   flushes, finalizes hash chains, writes `report.html`, and exits cleanly.
5. `loom view <run-id>` replays a finished run by reading the artifact dir
   directly. No daemon is involved.

### 3.2 Key design properties

- **The consumer never blocks on the daemon.** The decode loop's worst case is
  a ring-full spill, which is two `pwrite` syscalls into a pre-faulted file
  (single-digit µs). No IPC round-trip ever happens on a hot path.
- **Audit-sync semantics** are implemented as a per-event ack: when a caller
  opts into sync audit, the embed lib spins on a 64-bit `seq_acked` cell that
  the daemon updates after `fsync()`. Bounded wait, no signals.
- **Crash safety.** If the consumer crashes mid-run, the daemon finishes
  draining what is already in the ring (mmap'd memory survives consumer crash)
  and writes `crashed: true` into `manifest.json`. If the daemon crashes, a
  watchdog inside the CLI restarts it; the ring buffer survives across daemon
  restarts.
- **Single host only in v1.** No network sinks. The wire format reserves a
  `trace_id` field for cross-process trace propagation in a later version, but
  v1 does not implement it.

## 4. Wire format — `loom.event.v1`

The wire format has two skins: a fixed binary layout in the ring (for
producer-side speed), and canonical JSON Lines on disk (for consumers,
auditors, and tools). The daemon translates between them.

### 4.1 Ring binary record

Records are 64-byte aligned. Header is fixed-size; payload is variable
length, length-prefixed.

```
struct RingHeader {                  // 64 bytes
  uint64  seq;                       // monotonic per-process
  uint64  ts_ns;                     // CLOCK_MONOTONIC_RAW
  uint32  category;                  // 1=span 2=metric 3=error 4=audit 5=lifecycle
  uint32  payload_len;               // bytes following the header
  uint128 trace_id;                  // reserved, zero in v1
  uint64  span_id;                   // zero if not a span
  uint64  parent_span_id;            // zero at root
  uint64  flags;                     // bit 0: sync_ack_requested
                                     // bit 1: chain_continuation (audit only)
                                     // bits 2..63 reserved
};
```

Payload encoding is category-specific and opaque to the ring. The daemon
re-encodes payloads to canonical JSON for the on-disk artifacts.

### 4.2 Canonical JSON shape

One record per line; UTF-8; no trailing whitespace; `%.10g` for floats (matching
Bedrock's `bedrock.bench.v1` convention so tools that already parse `%.10g`
work unmodified).

```json
{"v":"loom.event.v1","cat":"span","seq":417,"ts":"2026-05-02T18:21:09.142Z",
 "name":"forward.layer","span_id":"01J3KTV6S5","parent":"01J3KTV6Q0",
 "dur_ns":8714203,"attrs":{"layer":8,"variant":"qkv_tri_fused"}}
{"v":"loom.event.v1","cat":"metric","seq":418,"ts":"...",
 "name":"tok_step_ms","value":8.71,"kind":"f64","attrs":{"step":42}}
{"v":"loom.event.v1","cat":"audit","seq":419,"ts":"...",
 "name":"file.read",
 "attrs":{"path":"/Users/charles/Docs/x.pdf","path@public":"<sha256:9f...>",
          "bytes":12384},
 "chain":{"prev":"<sha256:...>","this":"<sha256:...>"}}
{"v":"loom.event.v1","cat":"lifecycle","seq":420,"ts":"...",
 "name":"prompt.boundary","attrs":{"index":1}}
{"v":"loom.event.v1","cat":"error","seq":421,"ts":"...",
 "name":"cuda.launch_failed","message":"...","severity":"error",
 "attrs":{"kernel":"gemv_w4a16","code":700}}
```

### 4.3 Field-name annotation convention

Audit attribute keys may carry a single trailing annotation:

| Annotation | Meaning |
|---|---|
| (none) | Default policy applies. Default policy: hash-only in the public file. |
| `@public` | Force-include the value in `audit.public.jsonl` verbatim. |
| `@sensitive` | Force-hash even in the private file (used for things like raw passwords transiting the system). |
| `@redact` | Hash in private, omit entirely in public. |

Consumers populate both keys when they want a public-safe alias:
`attrs["path"] = "/full/path"; attrs["path@public"] = sha256("/full/path")`.
The redactor preserves only `@public`-tagged keys (and structural keys like
`name`, `ts`, `seq`) when generating the public file.

The annotation is part of the *key string* rather than a sidecar list because
it travels with the call site, is local to where the data is decided, and
cannot be silently dropped by a separate schema file.

### 4.4 Schema versioning

`loom.event.v1` is the wire schema. Any breaking change ships as `v2`; the
daemon supports the previous version for one minor-release deprecation window.
`manifest.json` records the schema versions used during the run.

## 5. Embed library — C++ public API

The library is a static archive `libloom.a` plus headers. C++17, no external
runtime dependencies, no exceptions across the API boundary.

### 5.1 `loom.h`

```cpp
// loom.h — public C++ API. C++17. No exceptions across the boundary.
#pragma once
#include <cstdint>
#include <initializer_list>
#include <string_view>

namespace loom {

// Attribute value type. Variant over the supported primitive types.
struct AttrValue {
  enum class Tag { I64, F64, Str, Bool, Bytes };
  Tag tag;
  union {
    int64_t i64;
    double  f64;
    bool    b;
    struct { const char* data; size_t len; } str;
    struct { const uint8_t* data; size_t len; } bytes;
  };
  // Constructors for each supported type.
  AttrValue(int64_t v);
  AttrValue(double v);
  AttrValue(bool v);
  AttrValue(std::string_view v);
  AttrValue(const uint8_t* p, size_t n);
};

struct Attr { std::string_view key; AttrValue value; };

// Initialization. Idempotent. Safe to call from any thread.
// If LOOM_RUN_ID env var is unset, all subsequent loom calls are no-ops.
// Returns 0 on success, negative errno-like code on failure.
int  init();
void shutdown();

// Whether the harness is active in this process. Cheap check; intended for
// callers that want to skip building expensive attribute payloads.
bool active() noexcept;

// ── Spans ────────────────────────────────────────────────────────────────
//
// RAII; nested by call stack. Span IDs are ULIDs assigned at construction.
struct Span {
  Span(std::string_view name) noexcept;
  Span(std::string_view name,
       std::initializer_list<Attr> attrs) noexcept;
  ~Span();                                         // emits span_end
  Span(const Span&) = delete;
  Span& operator=(const Span&) = delete;
  Span(Span&&) noexcept;
  Span& operator=(Span&&) noexcept;
  void annotate(std::string_view key, AttrValue value) noexcept;
};

// ── Metrics ──────────────────────────────────────────────────────────────
void metric_i64(std::string_view name, int64_t value) noexcept;
void metric_f64(std::string_view name, double  value) noexcept;
void counter_inc(std::string_view name, int64_t delta = 1) noexcept;

// ── Errors ───────────────────────────────────────────────────────────────
enum class Severity { Warn, Error, Fatal };
void error(std::string_view name,
           std::string_view message,
           Severity sev = Severity::Error,
           std::initializer_list<Attr> attrs = {}) noexcept;

// ── Audit ────────────────────────────────────────────────────────────────
//
// Audit events are sync by default: the call returns after the daemon has
// fsync'd the record. Set `async = true` to opt out (e.g., for high-frequency
// audit-flavored events that would dominate the budget).
struct AuditOptions { bool async = false; };
void audit(std::string_view name,
           std::initializer_list<Attr> attrs,
           AuditOptions opts = {}) noexcept;

// ── Lifecycle ────────────────────────────────────────────────────────────
void lifecycle(std::string_view marker,
               std::initializer_list<Attr> attrs = {}) noexcept;

}  // namespace loom
```

### 5.2 `loom_c.h` — C ABI mirror

A flat C interface mirroring the same six categories. Used by Python (ctypes),
Rust, Go (cgo), and any other FFI consumer. The C symbols *are* the canonical
implementation; the C++ wrappers in `loom.h` are thin inline shims that build
the attribute payload and dispatch to the C entry points. This keeps a single
source of truth for the call sequence and avoids drift between two
implementations.

### 5.3 `loom_hooks.h` — weak-symbol no-op interface

For consumers that cannot or should not unconditionally link Loom (notably
Bedrock, whose contract requires byte-identical default builds):

```c
// loom_hooks.h — weak-symbol stubs. No dependency on <loom.h>.
// When libloom.a is not linked, these resolve to no-op declarations with
// linker-provided null bodies on platforms that support weak symbols.
#ifdef __cplusplus
extern "C" {
#endif

__attribute__((weak)) void loom_hook_span_begin(const char* name);
__attribute__((weak)) void loom_hook_span_end  (const char* name);
__attribute__((weak)) void loom_hook_metric_f64(const char* name, double v);
__attribute__((weak)) void loom_hook_error     (const char* name,
                                                const char* message);
__attribute__((weak)) void loom_hook_audit     (const char* name,
                                                const char* attrs_json,
                                                int sync);
__attribute__((weak)) void loom_hook_lifecycle (const char* marker);

#ifdef __cplusplus
}
#endif
```

When `libloom.a` is linked, strong symbols override and forward into the full
embed lib. When it is not, the unreferenced weak symbols resolve to address
zero, and call sites must guard with a null check:

```cpp
if (loom_hook_span_begin) loom_hook_span_begin("forward.layer");
```

The null check compiles to a single load + conditional branch — branch-predicted
to "not taken" on the default-build path — and is what makes the off-path
guard test (§ 11) pass with ±0.5% tok/s. The guard predicate is wrapped in a
`LOOM_HOOK(...)` macro so call sites stay tidy.

### 5.4 Bindings

| Language | Path | Mechanism |
|---|---|---|
| Python  | `bindings/python/loom/` | ctypes against `loom_c.h`; thin Pythonic wrapper module |
| Go      | `bindings/go/loom/`      | cgo against `loom_c.h`; idiomatic `loom.Span(ctx, ...)` |
| Rust    | (deferred, post-v1)      | `bindgen` + safe wrapper crate |

## 6. Daemon — `loom-daemon`

Single Go binary. One process per run-id. Lifetime tied to the run.

### 6.1 Responsibilities

1. **Drain the ring.** MPSC consumer; lock-free read of head/tail; copy
   records into in-memory batch buffers.
2. **Read overflow spill files** in order if any exist.
3. **Apply the redaction pipeline** to audit records, producing both
   `audit.jsonl` (private, mode 0600) and `audit.public.jsonl` (public).
4. **Maintain hash chains** for both audit files. Each record carries
   `chain.prev` (SHA-256 of the previous record's serialized bytes) and
   `chain.this` (SHA-256 of the current record). The chain head is published
   in `manifest.json`.
5. **Append to the other artifact files** — `events.jsonl`, `spans.jsonl`,
   `metrics.jsonl` (Parquet deferred to v1.1).
6. **Refresh `manifest.json`** every N seconds and on `SIGUSR1`.
7. **Serve a local UDS** at `/tmp/loom/<run-id>/sock` so `loom watch`
   subscribers can stream events without re-tailing the file.
8. **Finalize** on `SIGTERM`: flush buffers, close hash chains, render
   `report.html` and `summary.md`, exit 0. Hard timeout 30 s; if exceeded,
   write a `manifest.json` flag `finalize_truncated: true` and exit anyway —
   we never wedge on shutdown.

### 6.2 Sub-packages (Go)

| Package | Responsibility |
|---|---|
| `internal/ring`     | mmap, lock-free MPSC reader, spill-file ordering |
| `internal/redact`   | pipeline of named redactors; deterministic, pure |
| `internal/sink`     | append-only file writers; per-category file ownership |
| `internal/chain`    | SHA-256 hash chain primitives; verification logic |
| `internal/uds`      | live subscription protocol (newline-delimited JSON over UDS) |
| `internal/manifest` | manifest snapshot, finalization |

### 6.3 Redaction pipeline

Configured via `loom.config.yaml` (§ 10). Each pipeline entry is a named
redactor. The pipeline is applied **only between the private record and the
public record**. The private record is always written verbatim (subject to
field-level `@sensitive` annotations).

Built-in redactors for v1:

| Name | Behavior |
|---|---|
| `path-prefix` | Replaces matching path prefixes with a hash-of-prefix sentinel. |
| `regex`       | Substitutes regex matches with a hash sentinel. |
| `entity:email` | Detects email addresses; replaces with hash. |
| `truncate`    | Truncates string values longer than N to a length-prefixed hash. |
| `drop`        | Removes a key entirely (for fields the operator never wants emitted, even hashed). |
| `keep`        | Whitelists a key for `@public` emission without an explicit annotation at the call site. |

Additional redactors can be supplied as user-provided Go plugin shared
objects, loaded by name. v1 ships built-ins only.

The pipeline is **deterministic and pure** — re-running it on the same private
file with the same config must produce the same public file. This is the
property `loom redact` uses to no-op detect.

## 7. CLI — `loom`

Single Go binary, separate from `loom-daemon`. Two binaries kept apart so the
CLI can be installed and run without daemon dependencies, and so the daemon
binary stays small for embedding in environments where only the daemon is
needed (e.g., a managed Jetson appliance).

### 7.1 Subcommand surface

| Command | Purpose |
|---|---|
| `loom run -- <cmd...>` | Allocate a run-id, start the daemon, exec the child. Echoes the run-id and artifact path before exec. Forwards exit code. `--quiet` suppresses the header. |
| `loom watch [--run ID \| latest]` | Live TUI. Default `latest`. Gracefully transitions to view mode when the run finishes. |
| `loom view <run-id>` | Static TUI for a finished run. Faster startup; no streaming subscription. |
| `loom report [<run-id>]` | Idempotent re-render of `report.html`. `--open` opens in the default browser. |
| `loom verify <run-id>` | Verifies hash chains. Exit codes: 0 verified, 2 chain broken, 3 file missing, 4 config mismatch. |
| `loom redact <run-id>` | Re-runs the redactor pipeline against the private file. No-op detected when the existing public file already matches the current config. |
| `loom doctor` | Environment diagnostic checklist (ring permissions, mmap support, CUPTI presence, config validity, terminal truecolor). |
| `loom version` | CLI, daemon, and wire-schema versions. Embed-lib version is reported when an explicit `--embed <path>` flag is given (since the embed lib is a static archive that is not loadable at runtime — its version is embedded as a symbol read by `objdump`/`nm`). Mismatch warnings inline. |

`loom attach` (connect to an already-running PID without launching it) is
deferred past v1.

### 7.2 TUI design (Bubble Tea + Lipgloss)

```
╭──── loom · run 01J3K…GZQ · running · 14.2s · ring 8% · spill 0 ────╮
│                                                                    │
│  Outline           ▌  Events   Spans   Metrics   Audit   Errors    │
│                    ▌                                               │
│  ▾ Setup           ▌  ┌──── span flamegraph ──────────────────┐    │
│    weights load    ▌  │ forward.step ████████████████ 8.7 ms  │    │
│    warmup          ▌  │  ├ qkv_tri_fused      ████  1.2 ms    │    │
│  ▾ Prompt 1        ▌  │  ├ attn               █████ 1.4 ms    │    │
│    tok 0           ▌  │  ├ ffn (gate+up)      ████  1.1 ms    │    │
│    tok 17 ◀ NOW    ▌  │  └ lm_head            ███   0.7 ms    │    │
│    tok 32          ▌  └────────────────────────────────────────┘   │
│  ▸ Decode          ▌                                               │
│  ▸ Teardown        ▌  ◇ tok/s 111.4   ◇ p99 8.93ms   ◇ jitter 0.7%│
│                    ▌                                               │
│ tab cycles panes · ↑↓ moves outline · / search · q quits           │
╰────────────────────────────────────────────────────────────────────╯
```

- **Outline rail (left).** Mirrors lifecycle events. Selecting an outline node
  filters every other panel to that time window. This is the primary
  interaction; it makes a 30k-event run actually browsable.
- **Tabbed main pane.** Five tabs, one per category. Default tab is `Spans`
  (flamegraph). Filters, search query, time-range cursor persist across tab
  switches.
- **Header bar.** Run id, status, elapsed, ring health, spill counter. Spill
  > 0 ever turns the counter orange immediately — it indicates the daemon is
  falling behind producers and the run's perf signal is degraded.
- **Footer.** Keybinds, *not* status. Status lives in the header.
- **Color palette.** One accent per category — span teal, metric indigo,
  audit amber, error red, lifecycle neutral grey. Severity within errors is
  encoded as shape (◯ warn, ⨯ error, 💥 fatal), not color, so the UI is
  legible to colorblind users.
- **Truecolor required.** Doctor checks for it; falls back to a 256-color
  palette with a one-time warning.

### 7.3 `report.html` design

- Single self-contained file. Embedded JSON data + ~20 KB vanilla JS for
  filtering. No CDN, no external requests. Survives e-mail attachment.
- **Cover page** — typographic title block: run id, model, host, duration,
  headline metrics. Reads like the front of a report.
- **Outline TOC** — same lifecycle hierarchy as the TUI, anchored. URL
  fragments survive: `report.html#prompt-1`.
- **Per-section content** — spans rendered as inline-SVG flamegraphs;
  metrics as inline-SVG sparklines; audit highlights as a clean table; errors
  as collapsible cards. Keeps the file small and screenshot-friendly.
- **In-browser verifier** — a built-in widget runs hash-chain verification in
  pure JavaScript over the embedded audit data. Shows ✓ or ⨯ at chain index N.
  This is the auditor experience: open the file, click verify, done.
- **Print styles** — separate `@media print` rules. Prints to PDF cleanly.
  Important for compliance archives.

## 8. Configuration — `loom.config.yaml`

Loaded from (in order, last wins):
1. `${XDG_CONFIG_HOME}/loom/config.yaml` (or `~/.config/loom/config.yaml`)
2. `<repo>/loom.config.yaml` (project-local)
3. `${LOOM_CONFIG}` env var (explicit path)

Schema is `loom.config.v1`. Example:

```yaml
schema: loom.config.v1
run:
  artifact_dir: ~/.loom/runs
  ring_bytes: 16777216          # 16 MiB
  daemon_path: loom-daemon       # discovered on PATH if unqualified
audit:
  sync_default: true
  hash_algorithm: sha256
redact:
  - keep: [model_id, layer, variant]
  - path-prefix: { prefix: "/Users/charles/", replacement: "<sha256-prefix>" }
  - entity:email
  - truncate: { max_len: 256, applies_to: [message, prompt, response] }
ui:
  theme: charcoal              # charcoal | parchment | high-contrast
  truecolor: required          # required | preferred | never
  default_tab: spans
```

Every field has a default, so a zero-config install runs.

### 8.1 Runtime environment variables

Loom honors a small set of env vars for ergonomics, sandboxing, and CI
isolation. They override the corresponding `loom.config.yaml` keys when set.

| Variable | Default | Purpose |
|---|---|---|
| `LOOM_RUN_ID` | unset | Set by `loom run` for the child process. The embed lib treats its absence as "no harness attached" and becomes no-op. Direct callers may set it manually for testing. |
| `LOOM_RING_PATH` | unset | Path to the mmap'd ring file. Set by `loom run`. |
| `LOOM_HOME` | `~/.loom` | Root of artifact storage. CI sets this to a temp dir for hermetic test runs. |
| `LOOM_TMPDIR` | `/tmp` | Root for transient ring files (`<LOOM_TMPDIR>/loom/<run-id>/ring`). |
| `LOOM_CONFIG` | unset | Explicit path to a `loom.config.yaml` overriding discovery. |

These variables are part of the public contract; renaming any of them is a
breaking change.

## 9. Performance budget

These are M5 design targets — what the ring-buffer + daemon path is
intended to deliver. **As of M3.5 they are not measured**; the
microbenchmark harness (`embed/bench/`) and the CI gate that pins them
on tier-A runners are M5 deliverables. The current `fwrite + fflush`
path (M2) is explicitly off this budget and the README discloses that.

| Event type             | Hot-path cost (target) | Path |
|---|---|---|
| `span` enter/exit      | < 300 ns | atomic `seq++`, 32-byte memcpy to ring slot |
| `metric` sample        | < 100 ns | single atomic update on counter cell |
| `error`                | < 2 µs   | formatted message, async to ring |
| `audit` (default sync) | < 5 ms p99 | event → ring → daemon → write + fsync → ack |
| `audit` (async opt-in) | < 500 ns | same as span but with hash-chain header |
| `lifecycle`            | irrelevant | infrequent |

### 9.1. Percentile and sampling algorithm (M2 / M3.5)

Span percentiles in `manifest.json` and `loom show` are computed
nearest-rank: `idx = floor(pct/100 * (n-1))`, no interpolation. With
small `n` this means p50 == p99 trivially (e.g. n=2 → p50=samples[0],
p99=samples[1]); the report does not currently warn on tiny samples.

Per-span samples are stored in a fixed-size buffer of `kMaxSpanSamples
= 4096` entries (`embed/src/state.h`). After the cap is hit, additional
samples for that span name are dropped from the percentile calculation
(FIFO truncation; the running `count` and `total_ns` continue to
include them, but percentiles are computed from the first 4096
samples). For Bedrock decode loops this will trigger; switching to
Algorithm-R reservoir sampling is a tracked follow-up.

Backpressure: ring overflow spills to a sequentially numbered file in the same
dir. Spill activity emits a `metric` (`loom_ring_spill_bytes`) so the TUI
ticker shows back-pressure as it happens. Drops are never silent.

## 10. Repo layout

```
loom/
├── README.md
├── LICENSE                        Apache-2.0
├── CHANGELOG.md
├── docs/
│   ├── design/
│   │   └── 2026-05-02-loom-design.md   ← this file
│   ├── api/embed.md               C++ + C + Python bindings reference
│   ├── api/wire.md                loom.event.v1 schema with examples
│   ├── compliance.md              audit chain, redaction policy, threat model
│   └── tutorials/
├── embed/                         C++17 library
│   ├── include/loom/loom.h
│   ├── include/loom/loom_hooks.h
│   ├── include/loom/loom_c.h
│   ├── src/{loom.cpp,ring.cpp,audit_chain.cpp,clock.cpp,format.cpp}
│   ├── tests/                     GoogleTest
│   └── bench/                     microbench against the budget table
├── daemon/                        Go module
│   ├── cmd/loom-daemon/main.go
│   └── internal/{ring,redact,sink,uds,chain,manifest}/
├── cli/                           Go module
│   ├── cmd/loom/main.go
│   └── internal/{run,tui,report,verify,doctor,latest}/
├── schema/
│   ├── loom.event.v1.json
│   ├── loom.config.v1.json
│   └── loom.manifest.v1.json
├── bindings/
│   ├── python/loom/
│   └── go/loom/
├── examples/
│   ├── cpp_minimal/
│   ├── python_minimal/
│   └── bedrock_integration/
├── tools/
│   └── replay/                    deterministic event replay for tests
├── go.work                        workspace pulls daemon + cli into one build
├── CMakeLists.txt
└── .github/workflows/{ci.yml,release.yml,goreleaser.yml}
```

## 11. Bedrock integration plan

Loom's first consumer is the Bedrock W4A16 inference runtime. Bedrock has
strict surface rules; the integration must respect them. All edits are gated
by `LOOM=1`.

1. **`BEDROCK_CONTRACT.md` § 2 amendment.** Add a new editable category:

   > **Observation hooks**: weak-symbol no-op declarations in
   > `libBedrock/include/bedrock/loom_hooks.h`, ≤ 20 call sites in editable
   > runtime files. Default-build behavior (no `LOOM=1`) must be byte-identical
   > and within ±0.5% tok/s. Pinned by `tools/test_loom_hooks_offpath.py`
   > (Ring 2 guard).

2. **New header** `libBedrock/include/bedrock/loom_hooks.h` — six
   `extern "C" __attribute__((weak))` no-op symbols matching
   `loom::span_begin/end`, `metric`, `error`, `audit`, `lifecycle`. Standalone
   — no dependency on `<loom.h>`. The header lives in Bedrock; the strong
   symbols live in Loom.

3. **`libBedrock/src/forward_w4a16.cu`** — six call sites:
   - per-layer span (`forward.layer` with `layer_id`, `variant` attrs)
   - attention region span
   - GEMV variant tag (metric `loom_gemv_variant_used`)
   - per-step metric (`tok_step_ms`)
   - KV state metric (`kv_bytes`)
   - error path on CUDA failures

4. **`bedrock_bench/bench.cpp`** — four call sites, all under the line ceiling:
   - `lifecycle("run.start")` with weights / git-sha / device attrs
   - `lifecycle("prompt.boundary")`
   - `lifecycle("decode.start"/"decode.end")`
   - `audit("bench.finished")` with output hash, tok/s, deterministic-hash
     status — sync mode (durable on crash)

5. **`tools/test_loom_hooks_offpath.py`** — Ring 2 guard. Builds the bench
   twice, runs both with the deterministic seed, asserts byte-identical
   `phase4_tokens.json` and `phase4_e2e.json` hashes, and tok/s within ±0.5%.
   This is the test that satisfies Failure Modes F + I.

6. **`LOOM=1` env var** — read by `bench.cpp` to set `LOOM_RUN_ID` /
   `LOOM_RING_PATH` propagation; otherwise hooks remain no-op. Real reader
   ⇒ Failure Mode F satisfied.

7. **`master.md` § 3** — file list updated for the new header and test.

The actual hook *implementations* live in Loom's repo, not Bedrock's. Bedrock
ships the no-op declarations only; the strong symbols arrive when Loom is
linked.

## 12. Testing strategy

| Layer | Approach |
|---|---|
| Embed lib unit | GoogleTest. Ring SPSC/MPSC correctness, audit-chain hash continuity, attribute formatting. |
| Embed lib bench | Microbench validates § 9 budget table; CI fails if span enter/exit > 400 ns on a tier-A runner (defined in § 18). Numbers regress-tracked. |
| Daemon unit | `go test`. Replay deterministic ring fixtures from `tools/replay/`, diff against golden artifact dirs. |
| CLI/TUI | ANSI golden-file tests via `teatest`. Snapshot diff on each frame. |
| `report.html` | Render against fixture run dirs, snapshot DOM (Playwright headless), validate hash-chain widget. |
| End-to-end | `examples/cpp_minimal` and `examples/bedrock_integration` exercised in CI; verify hash chain + schema + lifecycle outline ordering. |
| Bedrock off-path | `tools/test_loom_hooks_offpath.py` (in Bedrock repo) — defensive contract test. |
| Property-based | `rapid` for ring concurrency invariants; redaction-pipeline equivalence (re-redacting a public file is identity). |

## 13. Build and release

- **Embed lib:** CMake, C++17, position-independent static archive
  `libloom.a` plus headers. Optional shared `libloom.so`. Cross-compiled to
  `linux/aarch64` for Jetson via toolchain file.
- **Daemon and CLI:** Go 1.21, `CGO_ENABLED=0` so binaries are fully static.
  Cross-compile matrix `{darwin, linux} × {amd64, arm64}` via `goreleaser`.
- **Release:** GitHub Releases — tarballs per platform, SHA-256SUMS, SBOM
  (Syft), reproducible-ish build flags. Versioned independently of wire
  schema.
- **Compatibility window:** daemon supports the current and previous wire
  versions; embed lib emits the current. Mismatch is logged and telemetered,
  not fatal.

## 14. Platform notes

- **Weak-symbol availability.** Linux + macOS + Jetson (Linux/aarch64) all
  support `__attribute__((weak))`. Windows is out of scope for v1; if Windows
  support is added later, the hook header switches to a function-pointer
  table populated at `loom::init()`.
- **`CLOCK_MONOTONIC_RAW`** is used for timestamps to avoid NTP slew on
  long-running daemons.
- **macOS `mmap` semantics** require `MAP_SHARED` for cross-process visibility
  of the ring; same as Linux. Tested on both.
- **CUDA 11.4 stderr-pipe stall** (Bedrock failure mode H): Loom's daemon
  attaches via UDS, not stdout/stderr capture, specifically to avoid this
  trap. The CLI does capture child stdout/stderr but only for echoing to its
  own terminal — the SDK-emitted *events* go through the ring, which is
  pipe-free.

## 15. Versioning and compatibility

- **Loom binary versions:** SemVer.
- **Wire schema versions:** independent stream — `loom.event.v1`,
  `loom.config.v1`, `loom.manifest.v1`. A breaking schema change increments
  the schema number and runs through one minor-release deprecation window.
- **API stability:** the C++ public API in `loom.h` and the C ABI in
  `loom_c.h` follow SemVer like the binary. Breaking C++ changes are
  annotated in `CHANGELOG.md`.

## 16. Out of scope (explicit non-goals for v1)

Listed so the implementation plan does not silently re-import them:

- Network sinks (OTLP export, remote backends).
- Cross-process trace propagation (the `trace_id` field is reserved, but no
  propagation logic ships).
- Sampling policies. Every event emitted is recorded.
- Encryption-at-rest *inside* Loom. Delegate to FileVault / LUKS / dm-crypt.
- Distributed multi-host deployments.
- Windows host support.
- A graphical dashboard. The TUI and `report.html` are the surfaces.
- Live editing of the redaction config during a run. Config is read once at
  daemon start.
- Ingest from non-Loom sources. (One-way Bedrock telemetry merge is allowed
  as a tool, not a daemon feature.)

## 17. Implementation milestones (for the plan)

The implementation plan should sequence work into milestones the user can
accept and ship one at a time:

| Milestone | Scope | Acceptance |
|---|---|---|
| **M1 — Skeleton** | Repo layout, CMake, go.work, CI scaffolding, README, LICENSE. Empty embed lib + daemon + CLI that compile and run a no-op `loom run -- echo hi`. | Builds clean on macOS and Linux; CI green; `loom run -- echo hi` exits 0 and creates an empty run dir. |
| **M2 — Embed lib core** | Ring buffer, span/metric/error/lifecycle categories, embed lib API, C ABI mirror. Daemon drains the ring and writes `events.jsonl`. | `examples/cpp_minimal` produces a well-formed `events.jsonl`. Microbench passes the § 9 budget. |
| **M3 — Audit + redaction + chain** | Audit category, hash chain on `audit.jsonl`, redaction pipeline producing `audit.public.jsonl`, `loom verify`. | Hash chain verifies; tampered file detected; `loom redact` is idempotent. |
| **M4 — TUI** | `loom watch` + `loom view` with all five panels and the lifecycle outline rail. | Golden-file ANSI snapshots stable; manual eval passes the design checklist. |
| **M5 — `report.html`** | Single-file renderer with embedded data, sparklines, flamegraph SVG, and in-browser verifier. | Renders cleanly in Chrome, Safari, Firefox; verifier widget functional; print-to-PDF clean. |
| **M6 — Bedrock integration** | `loom_hooks.h` in Bedrock; hook call sites in `forward_w4a16.cu` and `bench.cpp`; off-path guard test; contract amendment. | `LOOM=1 ./run bench …` produces a fully populated run dir; default `./run bench …` is byte-identical to baseline (guard test green). |
| **M7 — Bindings + polish** | Python ctypes wrapper, Go cgo wrapper, README polish, `loom doctor`, release pipeline. | `pip install loom` from the local wheel works; `goreleaser` produces all four platform binaries. |

## 18. Open items deferred to plan-writing

- **Tier-A runner spec for CI.** Tier-A is the runner class against which the
  § 9 performance budget is enforced as a hard CI gate. Proposal: GitHub-hosted
  `ubuntu-22.04` (x86_64) and `macos-14` (Apple Silicon). Linux/arm64 (Jetson)
  is **tier-B**: bench numbers reported but not gated, until a self-hosted
  arm64 runner exists. The plan should pin exact runner labels and a
  `BUDGET_FACTOR` for tier-B.
- Exact wire-format binary endianness convention (proposal: little-endian on
  disk, native in-memory; document explicitly in `wire.md`).
- Whether `metrics.parquet` ships in v1 or stays JSONL (proposal: JSONL in
  v1; Parquet in v1.1 once a Go writer with no native deps is selected).
- ULID generator implementation (proposal: pure-Go in daemon/CLI; Bryan
  Ford's Go ulid lib; embed lib uses a 128-bit time + counter scheme without
  a crypto dep).
