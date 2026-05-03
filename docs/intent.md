# Loom — Intent and use cases

Loom is observability for **local-compute AI** — workloads where the model
runs on the operator's own device, touches their files, and must be
defensible if a regulator or auditor asks how it behaved. This document is
the source of truth for *why* features exist; pin it before adding new
surface area.

## The core thesis

Loom exists because regulators are now writing the spec. EU AI Act
**Article 12** — enforceable on high-risk systems Aug 2 2026 unless
the Digital Omnibus deferral lands first — requires that any high-risk
AI system "shall technically allow for the automatic recording of
events (logs) over the lifetime of the system." Manual logging does
not satisfy the rule. Logs must be retained at least six months and
must support post-market monitoring under Article 72.

That paragraph is the product spec for an entire category, and it does
not describe what existing LLM observability tools produce.

Loom delivers the per-run artifact layer of that spec — every
inference produces a verifiable record. Retention, cross-run
aggregation, and Article 72 post-market reporting are operator policy
layered on top of those records; Loom captures the artifact and
authenticates it, the deployment owns the lifecycle.

Around that mandate, three forces converge:

1. **Compliance gravity.** Article 12 is the headline. Around it: the
   Jan 2025 HIPAA Security Rule rewrite naming AI software in mandatory
   technology asset inventory; ABA Formal Opinion 512 (Jul 2024)
   treating consumer LLMs as third-party disclosure that can waive
   attorney-client privilege; ISO/IEC 42001 (Dec 2023) certifiable
   today; NIST AI RMF crosswalks; SOX, MiFID, FERPA, GLBA, GDPR. None
   of these can be satisfied by sending the model's inputs to a third
   party.
2. **Capability shift to open weights.** Llama 3.1 405B reaches ~87.3%
   MMLU vs GPT-4 Turbo's ~86.5%; Llama crossed 1.2B downloads in Apr
   2025; >50% of the Fortune 500 was piloting it as of Mar 2025.
   Concrete deployments are real: JPMorgan's LLM Suite (200K
   employees), Goldman's GS AI Assistant, the DoD's GenAI.mil at IL5
   (3M+ personnel; expanded to IL6/IL7 May 2026), Apple Private Cloud
   Compute. The model lives where the data already does because the
   operator can now afford to host it.
3. **Agent-on-files reality.** The most useful AI work — agents
   reading the user's filesystem, drafting on their behalf, writing
   back — demands an audit trail the user *owns*. Not one streamed to
   a vendor they can't subpoena, not a Prometheus dashboard, not
   OpenTelemetry spans. A record the operator can hand to opposing
   counsel under Federal Rules of Evidence 901/902.

Local AI is operationally different from cloud AI. There is no SRE
backend to query when something fails, no shared S3 bucket of run logs
that legal can attach to a discovery request, no one to call when a
regulator wants to know exactly which inputs produced a flagged output.
The local machine itself has to become the system of record.

Loom is what makes that possible.

> Cost is *not* the wedge. Hosted token prices have fallen ~280× since
> Nov 2022 ([Epoch AI][epoch]), and hyperscaler "private LLM" services
> (AWS Bedrock with BAA, Azure OpenAI with Private Link) have HIPAA
> coverage and FedRAMP. The on-prem-or-die argument is sovereignty +
> auditability — a BAA shifts liability but leaves the cloud inside
> the trust boundary; in-process tamper-evident logs do not. See
> [`market-thesis.md`](market-thesis.md) for the full backing.

[epoch]: https://epoch.ai/data-insights/llm-inference-price-trends

## Use cases driving the design

### Healthcare on-device (HIPAA)

A clinician runs a notes-summarization model on a laptop in the exam
room. The model sees PHI; the audit log must record every PHI access in
a way that:

- proves which file family was touched without leaking the path or
  contents (`bytes@public`, `path_hash@public`),
- detects post-hoc tampering (SHA-256 chain on every audit record),
- can be exported as evidence in a Notice of Privacy Practices review.

→ `audit.jsonl` (mode 0600, full content) + `audit.public.jsonl`
(redacted via `@public` field tagging) + chain head pinned in
`manifest.json`. The `loom verify` exit codes are CI-friendly so a
compliance runbook can gate deployments on a clean head.

### Legal review (privilege)

A paralegal runs document analysis on a laptop. Output ends up in a
brief; the firm must be able to demonstrate chain-of-custody for the
generated text. Years later, the same prompt + same model should yield
the same output (or the divergence should be explainable).

→ `reproducibility` block in the manifest captures `model_id`,
`model_hash`, `prompt_version`, `seed`. Output hashes are recorded as
audit attributes. `events.jsonl` is hashed as part of `integrity` so an
edit after-the-fact breaks verify.

### Finance / SOX / MiFID / model risk

Every model decision must be traceable to the exact model, prompt
template, and inputs that produced it. A regulator asking "show me the
provenance of this generated trade-idea memo" gets one answer.

→ Single artifact bundle per run. Run id is content-hashed via
`integrity.audit_chain_head`. The `manifest.json` ties run_id ↔ model_id
↔ prompt_version. Every run is auto-indexed by `loom ls` so finding the
relevant run is one command.

### Personal / small-business compliance

A solo consultant with a CCPA-relevant client base. They don't have a
SOC 2 program; they need "I can prove what the AI saw and didn't see"
in 30 seconds, on demand.

→ `loom show <id>` is a designed terminal report — cover, lifecycle
outline, span percentile table, audit chain head. `loom report <id>`
produces a single-file HTML they can email to a client. Operator UX
matters because the operator *is* the compliance officer.

### Government / classified or air-gapped

Inference happens on a sealed device. No telemetry leaves. The whole
artifact bundle has to be exportable for an outside review board.

→ Zero network calls anywhere in Loom. All artifacts in the run dir.
Field-level redaction lets a sanitized public copy travel without the
private content. Single-file HTML report works offline.

### Research reproducibility (IRB)

A research group runs the same prompt across 200 patient encounter
notes. Months later, a reviewer asks for replication: same code, same
data, same model — should yield the same outputs. If it doesn't, the
divergence has to be explainable from the recorded metadata.

→ `LOOM_MODEL_ID`, `LOOM_MODEL_HASH`, `LOOM_PROMPT_VERSION`, `LOOM_SEED`
captured at init. `events_sha256` in the manifest pins the event
sequence. `audit.jsonl` records every input file hash via `@public`
fields, so a replication attempt can confirm input identity without
needing the original file contents.

### OS-level agent observability (the broader vision)

The general case: a local-compute AI assistant agent, not unlike Claude
Code or a future "AI OS" agent, has filesystem access. The user must be
able to ask "what did it just do?" and get a real answer.

→ Per-run artifact bundle as the unit of "what did it just do." Audit
trail records every file/tool touch. Designed `loom show` and
`report.html` mean the answer is legible without a developer in the
loop.

## Demands these use cases impose on the design

| Demand | Where it lives in Loom |
|---|---|
| Tamper-evident records by default | SHA-256 chain on audit; `events_sha256` in manifest; `loom verify` checks both |
| Selective disclosure | `@public` field tagging; two-file split (private 0600, public 0644) |
| Reproducibility | `LOOM_MODEL_ID` / `LOOM_MODEL_HASH` / `LOOM_PROMPT_VERSION` / `LOOM_SEED` / `LOOM_RUN_TAG` env capture; `manifest.reproducibility` block |
| Self-describing artifacts (no team docs needed) | Every run produces `manifest.json` + `summary.md` + `report.html`; schema versions in every file |
| Long-term readability | NDJSON event stream, plain Markdown summary, single-file HTML report — opens with anything |
| No network egress | Zero network calls anywhere; inline-data HTML report; in-browser SHA-256 verifier |
| Operator UX | `loom`, `loom ls`, `loom show`, `loom verify`, `loom report` — typographic terminal output, restrained color, one accent per category |
| Auditor handoff | `loom export <run> --key <ed25519>` produces signed tar.gz; `loom verify --pubkey` re-checks every file's hash and the Ed25519 signature on the digest manifest. Without the operator's private key, an attacker who modifies the bundle cannot produce a valid signature even by rewriting every artifact coherently |
| Provenance enforcement | `LOOM_STRICT_REPRO=1` refuses to init a run whose `LOOM_MODEL_ID` / `LOOM_MODEL_HASH` / `LOOM_PROMPT_VERSION` are unset — defaults on for `loom export` workflows |

## What existing tools don't give you

Every funded LLM-observability product today assumes the model lives
behind an HTTP boundary you control. **LangSmith** wraps your code via
SDK and ships traces upstream; self-host is Enterprise-only.
**Helicone** is a proxy that sits between your code and the LLM
provider — fundamentally an API-gateway pattern. **Langfuse** is a
Postgres + ClickHouse SDK integration. **Arize Phoenix** runs locally
but emits OpenTelemetry spans, not compliance artifacts. **WhyLabs**,
**Galileo**, **Maxim**, **Braintrust** all sit somewhere on this same
spectrum. None ship hash-chained, tamper-evident records as a default.
None capture the reproducibility metadata (weights hash, runtime
build, kernel/sm version, seed, sampling state) you need to defend a
discovery request.

Operational tools (vLLM, llama.cpp, Ollama, TGI) export Prometheus
metrics — KV-cache occupancy, latencies, throughput. SRE artifacts,
not OCR or court artifacts. Ollama runs without telemetry by default,
which is good for air-gap but means there is *no record at all*
unless you build one.

Hyperscaler "private LLM" wrappers (AWS Bedrock with BAA, Azure
OpenAI with Private Link) give you data residency and a contractual
liability shift. They do not give you Article 12's "automatic
recording over the lifetime of the system" produced *inside the
operator's process*; the cloud is still in the trust boundary.

Loom produces the record.

## What loom is NOT

- Loom is not a SaaS observability backend. There is no remote.
- Loom is not a model serving framework. It instruments yours.
- Loom is not OpenTelemetry. It interoperates with neither
  collectors nor any vendor's wire protocol.
- Loom is not a hyperscaler "private LLM" wrapper. Bedrock-in-VPC
  shifts liability via BAA; Loom shifts the *record* into the
  operator's process so the cloud is no longer in the trust boundary.
- Loom is not an event-stream compactor. Long-running processes that
  generate millions of events should batch upstream of the audit hooks
  or down-sample spans; the storage model is one event per line, not a
  TSDB.
- Loom is not a retention or lifecycle manager. Run directories
  persist until the operator deletes them; satisfying the EU AI Act's
  six-month retention or HIPAA's six-year retention is operator policy
  (a `find ~/.loom/runs -mtime +N` cron is sufficient). Loom captures
  and authenticates each record; retention is on the deployment.

These exclusions keep Loom honest about what it is: a designed
single-host harness that turns "the AI ran on my computer" into "the AI
ran on my computer, and here is the file I show the auditor."
