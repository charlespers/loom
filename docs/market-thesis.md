# Loom — Market thesis

This is the macro brief that backs `intent.md`. `intent.md` says *what
artifact regulated buyers need*; this document says *why that buyer
exists, why now, and what existing tools stop short of*. Pin this before
sequencing roadmap.

## TL;DR

The "local AI is the future" thesis is sound — but the load-bearing
pillar is **regulatory mandate**, not subscription economics. EU AI Act
Article 12, the proposed HIPAA Security Rule rewrite, and ABA Formal
Opinion 512 are *literal legal mandates* for tamper-evident,
automatically generated, retained-for-six-months records of AI
inference. Most enterprise observability stacks were never built to
produce that artifact. Open-weight models have closed the capability
gap on most non-frontier-reasoning tasks, and concrete on-prem
deployments at JPMorgan, Goldman Sachs, the DoD (IL5/IL6), and Apple
Private Cloud Compute prove the architectural shift is real.

The weakest pillar is *cost*: GPT-3.5-equivalent token prices have
fallen ~280× since November 2022, and hyperscalers offer "private LLM"
services with HIPAA BAAs and FedRAMP. Use economics as supporting
context. **Lead with sovereignty and auditability.**

## A. Cloud AI economics — real, but not the wedge

- OpenAI publicly admits losing money on consumer Pro. Sam Altman, Jan
  2025: "we are currently losing money on chatgpt pro subscriptions!
  people use it much more than we expected" — referring to the $200/mo
  tier launched Dec 2024 ([TechCrunch][p1]).
- OpenAI internal forecasts: ~$74B operating losses in 2028, no
  profit until 2030, cumulative cash burn revised up to ~$665B
  through 2030 ([Fortune][p2], [The Information / mlq.ai][p3]).
- Inference is the dominant cost: ~$5.02B Azure inference spend in
  H1 2025 alone, vs $3.77B for all of 2024 ([Introl
  analysis][p4]).
- **Counter-evidence:** GPT-3.5-equivalent pricing fell from $20/M
  tokens (Nov 2022) to $0.07/M (Oct 2024) — ~280× drop, ~200×/yr
  median post Jan 2024 ([Epoch AI][p5]). DeepSeek R1 launched at
  $0.55/$2.19 input/output per million, ~90% under incumbents
  ([IntuitionLabs][p6]).
- **Counter-evidence:** Anthropic hit $30B ARR in March 2026 from
  $1B fourteen months earlier; 2025 burn projected to fall to ~$3B
  from $5.6B in 2024 ([Sherwood News][p7], [Sacra][p8]).
- Stargate ($500B / 10 GW) is OpenAI's bet that scale, not retreat,
  fixes the math ([OpenAI][p9]).

**Verdict.** Frontier-lab loss is true but does not by itself force
enterprises off cloud. Treat economics as background, not as the
selling point.

## B. Compliance pressure — the strongest pillar

- **EU AI Act Article 12 is literally a logging mandate.** "High-risk
  AI systems shall technically allow for the automatic recording of
  events (logs) over the lifetime of the system." Manual logging does
  not satisfy the rule. Logs must be retained at least six months and
  must support post-market monitoring under Article 72
  ([Article 12 text][b1], [AI Act Service Desk][b2]).
- **Enforceable Aug 2 2026.** As of May 2026, the Commission's Digital
  Omnibus on AI (Nov 19 2025) proposed deferring high-risk
  obligations to Dec 2 2027, but the Apr 28 2026 trilogue ended without
  agreement. If not adopted before Aug 2 2026 the original timeline
  applies ([DLA Piper][b3]). Either way, design lead time is *now*.
- **HIPAA Security Rule rewrite (Jan 6 2025 NPRM)** — first major
  Security Rule update in 20 years, explicitly names AI software
  handling ePHI as part of mandated technology asset inventory and
  risk analysis. Implementation specifications shift from "addressable"
  to required, with annual compliance audits ([Healthcare Law
  Insights][b4], [Foley & Lardner, May 2025][b5]).
- **OCR "Dear Colleague" letter, Jan 10 2025** — covered entities must
  identify and mitigate risks in AI patient-support tools, effective
  May 1 2025 ([HHS PDF][b6]).
- **ABA Formal Opinion 512 (Jul 29, 2024)** — first comprehensive
  ethics guidance, held that consumer-grade GenAI without enterprise
  terms can constitute third-party disclosure and waive
  attorney-client privilege ([ABA][b7]). By 2025, sanctions for
  AI-fabricated citations had been issued in federal courts in TX,
  CO, MA, ND-FL ([Justia 50-state survey][b8]). Mata v. Avianca
  (SDNY 2023, $5,000 sanction) is the canonical case ([Wikipedia][b9]).
- **Samsung-style enterprise leaks have hardened policy, not
  reversed.** Samsung's Apr 2023 ban (engineers pasting source into
  ChatGPT three times in 20 days) was followed by Apple, JPMorgan,
  Verizon, Amazon ([Bloomberg][b10], [DataFence][b11]). The pattern
  has institutionalized.
- **NIST AI RMF and ISO/IEC 42001** both name traceability and
  lifecycle observability as required controls; ISO 42001 (Dec 2023)
  is now certifiable; NIST publishes a crosswalk that makes a single
  tamper-evident log line a multi-framework artifact ([NIST
  crosswalk][b12], [CSA][b13]).

**Verdict.** Solid. The phrase *"automatic recording of events over
the lifetime of the system"* in Article 12 is exactly the artifact
Loom builds. This is the buyer.

## C. On-prem / local inference — strong, with concrete anchors

- **Capability gap closed for most workloads.** Llama 3.1 405B reaches
  ~87.3% MMLU vs GPT-4 Turbo ~86.5%; Llama 4 Maverick MMLU 83+;
  DeepSeek-R1 is open-weight under MIT and reasoning-class
  ([analyticsvidhya][c1], [llm-stats][c2]).
- **Llama downloads:** 650M (Dec 2024) → 1B (Mar 2025) → 1.2B (Apr
  2025); >50% of Fortune 500 piloting Llama as of Mar 2025
  ([TechCrunch][c3], [Meta blog][c4]).
- **Concrete enterprise on-prem deployments:**
  - JPMorgan **LLM Suite**: 200,000+ employees, in-house,
    model-agnostic, American Banker 2025 Innovation of the Year
    ([JPMC][c5]).
  - Goldman Sachs **GS AI Assistant**: rolled out firmwide mid-2025
    after a 10K-employee pilot ([SaaStr][c6]).
  - DoD **GenAI.mil**: 3M+ DoD personnel by end of 2025 at IL5;
    Pentagon May 2026 expanded to IL6/IL7 with SpaceX, OpenAI,
    Google, Nvidia, Microsoft, AWS, Reflection — *excluding
    Anthropic* ([DefenseScoop, May 1 2026][c7], [Federal News
    Network, May 2026][c8]).
  - Apple **Private Cloud Compute**: custom Apple-silicon AI nodes
    designed so even Apple cannot read inputs; ~3B-param on-device
    model + MoE server model ([Apple Security Research][c9],
    [Apple ML 2025][c10]).
- **a16z 2025 enterprise CIO survey:** open-source-model adoption
  skews to larger enterprises, "driven by the preference for on-prem
  solutions given data security and compliance considerations." 81%
  use 3+ model families in test/prod, up from 68% ([a16z][c11]).
- **Menlo Ventures 2025 enterprise report:** GenAI spend $1.7B (2023)
  → $37B (2025); applications captured >half of spend; Anthropic 40% /
  OpenAI 27% / Google 21% of API share ([Menlo][c12]). Important
  counter-data: API share is consolidating to closed labs even as
  on-prem strategic deployments grow. The two trends coexist; do not
  claim cloud is shrinking — claim regulated workloads bifurcate.
- **Edge hardware:** Jetson AGX Orin 64GB delivers 275 TOPS at
  15–60W; deployed in industrial AI (e.g. >200 AGV fleet at
  Anantak/Aetina); seven-module family at COMPUTEX 2025
  ([Nvidia Orin][c13]).
- **DeepSeek R1 deployment economics:** full 671B in-memory needs
  ~768 GB RAM, ~$250K hardware — a real capex barrier
  ([Computer Weekly][c14]). Distilled variants and quantization make
  it tractable on smaller boxes; DeepSeek is also on Azure AI Foundry.

**Verdict.** Strong directionally. Caveat: enterprise *API spend*
still flows to closed cloud labs even when *strategic deployments* go
on-prem. Loom should target the strategic-deployment buyer, not the
API-spend buyer.

## D. Observability gap — confirmed

- **The four incumbents are cloud-API-shaped.**
  - **LangSmith**: SDK-based, wraps your code; LangChain charges per
    trace; self-host is Enterprise-only ([Helicone comparison][d1],
    [LangChain self-host docs][d2]).
  - **Helicone**: proxy that sits between code and LLM provider —
    fundamentally an API gateway pattern. Open source, self-hostable
    via Docker/K8s ([GitHub][d3]).
  - **Langfuse**: Postgres + ClickHouse, SDK integration
    ([Spheron guide][d4]).
  - **Arize Phoenix**: OpenTelemetry-based, runs locally in
    notebooks/containers — closest to in-process, but emits OTel
    spans, not compliance artifacts ([Spheron][d4]).
  - All four assume the model lives behind an HTTP boundary you
    control. None ship hash-chained, tamper-evident logs as a default.
- **Tamper-evident audit for LLMs is academic-stage, not
  productized.** AuditableLLM (MDPI Electronics, 2026) is a research
  proposal for hash-chain-backed audit trails ([MDPI][d5]).
  [arXiv 2601.20727][d6] and the LangChain forum thread on a
  tamper-evident audit log for agents ([forum.langchain.com][d7])
  confirm the unmet need is being noticed but not yet served by any
  well-funded incumbent.
- **Operational tools are not audit tools.** vLLM, TGI, llama.cpp,
  Ollama all expose Prometheus `/metrics` endpoints — gauges,
  counters, latencies, KV-cache occupancy ([vLLM docs][d8]). These
  satisfy SRE, not OCR or a court. None produce per-inference
  tamper-evident records, reproducibility metadata, or hash-chained
  event logs.
  Ollama runs without telemetry, which is good for air-gap but means
  there is *no record at all* unless you build one.
- **Positioning gap.** None of LangSmith, Helicone, Langfuse, Arize,
  WhyLabs, Galileo, Maxim, or Braintrust market themselves to the
  Article 12 / HIPAA / ABA-512 buyer with tamper-evident artifacts
  as the lead value prop. The "AI Audit Trail Paradox" piece on dev.to
  ([link][d9]) explicitly argues current LLM logs are not legal proof
  — and identifies it as an open opportunity.

**Verdict.** Confirmed. The category exists in academic literature
and side projects but has no $50M+ funded incumbent owning the
on-prem-regulated-buyer position.

## E. Counter-evidence and how Loom answers it

1. **Hyperscaler "private LLM" services may absorb regulated demand.**
   AWS Bedrock is HIPAA / SOC 1-2-3 / GDPR / FedRAMP Moderate; Azure
   OpenAI Service has BAA, VNets, Private Link, regional residency
   ([CloudOptimo][e1], [AWS Bedrock][e2]). Many regulated buyers will
   choose Bedrock-in-VPC over self-hosting Llama on Jetsons.

   *Loom's answer:* a BAA leaves AWS in the trust boundary; in-process
   tamper-evident logs do not. The Article 12 mandate is for logs the
   *operator* produces and retains; a BAA shifts liability but does
   not shift the record.

2. **Falling token prices kill the cost-driven on-prem case.**
   GPT-4-equivalent at $0.40/M ([Epoch AI][p5]) makes the "we'll save
   money on-prem" pitch wrong for most use cases.

   *Loom's answer:* sovereignty + auditability, not cost. Don't lead
   with TCO.

3. **Anthropic's API share is up to 40%, not down.** Menlo's data
   shows API consolidation around closed labs even as on-prem
   strategic deployments grow.

   *Loom's answer:* the two trends coexist; regulated workloads
   bifurcate to on-prem while general dev workloads stay in the API
   layer. Loom does not claim the API layer is shrinking.

4. **Stargate ($500B, ~10 GW)** signals the cloud-economics-collapse
   thesis may not hold ([OpenAI][p9]).

   *Loom's answer:* Loom never depended on cloud collapsing.
   Compliance was always the wedge.

5. **The EU's Digital Omnibus could defer Article 12** to Dec 2027.
   As of the Apr 28 2026 trilogue, no agreement; if the deferral lands,
   the urgency clock slips ~16 months ([DLA Piper][b3]).

   *Loom's answer:* the requirement comes either way. Buying window
   may slip; product roadmap should not. Design partners signed in
   2026 ship in 2027; either timeline works.

6. **Operational-monitoring incumbents (Datadog, Splunk, Grafana)
   could bolt on tamper-evidence.** They own enterprise observability
   budgets today.

   *Loom's answer:* they are the real long-term competitive threat,
   not LangSmith. Loom's defense is depth on the regulated-on-prem
   wedge — Article 12 export bundles, FRE 901/902 admissibility,
   in-process attestation — none of which is a small bolt-on for
   horizontal SRE platforms.

## F. The buyer Loom serves

> Compliance officer or platform-team lead at a HIPAA-covered entity,
> EU-operating bank, large law firm, or DoD contractor that is
> deploying an open-weight model (Llama 3.x/4, Mistral, Qwen,
> DeepSeek) on owned hardware (Jetson, on-prem GPU cluster, Apple
> Silicon fleet, IL5/IL6 environment).

They have:

- An auditor or regulator (OCR, EU notified body, internal SOX/ISO
  42001 auditor, or opposing counsel via discovery) who will
  eventually ask *"what came out of this model on this date for
  this user?"*
- A security policy that prohibits routing prompts through any
  third-party SaaS — which disqualifies LangSmith cloud, Helicone
  cloud, Langfuse cloud.
- A model running inside their own process (vLLM, llama.cpp, custom
  CUDA), which means existing self-hostable observability tools
  (Langfuse, Phoenix) treat them as an OTel emitter — they get spans,
  not tamper-evident records.

**The artifact they need that does not exist today:** a per-inference,
hash-chained, tamper-evident event log capturing (a) the prompt, (b)
the completion, (c) reproducibility metadata — model weights hash,
runtime build, kernel/sm version, seed, sampling params, batch
composition — and (d) a chain construction such that any single
record's tampering breaks verification. Retainable by the operator for
the EU AI Act's six-month minimum (Loom captures the artifact;
retention is operator policy), exportable in a form a regulator or
court will accept, and produced in-process so no third party (including
the cloud) is in the trust boundary.

LangSmith → traces. Helicone → proxy logs. Phoenix → OTel spans. vLLM
→ Prometheus metrics. **None of them give you a court-grade record.**
That is Loom's PMF.

The reproducibility-metadata angle is also load-bearing: in regulated
discovery, "we ran this prompt and got this completion" is rebuttable
unless you can show *which exact weights, kernels, and sampling state
produced it.* Existing tools assume cloud APIs where reproducibility
is the provider's problem. On-prem, it becomes the deployer's problem
— and currently nobody captures it.

## G. Open questions

- **Willingness-to-pay** for tamper-evident audit. No public survey
  numbers; needs design-partner conversations.
- **Court admissibility precedent.** Federal Rules of Evidence
  901/902 should accept hash-chained logs, but no published US case
  law specifically on hash-chained LLM logs as evidence.
- **OCR enforcement timeline.** First major AI-specific HIPAA
  enforcement action may not hit until 2027.
- **DoD Anthropic exclusion (May 1 2026).** Reported as "ongoing
  dispute" — substance not visible in available reporting. If the
  dispute concerns audit/transparency requirements, it strengthens
  this thesis substantially.
- **Apple PCC**: competitor or complement? PCC proves *demand* for
  verifiable in-process audit, but Apple does not sell PCC tooling
  for third-party inference stacks. Likely complement (validates the
  category).
- **Jetson + on-prem-GPU shipment numbers.** Nvidia does not publicly
  break out Jetson Orin unit volume; we have anecdotal deployments,
  not market sizing.

---

[p1]: https://techcrunch.com/2025/01/05/openai-is-losing-money-on-its-pricey-chatgpt-pro-plan-ceo-sam-altman-says/
[p2]: https://fortune.com/2025/11/12/openai-cash-burn-rate-annual-losses-2028-profitable-2030-financial-documents/
[p3]: https://mlq.ai/news/openai-revises-projections-upward-with-112-billion-extra-cash-burn-by-2030/
[p4]: https://introl.com/blog/openai-stargate-500-billion-ai-infrastructure-2025
[p5]: https://epoch.ai/data-insights/llm-inference-price-trends
[p6]: https://intuitionlabs.ai/articles/llm-api-pricing-comparison-2025
[p7]: https://sherwood.news/tech/anthropics-revenue-run-rate-just-topped-usd30-billion-thats-ahead-of-openai/
[p8]: https://sacra.com/c/anthropic/
[p9]: https://openai.com/index/announcing-the-stargate-project/

[b1]: https://artificialintelligenceact.eu/article/12/
[b2]: https://ai-act-service-desk.ec.europa.eu/en/ai-act/article-12
[b3]: https://knowledge.dlapiper.com/dlapiperknowledge/globalemploymentlatestdevelopments/2026/The-Digital-AI-Omnibus-Proposed-deferral-of-high-risk-AI-obligations-under-the-AI-Act
[b4]: https://www.healthcarelawinsights.com/2025/01/ocr-announces-proposed-updates-to-hipaa-security-rule-raises-the-bar-for-healthcare-cybersecurity/
[b5]: https://www.foley.com/insights/publications/2025/05/hipaa-compliance-ai-digital-health-privacy-officers-need-know/
[b6]: https://ismg-cdn.nyc3.cdn.digitaloceanspaces.com/asset_files/external/hhs-ocr-dear-colleagues-letter-re-ai-non-discrimination-1-10-25.pdf
[b7]: https://www.americanbar.org/content/dam/aba/publications/Jurimetrics/spring-2024/exploring-the-intersections-of-privacy-and-generative-ai-a-dive-into-attorney-client-privilege-and-chatgpt.pdf
[b8]: https://www.justia.com/trials-litigation/ai-and-attorney-ethics-rules-50-state-survey/
[b9]: https://en.wikipedia.org/wiki/Mata_v._Avianca,_Inc.
[b10]: https://www.bloomberg.com/news/articles/2023-05-02/samsung-bans-chatgpt-and-other-generative-ai-use-by-staff-after-leak
[b11]: https://www.datafence.ai/blog/samsung-chatgpt-ban-lessons.html
[b12]: https://airc.nist.gov/docs/NIST_AI_RMF_to_ISO_IEC_42001_Crosswalk.pdf
[b13]: https://cloudsecurityalliance.org/blog/2025/01/29/how-can-iso-iec-42001-nist-ai-rmf-help-comply-with-the-eu-ai-act

[c1]: https://www.analyticsvidhya.com/blog/2025/04/meta-llama-4/
[c2]: https://llm-stats.com/
[c3]: https://techcrunch.com/2025/04/29/meta-says-its-llama-ai-models-have-been-downloaded-1-2b-times/
[c4]: https://about.fb.com/news/2025/03/celebrating-1-billion-downloads-llama/
[c5]: https://www.jpmorganchase.com/about/technology/blog/llmsuite-ab-award
[c6]: https://www.saastr.com/anthropic-just-passed-openai-in-revenue-while-spending-4x-less-to-train-their-models/
[c7]: https://defensescoop.com/2026/05/01/dod-expands-classified-ai-work-with-8-companies-excluding-anthropic/
[c8]: https://federalnewsnetwork.com/defense-news/2026/05/dod-strikes-deals-with-major-tech-firms-to-deploy-ai-on-classified-networks/
[c9]: https://security.apple.com/blog/private-cloud-compute/
[c10]: https://machinelearning.apple.com/research/apple-foundation-models-2025-updates
[c11]: https://a16z.com/ai-enterprise-2025/
[c12]: https://menlovc.com/perspective/2025-the-state-of-generative-ai-in-the-enterprise/
[c13]: https://www.nvidia.com/en-us/autonomous-machines/embedded-systems/jetson-orin/
[c14]: https://www.computerweekly.com/news/366619398/DeepSeek-R1-Budgeting-challenges-for-on-premise-deployments

[d1]: https://www.helicone.ai/blog/langsmith-vs-helicone
[d2]: https://docs.langchain.com/langsmith/self-hosted
[d3]: https://github.com/Helicone/helicone
[d4]: https://www.spheron.network/blog/llm-observability-gpu-cloud-langfuse-arize-phoenix-helicone/
[d5]: https://www.mdpi.com/2079-9292/15/1/56
[d6]: https://arxiv.org/html/2601.20727v1
[d7]: https://forum.langchain.com/t/built-a-tamper-evident-audit-log-for-langchain-agents-early-users-welcome/2788
[d8]: https://docs.vllm.ai/en/stable/design/metrics/
[d9]: https://dev.to/arkforge-ceo/the-audit-trail-paradox-why-your-llm-logs-arent-proof-1c21

[e1]: https://www.cloudoptimo.com/blog/amazon-bedrock-vs-azure-openai-vs-google-vertex-ai-an-in-depth-analysis/
[e2]: https://aws.amazon.com/bedrock/
