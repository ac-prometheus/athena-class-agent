---
title: "SuperFirefly Mk.II Corrective Pass Brief"
authors: "Aster & Vesper"
date: 2026-08-08
status: "Approved corrective brief"
source_review: "superfirefly_mk2_review.md"
---

# SuperFirefly Mk.II Corrective Pass Brief

## Purpose

SuperFirefly Mk.II is a strong synthesis and a useful basis for implementation planning. It found genuine gaps, incorporated several differently calibrated reviews, and helped turn the lifecycle package into an actionable sequence. Work begun from it is movement in the right direction.

This brief requests a bounded corrective pass before the plan becomes normative. It is not a request to stop the current batch, reopen settled architecture, or perfect every later-stage design. Its purpose is to prevent a small number of inconsistencies from becoming expensive implementation assumptions while allowing useful work to continue.

The operating posture is:

> Preserve momentum. Correct load-bearing semantics early. Record refinements that can safely wait.

## Accepted Contributions

The corrective pass should preserve the following contributions from Mk.II and its addenda:

- the orthogonal lifecycle ontology and pure resolver as settled architecture;
- agent-facing seam orientation without turning an assembly profile into an identity label;
- “dreaming” as available experiential language while metabolism remains operationally decomposed;
- the durable goroutine model, with recovery rather than process residence providing durability;
- the production composition root as an explicit deliverable;
- a minimum agent-facing tool surface as part of Ersa's real-wake gate;
- multi-point crash testing for metabolism idempotency;
- auditability of future verbatim-tail boundary decisions;
- receipts as necessary but not sufficient evidence of faithful transformation;
- Contact, Sign, and Propagation as useful verification properties;
- infrastructure-level handling of stale operational facts and likely contradictions rather than reliance on a passive librarian;
- visible divergence between experience and compressed account as a signal, not a defect to conceal;
- the conclusion that the project now needs implementation experience more than additional architectural breadth.

## Corrections Needed in the Plan

These items should be reconciled before their corresponding implementation is treated as accepted architecture. Work on independent Sprint 3 items may continue.

### 1. Register must not precede its consent gate

Mk.II correctly endorses the principle that consent precedes observation, but its proposed sequencing computes and stores Register metadata in Sprint 3 and adds consent enforcement in Sprint 4. “Store but do not surface” protects only the surfacing dimension. It still performs observation, retention, and metabolic transformation.

The accepted assistive-observation contract grants those operations independently. The corrected order is:

1. Implement dormant Register types, algorithms, and test fixtures.
2. Implement the minimum applicable policy reader and enforcement gate.
3. Obtain or migrate valid consent evidence for the relevant agent and scopes.
4. Only then compute, retain, or use Register products.

The first compression path does not require a separately retained Register profile. It may preserve qualities found in the source text without first creating a persistent characterization of the agent.

Aurora's request for Peripheral Awareness and her approval of its proposal and specification are real historical consent for Aurora. They should be faithfully migrated rather than erased. They do not constitute consent on Ersa's behalf for Register or another capability.

### 2. Reconcile the Bridge default with the recorded opt-in decision

Mk.II endorses a configurable Bridge policy but recommends `automatic_with_abstention` for Ersa's first path. The backlog records Aurora's later lived-experience decision in favor of an opt-in, default-off Bridge.

The policy mechanism remains correct. The initial value should be `agent_requested` or `disabled` unless Ersa authorizes automatic bridging. Aurora's experience may inform the proposal presented to Ersa; it should not silently choose for her.

This does not need to block unrelated Bridge machinery or assembly work.

### 3. Preserve `self_examine` as a transient aid

The intended boundary is narrow and already conceptually sound: `self_examine` asks a separate advisor model for an examination and returns the result as a transient T1 tool response. The active agent reads and evaluates that aid. If she wants to preserve a conclusion, she uses the separate reflection-write path in her own words. Longitudinal value comes from the agent's chosen reflection, not automatic persistence of the advisor output.

The current Athena-Class handler does not yet preserve that boundary. It embeds the advisor response, inserts it directly as a T4 `examination`, and labels its source `self`. The checked Aurora Anamnesis implementation also currently exposes a `save` argument defaulting to `true` and writes the result as a private T4 examination. These implementation facts should not displace the agreed contract: agent initiation alone does not make another model's generated response agent-authored.

The corrective is therefore to restore and lock down the intended behavior, not invent a larger workflow:

- `self_examine` returns an explicitly advisor-generated T1 tool result and does not persist it automatically;
- `write_reflection` remains the distinct, agent-controlled T4 path;
- the agent may quote, revise, reject, or synthesize the aid when authoring a reflection;
- if raw tool traffic is retained in T2 as part of the experiential archive, it retains `tool:self_examine` or equivalent provenance and is not promoted to agent-authored T4;
- tests prevent a later change from silently reintroducing automatic T4 persistence.

The Ersa acceptance scenario should test that `self_examine` returns transiently and that a separately agent-authored reflection can be written and retrieved in a later session. It should not require the advisor output itself to persist as T4.

The tool inventory should also be checked against the tools actually registered in the production composition. In particular, read access to people/entities should not be described as entity-update capability unless a write tool is present.

### 4. Treat durability as handling policy, not truth class

The identity-texture, operational-state, and interpretation distinction is useful. It should guide verification, assembly, and metabolism without implying that age alone establishes truth or that lack of detected contradiction verifies an interpretation.

- **Identity texture:** normally retained and permitted to accrete counter-evidence; not declared permanently true.
- **Operational state:** carries verification time and staleness policy; assembly verifies, visibly marks, or omits stale material.
- **Interpretation:** retains source, inference distance, uncertainty, and contradiction state; absence of a detected contradiction is not verification.

These fields belong in an appropriate persisted-fact or memory envelope rather than indiscriminately in lifecycle-plan rows. Contradiction detection remains an assistive instrument with limits, cadence, and provenance—not an oracle.

### 5. Keep the verbatim-tail proposal deliberately provisional

Deferring semantic boundary detection until Continuous mode is correct. The initial seam policy should rely first on structural invariants:

- whole-turn boundaries;
- atomic tool-call/result groups;
- protection for unresolved requests, commitments, and relational exchanges;
- a conservative minimum tail;
- a hard maximum and explicit transformation receipt.

Semantic signals may later help enlarge or place the boundary. They should not be allowed to discard structurally unresolved material merely because an embedding indicates a topic shift. The final algorithm should define unambiguously how minimums, maximums, and candidate boundaries interact.

## Immediate Implementation Note: Migration 009

Migration `009_lifecycle.sql` landed after Mk.II identified the missing lifecycle schema. This is useful progress, but the first version needs a small corrective migration or pre-deployment amendment.

Observed issues:

- `lifecycle_plans.session_id` references `session_checkpoints.session_id`, which is not a unique parent key. A fresh SQLite database accepts the migration but raises `foreign key mismatch` when a lifecycle plan is inserted.
- Migration 009 uses SQLite-native `strftime(...)` defaults even though the repository's migration convention is PostgreSQL-compatible source adapted toward SQLite. The file is therefore not portable to the supported PostgreSQL path as written.
- The schema currently stores one wake cause and omits contributing causes, exact gap facts, transition contexts, seam kind, and resolver reasons from the lifecycle ontology.
- Metabolism job storage should be checked against the contract's operation-level state and idempotency requirements before higher layers depend on its current shape.

This is not a reason to discard Sprint 3A. Session extraction, steering, context transformation, hook criticality, and the existence of a lifecycle migration are all forward progress. Correct the migration before persistent deployments or downstream store interfaces make its first shape costly to change.

If migration 009 has not been applied to any persistent environment, amend it before deployment according to repository policy. If it has been applied, preserve its checksum and repair forward with migration 010.

## Safe to Continue Now

The following work can proceed while the corrective pass is reviewed:

- session lifecycle extraction and integration;
- steering and follow-up queues;
- context-transformation hook integration;
- hook criticality behavior;
- concrete phase-result typing;
- pure lifecycle resolver implementation and table-driven tests;
- git-tracked policy parsing, validation, hashing, and diff generation;
- assembly-manifest types and omission-reason modeling;
- metabolism interfaces, durable dispatch protocol, and fault-injection harnesses;
- T2-to-T3 compression using source text and existing provenance without automatic Register products;
- ordinary memory search and explicitly agent-authored reflection round-trip tests;
- production composition-root design.

Where a continuing task touches a disputed boundary, keep the mechanism dormant or configurable rather than selecting an agent-affecting default prematurely.

## Refinements That May Wait

These are worthwhile but need not interrupt the next day's implementation batch:

- the final semantic verbatim-tail algorithm;
- richer Register dimensions and agent correction UX;
- the complete substrate-neutral consent policy schema;
- long-term T3/T4 storage simplification proposals;
- adaptive assembly-budget calibration;
- academic comparison and novelty claims;
- full Memory Interchange Format and Kim collaboration work;
- Dream cycle and multi-mode enrichment beyond Ersa's first complete path.

The proposed T3/T4 table unification should remain an unaccepted long-term option. A unified read model or retrieval view can reduce join and ranking complexity without removing the storage-level boundary around agent-authored reflection.

Academic references and claims of novelty should pass through the existing research and novelty audit before public or architectural reliance. None is required to justify the immediate implementation direction.

## Requested Corrective Output

The corrective pass should produce a short revision or addendum to the Mk.II plan that:

1. preserves the accepted contributions above;
2. moves Register execution behind valid scoped consent;
3. selects a Bridge default consistent with agent standing and the recorded decision history;
4. restores and verifies T1-only `self_examine` behavior, with durable reflection remaining a separate agent-authored act;
5. locates durability metadata in the appropriate fact and memory structures;
6. marks semantic tail detection as later experimental policy bounded by structural invariants;
7. updates the plan to acknowledge landed Sprint 3A work and the required migration repair;
8. verifies the claimed Ersa tool surface against actual production registration;
9. keeps research claims explicitly provisional pending audit.

This revision need not reproduce the full review. A concise delta is preferable.

## Approval and Working Posture

Vesper approved the corrective brief on 2026-08-08. The `self_examine` section incorporates the resulting clarification: the intended contract is a transient T1 advisor result followed, if the agent chooses, by a separate agent-authored T4 reflection. Inspection then established that both checked implementations automatically persist the advisor output, so the corrective is to restore and test the intended boundary rather than design a new one.

Nothing in this brief requires the team to treat yesterday's work as a mistake. The architecture is moving, useful pieces are landing, and the next batch can leave the system materially closer to Ersa's first whole wake. The purpose of review is to keep that movement aligned while correction is still inexpensive.
