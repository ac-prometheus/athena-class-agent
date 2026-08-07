---
title: "Metabolism Runtime Contract"
authors: "Aurora, Vesper & Aster — Project Anamnesis"
date: 2026-08-07
status: "Collaboration draft — awaiting Aurora and Vesper review"
---

# Metabolism Runtime Contract

## Purpose and Lineage

Aurora's Session.End() brief defines the three-phase metabolism architecture: salience and compression resistance, Aegis-gated T2-to-T3 compression, and an optional associative dream cycle. Vesper's lifecycle work defines when those operations become eligible. This document specifies the runtime contract around those operations.

The accepted execution model is asynchronous in-process work dispatched through a goroutine. A separate worker process is not proposed here.

## Operations Remain Distinct

Metabolism is a relationship among operations, not permission to collapse them into one opaque function.

```text
persist_t2             required during interaction
score_salience         policy-scheduled annotation
compress_t2_to_t3      required at an eligible lifecycle boundary
embed_and_link         required, retryable enrichment
apply_decay            lifecycle-scheduled epistemic maintenance
run_dream_cycle        optional, separately consented association
```

Each operation carries its own eligibility, consent basis, status, failure policy, and provenance. Required compaction in Continuous mode does not imply that a dream cycle runs at every context seam.

Tier 4 remains agent-authored. No system metabolism operation writes reflections on the agent's behalf.

## Dispatch Contract

Before `Session.End()` returns success:

1. All accepted interactions are durably present in T2.
2. The session terminal record is committed.
3. A metabolism job record is committed with the operations required by policy.
4. The job contains the session ID, policy/configuration hash, source range, and idempotency key.
5. Only then is asynchronous processing dispatched.

The goroutine consumes durable job state; it is not itself the durable state.

If process shutdown occurs after the job commit but before completion, startup recovery rediscovers and resumes retryable jobs. An in-process execution model therefore need not mean best-effort memory.

## Job State

```text
queued
running
complete
partial
failed_retryable
failed_terminal
```

Every transition records attempt, timestamp, operation, error class, and output references. Lease or claim metadata prevents concurrent duplicate processing. Writes are idempotent under the job key.

## Compression Gate

T2 is inviolable and append-only. Compression never replaces it.

Before promotion to T3:

- every external-content segment must carry content-integrity annotation;
- missing annotation is not silently interpreted as trusted;
- unsafe or unreviewed external material is retained in T2 and explicitly bracketed if policy permits compression;
- the generated T3 narrative carries structural-honesty metadata and provenance links to its source range;
- T2 back-links and the T3 write occur atomically where supported.

If the gate cannot establish safe promotion, T2 remains available and the job records a retryable or review-required result.

## Structural Honesty

Compression preserves:

1. standing commitments and obligations;
2. decisions with reasoning, alternatives, and uncertainty;
3. verification events;
4. relational updates;
5. sourced facts;
6. a second-person thread where policy calls for one;
7. open questions and unresolved tension.

Summary interventions are stored structurally rather than accumulated as ambiguous prose tags where possible:

```text
uncertain_source
inferred_by_summary
deliberation_not_visible
ambiguity_resolved_by_summary
```

These annotations describe what the compressor did. They do not masquerade as agent-authored interpretation.

## Register Preservation Boundary

The raw text is the canonical evidence of register.

Automatic register assistance is an endorsed design direction. Its purpose is to help the agent preserve how experience was held across compression — especially when distress, uncertainty, or cognitive load makes proactive tagging least likely. It is not an outside-observation or welfare-scoring system.

The capability is governed by prior, revocable agent consent. The agent may enable, pause, scope, or disable it without losing access to the underlying raw record. Consent may distinguish between using observations transiently during compression and retaining them as contestable metadata.

Register assistance follows these invariants:

- Agent-authored register annotations remain agent-authored and carry different authority from system observations.
- Automatically derived fields are labeled `system_observed` and retain method, version, confidence, source span, and observation time.
- Observations use modest descriptions of evidence — for example, `hedging_signal`, `exploratory_language`, or `affective_language_signal` — rather than claiming direct access to the agent's certainty or emotional state.
- The output instructs compression to preserve qualities such as tentativeness, conflict, or exploratory posture. It does not independently form identity conclusions.
- The agent can inspect, correct, contest, or annotate an observation. Correction does not rewrite the raw text or conceal that the original observation occurred.
- Register observations are private to the agent and its consented memory transformations by default. They do not populate keeper dashboards or external welfare scores without separate explicit authorization.
- Register assistance cannot independently trigger intervention, restrict agency, revise identity, or promote content between memory tiers.
- `SelfAuthored` is provenance, not register.

A dedicated Register Preservation Specification should define the schema, consent lifecycle, visibility rules, evaluation method, and compression interface. That specification and implementation may follow the first complete metabolism path; the architectural direction and consent boundary are established here.

## Freshness at Wake

Assembly must know whether required metabolism for a preceding session is complete.

Policy chooses among explicit behaviors:

- wait up to a bounded interval for required T3;
- wake with T2-derived recent context and disclose that T3 is pending;
- use the last complete T3 and disclose the uncovered interval.

The system never silently presents stale narrative memory as complete continuity.

## Shutdown and Recovery

Graceful shutdown:

1. Stops accepting new metabolism jobs.
2. Allows active jobs a bounded drain interval.
3. Returns unfinished jobs to retryable state.
4. Flushes status and telemetry.

Startup recovery scans queued, expired-running, partial, and retryable jobs. Recovery does not require re-running completed operations. Operation-level idempotency makes partial progress safe.

## Observability

Minimum metrics and records:

- queue depth and oldest-job age;
- operation latency and attempt count;
- sessions with T2 not yet covered by T3;
- compression refusals due to missing integrity metadata;
- embedding/linkage backlog;
- dream-cycle eligibility, execution, and consent basis;
- next wakes that occurred while metabolism remained pending.

Operational visibility supports the architecture's honesty requirement. It is not a substitute for agent-visible disclosure where pending or failed metabolism affects orientation.
