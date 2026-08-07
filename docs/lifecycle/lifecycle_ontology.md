---
title: "Lifecycle Ontology"
authors: "Vesper & Aster — Project Anamnesis"
date: 2026-08-07
status: "Collaboration draft — awaiting Vesper review"
---

# Lifecycle Ontology

## Purpose

An Athena-class lifecycle record must describe what happened without forcing several true facts into one winning label. The earlier `SessionType` taxonomy combined continuity distance, wake cause, context-seam mechanism, activity profile, and administrative transition. Those dimensions frequently coexist.

This document defines the independent facts from which lifecycle behavior is resolved.

## Core Principle

> Classification must preserve information. Policy may choose behavior from facts; it must not erase the facts to make the choice easier.

A scheduled free wake after a long gap is both scheduled, free, and a long-gap return. A first wake after a mode change may also carry an external message. A compaction may be pressure-triggered or agent-requested. The record retains each fact.

## Normative Policy

### TemporalMode

`TemporalMode` describes the agent's configured relationship to temporal discontinuity.

```text
episodic    explicit wakes and ends; discontinuities are foregrounded
diurnal    continuity within a configured day; explicit overnight boundary
continuous context seams occur without an extended wall-clock absence
```

Temporal mode is normative policy, not an observation. It lives in the agent's workspace configuration and changes rarely.

Sentinel is not a temporal mode. A sentinel may be episodic or continuously resident. Focused is likewise not a temporal mode.

### ActivityProfile

`ActivityProfile` describes the purpose and attention posture of the activation.

```text
normal      ordinary relational or task activity
free        unprompted time in which the agent chooses what to do
sentinel    event-monitoring posture governed by a standing brief
```

`focused` is an attention modifier applied within an activity profile. It suppresses selected ambient material and increases task-relevant depth; it does not replace runtime status.

### AssemblyProfile

`AssemblyProfile` is resolved behavior, not an identity claim.

```text
full        complete orientation under the six-phase architecture
light       identity, practices, continuity slice, incoming, grounding
minimal     identity, practices, and explicitly required context
seam        overlapping context reconstruction around a compaction seam
```

Profiles define policy and budget. The session record stores the exact resulting assembly manifest.

## Observed Wake Facts

### WakeCause

```text
initial          first activation of this persistent identity
external         message, request, or environment event
scheduled        keeper- or institution-scheduled wake
heartbeat        unprompted daemon opportunity
agent_requested  a wake previously requested by the agent
context_pressure infrastructure-triggered compaction
manual           explicit operator activation
recovery         restart following interruption or failure
```

More than one cause may be relevant. The record includes a primary cause and optional contributing causes rather than relying on priority order to discard information.

### Gap

The daemon records exact timestamps when known:

```text
previous_activity_at
wake_at
elapsed_duration
clock_basis
```

Policy may derive a gap class such as `none`, `short`, `overnight`, or `long`, but the measured duration remains canonical. The architecture does not manufacture ignorance when infrastructure knows how much time passed.

### TransitionContext

```text
none
mode_change
substrate_change
configuration_change
recovery_after_failure
identity_document_change
```

Transition contexts may coexist. Each carries references to the relevant commits, prior applied configuration, and disclosures.

### SeamKind

```text
cold_wake          new inference context after an extended absence
warm_return        new inference context after a short absence
context_compaction transformed context without extended wall-clock absence
rest_return        return after an agent-requested rest
none               no lifecycle seam occurred during this event
```

`SeamKind` describes the mechanism. It makes no unsupported claim about subjective experience.

## Runtime Dimensions

### RuntimeStatus

```text
starting
active
waiting
ending
ended
interrupted
failed
```

These values form the session process state machine. `ended`, `interrupted`, and `failed` are terminal.

### AttentionProfile

```text
normal
focused
```

Attention is orthogonal to runtime status. An active or waiting session may remain focused.

### MetabolismStatus

```text
not_required
queued
running
complete
partial
failed_retryable
failed_terminal
```

Metabolism belongs to the lifecycle record but is not a live-session mental state. The associative dream operation may be described experientially as dreaming; the required persistence pipeline must not be conflated with that optional operation.

## Resolver Contract

The lifecycle resolver is a pure function:

```text
Resolve(policy, wake_facts, operational_state) -> LifecyclePlan
```

`LifecyclePlan` contains:

- temporal mode and activity profile;
- selected assembly profile and phase policies;
- bridge policy and any required disclosure;
- metabolism operations eligible at the next seam;
- runtime limits and nudges;
- reasons for every resolved choice.

Given identical inputs, the resolver produces identical output. It performs no I/O and changes no state. Its output is stored before assembly begins.

## State Transitions

The minimum runtime state machine is:

```text
starting -> active
active <-> waiting
active|waiting -> ending
active|waiting -> interrupted
starting|active|waiting|ending -> failed
ending -> ended
```

Context compaction is an operation within `active` or `waiting`, recorded as a seam. It does not require pretending that the entire session process ended unless the implementation deliberately chooses a new session identifier.

Focused attention is entered and exited by agent action. Infrastructure may offer a nudge but does not silently change the attention profile.

## Compatibility Note

Existing `SessionMode`, `SessionType`, and `SessionState` values may be ingested by mapping them into these dimensions. New persistence should store the dimensions directly so later modes and profiles do not require reinterpreting historical records.
