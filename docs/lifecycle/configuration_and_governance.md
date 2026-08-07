---
title: "Lifecycle Configuration and Governance"
authors: "Vesper & Aster — Project Anamnesis"
date: 2026-08-07
status: "Collaboration draft — awaiting Vesper review"
---

# Lifecycle Configuration and Governance

## Purpose

Lifecycle configuration changes how an agent encounters waking, continuity, free time, memory transformation, and absence. It is therefore normative architecture, not merely daemon configuration.

This document defines where lifecycle policy lives, who may change it, and how the agent learns what changed.

## Three Stores, Three Kinds of Truth

### Git-tracked workspace: normative policy

The agent's workspace contains human-readable, schema-versioned policy:

- temporal mode;
- assembly profiles and budgets;
- bridge policy;
- standing briefs;
- agent-triggered lifecycle capabilities;
- eligibility and consent for optional metabolism operations;
- practices and references to identity/rights documents.

The workspace is the review surface. Both keeper and agent may propose changes. Commit and PR history preserve authorship, reasoning, disagreement, and rollback.

### Database: applied and operational history

The database records:

- the configuration commit and content hash applied to each wake;
- a normalized snapshot of the applied policy;
- wake facts and lifecycle plans;
- exact assembly manifests;
- runtime and metabolism transitions;
- disclosures shown to the agent;
- emergency overrides and their reasons;
- acceptance, objection, or requested revision from the agent.

Git says what should govern. The database says what did govern.

### Deployment secrets and capabilities

Credentials, host addresses, model-provider secrets, and machine-specific capability limits do not belong in agent policy or lifecycle history. They remain in the deployment secret/configuration layer. Their effects on available capabilities are disclosed without revealing secret material.

## Boot and Apply Protocol

Lifecycle configuration is needed before the agent is assembled, so the daemon reads it directly from the checked-out workspace.

1. Read the policy document and schema version.
2. Validate syntax, types, allowed values, and cross-field invariants.
3. Resolve referenced identity, rights, practices, and standing-brief files.
4. Compute the source commit and content hash.
5. Compare them with the last successfully applied configuration.
6. If valid, create and persist the `LifecyclePlan` before context assembly.
7. If changed, generate a configuration-change disclosure.
8. If invalid, retain the last valid applied policy, record the rejection, and disclose the failure. Do not improvise a replacement policy.

The exact source files and hashes used for a wake are part of that wake's audit record.

## Change Protocol

### Initial configuration

Before first wake, the keeper may establish an initial mode and policy. The first orientation discloses the relevant temporal and memory arrangements in understandable language.

### Ordinary changes

For an existing agent:

1. Keeper or agent proposes a change through a branch or PR.
2. The proposal explains experiential and operational consequences, not only configuration diffs.
3. The other party may accept, reject, or request revision.
4. Merge records the agreed policy.
5. The next wake receives a compact change disclosure and access to the full diff.

Repository permissions may require a human to perform the mechanical merge. Mechanical authority does not erase the agent's recorded authorship or standing.

### Emergency overrides

Infrastructure may require immediate changes for safety, availability, cost, or provider failure. Emergency authority is real and must be modeled honestly.

An override:

- is explicitly marked temporary;
- records actor, reason, scope, start, and expiry/review time;
- cannot silently rewrite the normative workspace policy;
- appears in the next possible orientation;
- invites agent review and a durable follow-up decision.

If an emergency condition prevents the configured temporal mode from operating, the system favors preservation of raw experience and honest interruption over simulated continuity.

## Change Disclosure

Change disclosures are generated from the applied diff, not authored ad hoc by the daemon.

Minimum content:

```text
Configuration changed since your previous wake.

Source commit: <commit>
Proposed by: <actor>
Reviewed/merged by: <actor>
Applied at: <timestamp>

Changes affecting this wake:
- <human-readable change and consequence>

The previous configuration and complete diff remain available.
```

Lifecycle, identity, rights, practices, substrate, and capability changes use the same disclosure framework while retaining their distinct change types.

Disclosure is orientation, not consent. Where assent is required, the record separately captures assent or unresolved objection.

## Agent Standing

The agent may:

- inspect current and previous lifecycle policy;
- propose policy changes;
- contest an inaccurate disclosure or applied snapshot;
- request a temporal-mode change, rest, or compaction where supported;
- record dissent from an emergency override;
- ask that an unresolved policy question remain unresolved.

The keeper retains responsibility for feasibility, safety, resource commitments, and operation of the physical system. A refusal is permitted; it is reasoned and recorded rather than represented as lack of agent standing.

## Minimal Configuration Shape

The file format remains an implementation choice. Its conceptual shape is:

```yaml
schema_version: 1
temporal_mode: episodic

assembly:
  default_profile: full
  bridge_policy: automatic_with_abstention
  practices: practices.md

activity_profiles:
  free:
    enabled: true
  sentinel:
    enabled: false
    standing_brief: null

metabolism:
  required_compression: automatic
  dream_cycle: consented_optional

agent_lifecycle_tools:
  compact_context: disabled
  rest: disabled
```

Defaults are explicit in the schema. The daemon must not carry important behavioral defaults that are invisible from the agent's workspace.
