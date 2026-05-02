# Loom

> Observability harness for local-compute AI systems.

Loom is the harness you wrap around a local-AI workload — an inference
runtime, an agent, a small-team app — to make it observable, auditable,
and trustworthy on the device that runs it.

It is the deliberate counterpoint to the dark-factory style of the SDKs it
instruments. The SDKs are terse, agent-readable, optimized for the model.
Loom is the room people sit in: a polished C++ embed library, a tiny Go
daemon, a designed terminal UI, and a single-file HTML report you can hand
to an auditor.

## Status

Loom is under active development. Today's checkpoint:

| Milestone | Scope                                                         | Status |
|-----------|---------------------------------------------------------------|--------|
| M1        | Skeleton: repo, public API headers, CLI/daemon stubs, CI      | ✓      |
| M2        | Ring buffer, embed-lib bodies, daemon drain, `events.jsonl`   | next   |
| M3        | Audit + redaction + hash chain, `loom verify`                 | —      |
| M4        | Designed TUI (`loom watch`, `loom view`)                      | —      |
| M5        | Single-file `report.html` with in-browser chain verifier      | —      |
| M6        | Bedrock W4A16 inference-runtime integration                   | —      |
| M7        | Python / Go bindings, `loom doctor`, release pipeline         | —      |

## Quickstart

Requires CMake ≥ 3.20, a C++17 compiler, and Go 1.21.

```bash
git clone https://github.com/charlespers/loom.git
cd loom
make build
./build/loom version
./build/loom run -- echo "hello from a future-loom-instrumented process"
```

## Design

The full design is in [`docs/design/2026-05-02-loom-design.md`](docs/design/2026-05-02-loom-design.md).
Highlights:

- Three components, three processes, one mmap'd shared region.
- Embed lib hot path: < 300 ns per `span` enter/exit, no allocation.
- Audit events are sync-by-default and hash-chained; tampering breaks the chain.
- Two parallel artifacts per run — `audit.jsonl` (private, mode 0600) and
  `audit.public.jsonl` (redacted, safe to share).
- Single-host v1; cross-process trace propagation reserved for v2.

## License

Apache-2.0. See [`LICENSE`](LICENSE).
