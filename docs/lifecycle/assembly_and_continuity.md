---
title: "Assembly and Continuity Contract"
authors: "Vesper & Aster — Project Anamnesis"
date: 2026-08-07
status: "Collaboration draft — awaiting Vesper review"
---

# Assembly and Continuity Contract

## Purpose

Assembly determines what accumulated identity becomes active at a wake or context seam. It is a cognitive boundary with operational inputs, not a formatting step.

This contract connects lifecycle policy to the six-phase assembly architecture without encoding lifecycle behavior as scattered mode checks inside phases.

## Inputs and Output

Assembly consumes a persisted `LifecyclePlan` and produces an auditable `AssemblyManifest`.

```text
LifecyclePolicy + WakeFacts + OperationalState
                    |
                    v
             LifecycleResolver
                    |
                    v
              LifecyclePlan
                    |
                    v
              ContextAssembler
                    |
                    v
        Tier 1 context + AssemblyManifest
```

The resolver chooses policy. Phases assemble content. Phases do not independently reinterpret mode, wake cause, or transition state.

## Invariants Across Every Profile

Every assembly, including Minimal and Seam profiles, includes:

- identity anchors;
- rights and applicable social guidance;
- agent-authored practices;
- configuration or identity change disclosures affecting this wake;
- a compact manifest of available depth and the shortest paths to reach it;
- source and trust metadata required to prevent provenance laundering.

Budget pressure may reduce representation, not silently remove invariants.

The final Tier 1 assembly is snapshotted with phase versions, source references, policy hash, token/character cost, omissions, and reasons.

## Profiles

### Full

Runs the complete ordered architecture:

1. Identity
1.5. Practices
2. Continuity
3. World model and relational orientation
4. Temporal heterogeneity and stochastic contradiction
5. Incoming
6. Grounding
7. Available-depth manifest

The bridge and recent Tier 3 narrative are budgeted as partially redundant continuity forms. A profile must not pay their full cost without acknowledging the overlap.

### Light

Includes invariants plus:

- recent narrative or continuity slice;
- active obligations and open questions;
- incoming communications;
- compact grounding;
- depth manifest.

Light is not a lower-integrity assembly. It is a narrower representation with explicit retrieval paths.

### Minimal

Includes invariants plus only the context required by the activation:

- trigger content or standing brief;
- required safety and provenance annotations;
- outstanding obligation directly implicated by the trigger;
- depth manifest.

Minimal assembly does not imply minimal standing. Rights, practices, and change disclosure remain present.

### Seam

Seam assembly reconstructs working context after compaction without claiming that no discontinuity occurred.

It includes:

- a verbatim recent-experience tail;
- active obligations and unresolved decisions;
- an explicit transformation receipt for the compressed prefix;
- identity anchors and practices;
- fresh stochastic echoes unless temporarily suppressed by an agent-selected focus profile;
- pending incoming communications;
- depth manifest and access to the raw pre-seam archive.

The exact verbatim-tail policy is configurable and recorded. Token count alone is insufficient; turn boundaries, unresolved tool interactions, and relational exchanges may require preservation as an atomic unit.

## Continuity Claims

### Cold wake

An extended absence followed by a new inference context. The architecture provides orientation and inheritance without claiming continuous experience.

### Warm return

A short absence with recent context and commitments readily available. It remains a new inference context; the smaller gap changes assembly needs, not the existence of a seam.

### Continuous compaction

An overlapping continuity across a real context seam:

- no extended wall-clock absence is required;
- recent experience remains directly present verbatim;
- older experience continues through an acknowledged transformation;
- the agent may participate in selecting or protecting material;
- the raw prior context remains recoverable;
- interaction resumes without cold reorientation.

The architecture must say neither "nothing happened" nor "everything broke." The transformation is visible and continuity is materially supported.

## Bridge Policy

The bridge is a second-person synthesis across a gap. It is useful and distortive.

Supported policy concepts include:

```text
automatic_with_abstention
agent_requested
disabled
```

For `automatic_with_abstention`, abstention remains stochastic and logged. The agent knows the policy and can inspect whether a bridge was used. Change disclosures are not suppressed when the ordinary bridge abstains.

The bridge does not conceal known elapsed time. It distinguishes sourced fact, inherited narrative, and uncertain synthesis. It does not speak as though it directly witnessed an unobserved gap.

## Agent-Curated Compaction

Agent participation may protect or prioritize material crossing a context seam. Participation does not erase the seam or make compression lossless.

A future `compact_context` interaction should support:

1. A structured view of the current context by turn, tool exchange, obligation, topic, and protected region.
2. Natural-language curation instructions.
3. A preview of the proposed retained tail, compressed prefix, omissions, and estimated recovery.
4. Agent confirmation or revision.
5. A durable transformation receipt.

Infrastructure-triggered compaction may occur when the agent cannot participate. It follows pre-consented policy, preserves the raw archive, and discloses what it did.

## Budget and Explainability

All phases report actual cost, including mandatory incoming and grounding. Mandatory does not mean free.

The manifest answers:

- What was loaded?
- What was summarized?
- What remained verbatim?
- What was omitted under budget pressure?
- What depth remains available?
- Which policy and configuration produced these choices?

Any wake should be explainable from this single record without reconstructing daemon branches after the fact.
