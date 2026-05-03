# Loom

> Tamper-evident observability and audit for local-compute AI.

Loom is the harness you wrap around a local-AI workload to make it
observable, auditable, and defensible on the device that runs it. Run
something under it and you get a self-describing artifact bundle —
every event with a value, every span with a duration, a SHA-256
hash-chained audit trail, reproducibility metadata (model, weights,
kernel, seed, sampling state), and a human-readable run summary.

```
$ LOOM_MODEL_ID=tinyllama-1.1b LOOM_PROMPT_VERSION=rag-v3 \
    loom run -- ./bedrock_bench weights/ runs/iter1/
loom · run 01J3KTV6S5… · artifacts ~/.loom/runs/01J3KTV6S5…

$ loom
ID            STATUS  AGE  DURATION  EVENTS  AUDIT  ERRORS  COMMAND
───────────────────────────────────────────────────────────────────
01J3KTV6…GZQ  ✓ ok     2s     8.7 s     312      4       0  bedrock_bench
01J3KTRX…ABC  ✓ ok    3m      8.6 s     308      4       0  bedrock_bench

$ loom show latest
Run 01J3KTV6S5…GZQ   COMPLETED   8.7 s
STARTED  May 2, 2026 · 22:28:41 UTC
COMMAND  ./bedrock_bench weights/ runs/iter1/
MODEL    tinyllama-1.1b
PROMPT   rag-v3
…

$ loom verify latest
verify: ✓ 4 audit records, head a9060cce…7798

$ loom report latest
report: ✓ ~/.loom/runs/01J3KTV6S5…/report.html
```

Built for the workload under regulatory eyes — EU AI Act Article 12,
the proposed HIPAA Security Rule, attorney-client privilege, ISO/IEC
42001, SOX, MiFID — where an auditor or opposing counsel will
eventually ask *"what came out of this model on this date for this
user?"* and the answer cannot be a SaaS trace shipped to a vendor
outside the operator's trust boundary. LangSmith, Helicone, Langfuse,
and Phoenix all assume a cloud HTTP boundary you control; AWS
Bedrock-in-VPC shifts liability via BAA but leaves the cloud inside
the trust boundary. Loom assumes the model is running in your process
on your hardware, and the record has to survive an integrity challenge
from an auditor or opposing counsel — hash-chained, tamper-evident
events the operator owns and retains for as long as the regime
requires. See
[`docs/intent.md`](docs/intent.md) for use cases and
[`docs/market-thesis.md`](docs/market-thesis.md) for why the category
exists.

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

## Auditor handoff

When the operator needs to give an auditor (or opposing counsel, or a
notified body) the artifact for a specific run, `loom export` bundles
the run dir into a signed `.tar.gz`. The signature is Ed25519 over a
digest manifest of every file in the bundle, so an auditor with the
operator's public key can confirm the bundle came from the claimed
key and was not modified post-export.

```
$ loom keygen --out ./loom-sign.key
keygen: ✓ wrote ./loom-sign.key (private, 0600) + ./loom-sign.key.pub (public)
        pubkey-sha256: 13aa0aae7f5e0d18777cf0ba16f1a94e00b0a0068ff3fbe652136dba8a1b0b27
        record the fingerprint above; auditors pass it as `loom verify --pubkey ...`.

$ LOOM_STRICT_REPRO=1 loom run -- ./bedrock_bench …      # refuses w/o LOOM_MODEL_HASH etc

$ loom export latest --out audit-2026Q2.tar.gz --key ./loom-sign.key
export: ✓ audit-2026Q2.tar.gz · 5 files  signed pubkey-sha256=13aa0aae…0b27

# auditor side, after receiving the tarball + the operator's public key
# through a trusted channel:
$ tar xzf audit-2026Q2.tar.gz -C ./extracted
$ loom verify ./extracted --pubkey ./loom-sign.key.pub
verify: ✓ 4 audit records, head a9060cce…7798
verify: ✓ 01J3KTV6S5… · digest re-checked, signature valid (pubkey-sha256 13aa0aae…0b27)
```

The signature anchors integrity to a key the operator holds. An
attacker who modifies the tarball after export — even one who
coherently rewrites every artifact, the hash chain, the manifest's
integrity block, and `digest.txt` — cannot produce a valid signature
without the private key. `verify --pubkey` returns exit code 2 on any
detected tampering. `verify --public` runs structural-chain
verification against `audit.public.jsonl` for regulators who never
need to see the unredacted private file.

## Status

| Milestone | Scope                                                          | Status |
|-----------|----------------------------------------------------------------|--------|
| M1        | Skeleton: repo, public API, CLI/daemon stubs, CI               | ✓      |
| M2        | Rich event payloads, manifest, summary, audit chain, `verify`  | ✓      |
| M3        | Field-level `@public` redaction, single-file `report.html`, integrity-hash `verify` | ✓ |
| M3.5      | Operator surface: `loom`/`ls`/`show`/`doctor`, reproducibility metadata | ✓ |
| M3.6      | `loom export` + `loom keygen` (Ed25519-signed audit bundles), `verify --pubkey`/`--public`, `LOOM_STRICT_REPRO`, fail-loud in-browser verifier | ✓ |
| M4        | Designed TUI (`loom watch`, `loom view`)                       | —      |
| M5        | Ring buffer + daemon (target: < 300 ns/span, unmeasured)       | —      |
| M6        | Bedrock W4A16 inference-runtime integration                    | partial |
| M7        | Python / Go bindings, release pipeline                         | —      |

The current cut writes one event per `fwrite + fflush` under a single
mutex. Suitable for debugging, integration testing, and any workload
not running on Jetson's hot decode loop. The < 300 ns/span budget is a
design target for M5 (ring buffer + drain daemon); it is not measured
in this release and there is not yet a benchmark harness in the repo.
Per-span samples are also capped at 4096 per name (FIFO truncation),
so percentiles on long-running spans reflect the first 4096 samples;
this is an M-something fix and is documented for honesty until then.

See [`docs/intent.md`](docs/intent.md) for the use cases (healthcare,
legal, finance, research, agent-on-files) driving every design
decision, and [`docs/market-thesis.md`](docs/market-thesis.md) for the
backing brief on why this category exists and who buys it.

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
