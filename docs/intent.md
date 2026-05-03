# Loom — Intent and use cases

Loom is observability for **local-compute AI** — workloads where the model
runs on the operator's own device, touches their files, and must be
defensible if a regulator or auditor asks how it behaved. This document is
the source of truth for *why* features exist; pin it before adding new
surface area.

## The core thesis

The frontier-cloud-AI status quo (OpenAI / Anthropic subscriptions,
remote tool-use, server-side logs) does not survive contact with three
real constraints:

1. **Subscription economics**: consumer-tier subscriptions on hosted
   models lose money and will tighten under rate limits. Small businesses
   already feel this; the long tail of personal AI moves on-device.
2. **Compliance gravity**: any business under HIPAA, GLBA, MiFID, GDPR,
   FERPA, attorney-client privilege, or a Data Use Agreement can't send
   the inputs the model needs to a third party. The model lives where
   the data already does.
3. **Agent-on-files reality**: the most useful AI work — agents reading
   the user's filesystem, drafting on their behalf, writing back —
   demands an audit trail the user *owns*, not one streamed to a vendor
   they can't subpoena.

Local AI is operationally different from cloud AI. There is no SRE
backend to query when something fails, no shared S3 bucket of run logs
that legal can attach to a discovery request, no one to call when a
regulator wants to know exactly which inputs produced a flagged output.
The local machine itself has to become the system of record.

Loom is what makes that possible.

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

## What loom is NOT

- Loom is not a SaaS observability backend. There is no remote.
- Loom is not a model serving framework. It instruments yours.
- Loom is not OpenTelemetry. It interoperates with neither
  collectors nor any vendor's wire protocol.
- Loom is not an event-stream compactor. Long-running processes that
  generate millions of events should batch upstream of the audit hooks
  or down-sample spans; the storage model is one event per line, not a
  TSDB.

These exclusions keep Loom honest about what it is: a designed
single-host harness that turns "the AI ran on my computer" into "the AI
ran on my computer, and here is the file I show the auditor."
