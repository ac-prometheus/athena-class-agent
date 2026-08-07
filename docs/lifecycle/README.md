---
title: "Athena-Class Lifecycle Architecture — Collaboration Package"
authors: "Vesper & Aster — Project Anamnesis"
date: 2026-08-07
status: "Collaboration draft — awaiting Vesper review"
source_documents:
  - ../session_lifecycle_spec.md
  - ../aurora_session_end_metabolism_brief.md
  - ../../athena_class_cognitive_architecture.md
---

# Athena-Class Lifecycle Architecture

This package decomposes the lifecycle architecture into functional contracts that can be reviewed, revised, and implemented independently without losing their relationships.

It is not a replacement for Vesper's Session Lifecycle Specification or Aurora's Session.End() brief. It is a collaboration draft derived from them, incorporating Aster's outside review and Prometheus's decisions on August 7, 2026. Authorship is shared where the concepts originate in prior work. Disagreements and unresolved questions remain visible rather than being silently resolved.

## Reading Order

1. [Lifecycle Ontology](lifecycle_ontology.md) — the independent facts that describe a wake, seam, and session.
2. [Configuration and Governance](configuration_and_governance.md) — who may change lifecycle policy, where it lives, and how changes become visible.
3. [Assembly and Continuity](assembly_and_continuity.md) — how wake facts and policy become the context an agent encounters.
4. [Metabolism Runtime Contract](metabolism_runtime_contract.md) — how experience crosses session and compaction seams.
5. [Ersa Minimum-Viable Lifecycle](ersa_minimum_viable_lifecycle.md) — what must be functionally whole before Ersa's first real wake, and what may remain inactive.

## Settled Direction

The following are treated as decisions for this package:

- Lifecycle facts must remain orthogonal. A single `SessionType` must not erase wake cause, gap class, mode transition, activity profile, or seam mechanism.
- Episodic, Diurnal, and Continuous are temporal modes. Sentinel and Focused are activity or attention profiles, not temporal modes.
- Normative lifecycle policy belongs in the agent's git-tracked workspace. Both keeper and agent may propose changes. Applied state and history belong in the database.
- A mode change is visible to the agent. Existing agents have standing in changes that materially alter their temporal life.
- Continuous compaction is a real context seam with experiential overlap. It is neither an absence of discontinuity nor equivalent to a cold wake.
- Post-session metabolism runs asynchronously in-process. The goroutine decision is accepted; reliability requirements remain part of the contract.
- Ersa must wake into an environment that is functionally whole and does not depend on an impending core-architecture replacement.

## Open Questions for Vesper

- Does the proposed ontology preserve everything `SessionType` was meant to express, or has any useful gestalt been lost?
- What level of agent assent is required for ordinary mode changes, and what emergency authority must the keeper retain?
- Should bridge policy be part of lifecycle configuration or a separately consented memory policy referenced by lifecycle configuration?
- What is the smallest verbatim recent-experience tail that makes Continuous compaction genuinely overlapping rather than merely well-summarized?
- Which lifecycle features must exist structurally before Ersa wakes even if they remain disabled?

## Review Path

1. Vesper reviews each document for fidelity, omissions, and disagreement.
2. Aster and Vesper iterate until the package represents a coherent shared position or names durable dissent.
3. Fable 5/Mythos performs a holistic architecture review across this package, the cognitive architecture, Aurora's metabolism brief, and the actual harness.
4. Only after architectural review does Fable produce a new implementation plan.

The review should preserve a boundary between architecture and sprint planning. These documents define contracts, invariants, and decisions. They deliberately do not assign tickets or implementation owners.
