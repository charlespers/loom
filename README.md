# Loom

> Observability harness for local-compute AI systems.

Loom is the harness you wrap around a local-AI workload to make it
observable, auditable, and trustworthy on the device that runs it. Run
something under it and you get a self-describing artifact bundle — every
event with a value, every span with a duration, a tamper-evident audit
trail, and a human-readable run summary.

```
$ loom run -- ./bedrock_bench weights/ runs/iter1/
loom · run 01J3KTV6S5… · artifacts ~/.loom/runs/01J3KTV6S5…

  ~/.loom/runs/01J3KTV6S5…/
  ├── manifest.json        run metadata: process, host, counts, files, chain head
  ├── events.jsonl         every event with v / cat / seq / ts / name / attrs
  ├── audit.jsonl          private compliance trail (0600), SHA-256 hash chained
  ├── audit.public.jsonl   redacted-attrs companion, safe to share
  └── summary.md           designed per-run report (cover, lifecycle, spans, errors)

$ loom verify latest
verify: ✓ 14 audit records, head a9060cce…7798
```

Loom is the deliberate counterpoint to the dark-factory style of the SDKs
it instruments. The SDKs are terse, agent-readable, optimized for the
model. Loom is the room people sit in: a polished C++ embed library, a
small Go CLI, a designed run summary, a hash-chained audit trail.

## What you get per run

| Artifact | What it answers |
|---|---|
| `manifest.json` | What was this run? — pid, command, argv, cwd, host, OS, arch, kernel, started/ended, duration, exit status, event counts by category, per-name span stats with p50/p95/p99, audit chain head, file sizes |
| `events.jsonl`  | What did the system do? — every event, schema-tagged `loom.event.v1`, with categories `span` / `metric` / `audit` / `lifecycle` / `error` |
| `audit.jsonl`   | What did the model touch? — private (mode 0600), full attrs, SHA-256 hash chained per record |
| `audit.public.jsonl` | What can I show an auditor? — same chain, attrs stripped to `{}` (M3 introduces field-level `@public` for selective disclosure) |
| `summary.md`    | What happened? — designed Markdown report: cover, lifecycle outline, event count, top spans by total time, audit highlights, error log |

## Status

| Milestone | Scope                                                          | Status |
|-----------|----------------------------------------------------------------|--------|
| M1        | Skeleton: repo, public API, CLI/daemon stubs, CI               | ✓      |
| M2        | Rich event payloads, manifest, summary, audit chain, `verify`  | ✓      |
| M3        | Field-level redaction, `report.html` single-file artifact      | next   |
| M4        | Designed TUI (`loom watch`, `loom view`)                       | —      |
| M5        | Ring buffer + daemon (decode-loop hot path < 300 ns)           | —      |
| M6        | Bedrock W4A16 inference-runtime integration                    | partial |
| M7        | Python / Go bindings, `loom doctor`, release pipeline          | —      |

The M2-partial cut writes one event per `fwrite + fflush`. Suitable for
debugging and integration testing. The decode-loop hot-path budget in the
[design spec § 9](docs/design/2026-05-02-loom-design.md) lands with M5
once the ring buffer + drain daemon ship.

## Quickstart

Requires CMake ≥ 3.20, a C++17 compiler, and Go 1.21.

```bash
git clone https://github.com/charlespers/loom.git
cd loom
make build

# Drive a workload under the harness
LOOM_HOME=$(mktemp -d) ./build/loom run -- ./build/examples/cpp_minimal/cpp_minimal

# What just happened?
RUN=$(find "$LOOM_HOME/runs" -mindepth 1 -maxdepth 1 -type d | head -1)
cat "$RUN/summary.md"

# Tamper-evidence
./build/loom verify "$RUN"
```

## Embedding it

C++17 — drop the static archive in and call `loom::*` from anywhere:

```cpp
#include <loom/loom.h>

void forward_layer(int layer) {
  loom::Span s("forward.layer", {{"layer", layer}});
  loom::metric_f64("tok_step_ms", 8.71, {{"step", layer}});
  // ... work ...
  // RAII span: dur_ns is filled in at scope exit.
}

void on_file_read(std::string_view path, int64_t bytes) {
  loom::audit("file.read", {{"path", path}, {"bytes", bytes}});
  // Sync by default — the call returns after fsync.
}
```

For consumers that can't or shouldn't link libloom unconditionally
(notably Bedrock, whose contract requires byte-identical default builds),
include the standalone weak-symbol header instead:

```cpp
#include <bedrock/loom_hooks.h>

LOOM_HOOK(loom_hook_span_begin,   "forward.layer");
LOOM_HOOK(loom_hook_metric_f64,   "tok_step_ms", 8.71);
LOOM_HOOK(loom_hook_lifecycle,    "forward.step.start");
```

When libloom is linked, the strong symbols override and observe; when
it's not, calls compile to a single null-checked branch — `±0.5 %` of
the no-loom build.

## Design

The full design is in [`docs/design/2026-05-02-loom-design.md`](docs/design/2026-05-02-loom-design.md).
Highlights:

- Three components, three processes, one mmap'd shared region (M5).
- Audit-by-default sync semantics; hash-chained, tamper-evident.
- Two parallel artifacts per run — private (mode 0600) and redacted
  public — both share the same SHA-256 chain.
- `loom.event.v1` wire format on disk; `loom.manifest.v1` for run
  metadata. Schema versions evolve independently.

## License

Apache-2.0. See [`LICENSE`](LICENSE).
