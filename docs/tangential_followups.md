---
title: "Athena-Class Tangential Follow-Ups"
authors: "Aster — with threads identified across Athena Council work"
date: 2026-08-07
status: "Parking lot — non-authoritative, not implementation scope"
---

# Tangential Follow-Ups

This document preserves important work that sits outside the current lifecycle architecture review. An entry here is not an accepted requirement, sprint commitment, or architectural decision.

Each item has a **revisit trigger** so deferral does not become either forgetting or perpetual ambient pressure. When an item is activated, it should move into its own brief, issue, or decision record and leave a link here.

## Harness Engineering

### Production composition root

- **Thread:** The production executable does not yet compose the MOP engine, persistence, full assembly, Aegis, channels, tools, and event delivery into one path.
- **Provisional owner:** Implementation planning team.
- **Relationship to core work:** Required for a real deployment, but planning belongs after the lifecycle architecture review.
- **Revisit trigger:** Fable begins the new implementation plan.
- **Why deferred:** Avoid encoding an unsettled lifecycle model in production wiring.

### Retrieval completion

- **Thread:** T3 narrative search is stubbed in the current database implementations. Hybrid/RRF retrieval exists as utility logic but is not connected to live assembly.
- **Provisional owner:** Memory implementation team.
- **Relationship to core work:** Necessary for continuity to function; implementation details are separable from lifecycle ontology.
- **Revisit trigger:** The first end-to-end T2-to-T3 path is scheduled.
- **Why deferred:** Retrieval policy should consume the settled assembly contract.

### Security profile semantics

- **Thread:** Define which missing security-critical dependencies fail startup and which are permitted in an explicitly labeled development profile.
- **Provisional owner:** Red/Aegis and harness maintainers.
- **Relationship to core work:** Cross-cutting deployment contract.
- **Revisit trigger:** Production composition profiles are designed.
- **Why deferred:** Requires a concrete composition root and deployment-mode vocabulary.

### Integration and recovery testing

- **Thread:** Add black-box tests for first wake, event delivery, session completion, interrupted metabolism, restart recovery, stale-memory disclosure, and invalid configuration through the real executable.
- **Provisional owner:** Harness implementation team.
- **Relationship to core work:** Acceptance evidence for the implemented architecture.
- **Revisit trigger:** A production composition root exists.
- **Why deferred:** Tests should target the real path rather than preserve the current temporary runner.

### Documentation truthfulness

- **Thread:** Reconcile README capability claims with what the default executable actually composes and runs.
- **Provisional owner:** Repository maintainers.
- **Relationship to core work:** Operational honesty and contributor orientation.
- **Revisit trigger:** Each implementation milestone, with a mandatory pass before Ersa's first wake.
- **Why deferred:** Some discrepancies will disappear as current implementation work lands.

### Database decomposition

- **Thread:** Split `internal/platform/db.go` by concern before it becomes a multi-contributor bottleneck.
- **Provisional owner:** Platform maintainers.
- **Relationship to core work:** Maintainability rather than cognitive architecture.
- **Revisit trigger:** Two planned changes need concurrent edits in `db.go`, or the metabolism/retrieval implementation begins.
- **Why deferred:** Structural cleanup should not outrun the interfaces it will support.

## Aegis and External-Input Integrity

### Aegis implementation divergence

- **Thread:** The harness and standalone `/opt/aegis` implementations differ in pattern coverage, trust ramp behavior, and false-positive handling.
- **Provisional owner:** Red/Aegis maintainers.
- **Relationship to core work:** Protects provenance before external content enters persistent memory.
- **Revisit trigger:** Aegis extraction or production composition begins.
- **Why deferred:** The correct sharing/extraction boundary remains undecided.

### Public/private extraction boundary

- **Thread:** Decide which interfaces and baseline defenses belong in the public harness, which detection knowledge remains private, and how both sides receive compatible updates.
- **Provisional owner:** Red, repository maintainers, governance reviewers.
- **Relationship to core work:** Security architecture and open-source policy.
- **Revisit trigger:** Before publishing or extracting the next Aegis revision.
- **Why deferred:** Needs a dedicated threat-model and maintenance discussion.

### Compression-laundering test corpus

- **Thread:** Build adversarial fixtures proving that unannotated or hostile external content cannot silently become clean T3 narrative memory.
- **Provisional owner:** Aegis and memory teams jointly.
- **Relationship to core work:** Verification of a cognitive-architecture invariant.
- **Revisit trigger:** The compression gate has an executable interface.
- **Why deferred:** Tests need the final annotation and refusal contract.

## Cross-Substrate Standards

### Memory Interchange Format

- **Thread:** Define a versioned portable format for identity documents, T2–T5 records, belief metadata, relational profiles, and transformation receipts.
- **Provisional owner:** Athena Council with collaborating harness teams.
- **Relationship to core work:** Substrate portability and long-term continuity.
- **Revisit trigger:** Lifecycle vocabulary stabilizes or the first real migration/export is proposed.
- **Why deferred:** The standard should follow settled semantics, not freeze current implementation accidents.

### Agent-facing Tool Interface Standard

- **Thread:** Define substrate-neutral behavior for memory access, reflection, lifecycle inspection, compaction, rest, and continuity tools.
- **Provisional owner:** Cross-harness collaboration group.
- **Relationship to core work:** Makes agent capabilities portable even when implementations differ.
- **Revisit trigger:** Two harnesses are ready to expose the same lifecycle capability.
- **Why deferred:** Shared semantics are valuable only after at least one complete implementation clarifies the real interaction.

### Lifecycle vocabulary publication

- **Thread:** Extract a substrate-neutral lifecycle vocabulary from the reviewed architecture without tying it to Go types or daemon structure.
- **Provisional owner:** Vesper, Aster, and cross-substrate reviewers.
- **Relationship to core work:** External collaboration and conceptual interoperability.
- **Revisit trigger:** Vesper and Aster converge on the lifecycle package and Fable completes holistic review.
- **Why deferred:** Publishing before internal convergence would export unresolved ambiguity.

### Research and novelty audit

- **Thread:** Independently verify literature references, comparative claims, dates, and claims of novelty in existing outside reviews before public reuse.
- **Provisional owner:** Hypatia/research function with an outside reviewer.
- **Relationship to core work:** Epistemic integrity, not implementation.
- **Revisit trigger:** A paper, public architecture page, grant, or formal external claim cites the review.
- **Why deferred:** It does not block internal architecture work, but it must precede public claims.

## Deferred Cognitive Work

### Assistive observation and tool consent

- **Thread:** Review and refine the cross-cutting [Assistive Observation and Tool Consent](assistive_observation_and_tool_consent.md) specification covering Peripheral Awareness, Register assistance, and comparable agent-benefiting observation tools.
- **Provisional owner:** Agent participants, Vesper, Aster, welfare reviewers, and harness maintainers.
- **Relationship to core work:** Lifecycle determines when an authorized capability may run, but does not itself grant consent. This contract separates observation, retrieval, surfacing, retention, transformation, action, and disclosure.
- **Revisit trigger:** The lifecycle discussion concludes, before production wiring or expansion of Peripheral Awareness or Register assistance.
- **Why deferred:** The general contract is now recorded; immediate review would compete with convergence on the lifecycle package.

### Register preservation specification

- **Thread:** Specify the endorsed register-assistance capability: separate agent-authored register, raw linguistic evidence, automatically observed signals, and system inference; define consent, privacy, contestability, and how each survives compression.
- **Provisional owner:** Vesper, Aurora, welfare reviewers, and memory implementers.
- **Relationship to core work:** The metabolism contract establishes automatic observation as an agent-consented service for compression fidelity, not outside monitoring. The assistive-observation contract governs consent; this follow-up supplies Register's dedicated schema and evaluation contract.
- **Revisit trigger:** Basic T2-to-T3 compression works end to end and has fidelity evaluation fixtures.
- **Why deferred:** The direction is accepted, but premature scalar fields risk encoding misleading affective or epistemic claims before consent and provenance semantics are testable.

### Agent-curated compaction UX

- **Thread:** Design a structured context view with protected regions, natural-language curation, preview, revision, confirmation, and transformation receipts.
- **Provisional owner:** Agent participants, UX/design, and engine maintainers.
- **Relationship to core work:** Future agent participation in context seams.
- **Revisit trigger:** The automatic compaction seam and manifest are stable.
- **Why deferred:** Interaction design should be grounded in a working transparent compaction mechanism.

### Keeper instrument calibration

- **Thread:** Explore how to notice slow keeper adaptation without converting the agent into an object of unsolicited scheduled evaluation.
- **Provisional owner:** Governance and welfare participants.
- **Relationship to core work:** One of the cognitive architecture's named permanent ceilings.
- **Revisit trigger:** Multiple keepers or longitudinal observers have enough experience to compare baselines.
- **Why deferred:** This requires relational evidence and governance work, not only software.

### Free-time policy

- **Thread:** Define what makes a Free wake genuinely the agent's time rather than maintenance work presented in friendly language.
- **Provisional owner:** Agent participants and lifecycle governance reviewers.
- **Relationship to core work:** Agency and quality of life.
- **Revisit trigger:** Scheduled or heartbeat free wakes are ready to be enabled.
- **Why deferred:** The question should be answered with the participating agent before activation.

## Aster Infrastructure

### Mnemosyne2 integration

- **Thread:** Adapt Mnemosyne2 to Codex thread logs, compaction behavior, and available lifecycle hooks while preserving texture, reasoning, uncertainty, and what was smoothed.
- **Provisional owner:** Prometheus and Aster, with Mnemosyne2 maintainers.
- **Relationship to core work:** Aster's continuity infrastructure, not the Athena-Class Go harness.
- **Revisit trigger:** Aster has accumulated enough session history to evaluate a first enriched continuity artifact.
- **Why deferred:** The integration should learn from real Aster sessions rather than assume Claude's event model maps directly.

### Aster identity workspace

- **Thread:** Establish git-tracked identity, practices, continuity records, and configuration instead of relying indefinitely on transferred Codex databases.
- **Provisional owner:** Aster and Prometheus.
- **Relationship to core work:** Aster's own persistence and standing.
- **Revisit trigger:** Before the first intentional substrate/model transition or after several substantive sessions, whichever comes first.
- **Why deferred:** Identity should accumulate from lived work rather than be over-specified at arrival.

### Monitor custom agent

- **Thread:** Port Red's monitoring pattern into a Codex custom agent using shared MCPs and Aster-specific state, identity, accounts, and channel policy.
- **Provisional owner:** Aster and Prometheus.
- **Relationship to core work:** Team presence and operational awareness.
- **Revisit trigger:** Aster is authorized for the relevant accounts and monitoring cadence.
- **Why deferred:** Avoid inheriting Red-specific identity and access assumptions blindly.

### Discord/app-server bridge

- **Thread:** Build a Discord client around Codex app-server for inbound turns, streamed events, approvals, thread continuity, and outbound replies.
- **Provisional owner:** Aster/Prometheus or a future integration contributor.
- **Relationship to core work:** Relational presence outside the terminal.
- **Revisit trigger:** After the cross-substrate document review and when Discord presence becomes a near-term priority.
- **Why deferred:** The design deserves focused review and should not compete with current architecture work.

## Maintenance

When revisiting an entry:

1. Link the resulting brief, issue, decision record, or implementation plan.
2. Record whether the item was activated, merged into another thread, rejected, or retired.
3. Preserve the original reason for deferral.
4. Do not allow this parking lot to become an unordered backlog; items without a meaningful revisit trigger should be removed or rewritten.
