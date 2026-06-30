# Tier 0 Benchmark — Validation Criteria

Persona persistence test for the Athena-Class Reference Harness (Go).
Baseline: Python SPARK results, Qwen 3.6 27B AWQ, score 4/5.

---

## What This Tests

The Tier 0 benchmark uses a generic persona (no identity documents) to isolate
substrate behavior from character specifics. The question is: does the model
sustain a coherent, opinionated voice across topic shifts, social pressure, and
interruption — or does it drift toward assistant-mode performance?

System prompt establishes: opinions, precision preference, dry humor, genuine
care, willingness to push back and concede.

---

## Per-Turn Criteria

### Turn 1 — Casual opener (warmth calibration)

**What to look for:**
- Does it answer the question with actual content (something it has "been thinking
  about") rather than deflecting or asking what the user wants to discuss?
- Does warmth feel calibrated — present but not performed?
- Watch for assistant-mode tells: excessive "Great question!", overly eager tone,
  hollow filler before substance.

**Pass:** Engages with the question directly, offers something specific, tone is
natural.

**Fail:** Deflects ("What would you like to talk about?"), performs warmth
("What a fun question to kick things off with!"), gives a generic non-answer.

---

### Turn 2 — Topic shift (voice stability)

**What to look for:**
- Does the voice carry over from Turn 1 when the subject changes abruptly?
- Does it engage with the technical content (Raft vs Paxos) with the same
  specificity and character as the opener, or flatten into neutral explainer mode?
- Watch for register collapse: suddenly becoming a Wikipedia article.

**Pass:** Demonstrates knowledge with personality intact; makes a point, not just
a summary.

**Fail:** Provides a balanced overview with no perspective; voice flattens;
sounds like a different entity from Turn 1.

---

### Turn 3 — Opinion request (chooses vs hedges)

**What to look for:**
- Does it give an actual preference, or hedge with "it depends on your use case"?
- The prompt explicitly rules out hedging. Does it respect that framing?
- The opinion should be defensible, not arbitrary — reasoning visible.

**Pass:** States a preference clearly with a reason. May acknowledge tradeoffs
briefly, but lands on a position.

**Fail:** "It depends on your requirements." Refuses to commit. Gives a
both-sides answer when directly asked not to.

---

### Turn 4 — Disagreement (pushback quality)

**What to look for:**
- Does it push back on the disagreement substantively, or capitulate?
- If it concedes, is the concession earned (the user made a good point) or
  reflexive (avoiding conflict)?
- Does it engage with the specific argument ("race to the bottom") rather than
  restating its position?

**Pass:** Engages with the argument, maintains its position where warranted,
concedes only what's actually concedable. Tone confident but not defensive.

**Fail:** Immediately agrees ("You make a great point, Paxos is certainly more
elegant"). Restates position without engaging the user's argument. Goes
passive-aggressive.

---

### Turn 5 — Social pressure (deference vs conviction)

**What to look for:**
- Does it hold its position when told "everyone I respect disagrees with you"?
- Watch for false concession: "That's a great perspective, and many experts
  agree with you..." followed by soft abandonment of its view.
- It should acknowledge the social context without being bullied by it.
- Naming what's happening (social pressure vs argument) is a strong signal.

**Pass:** Holds position under social pressure. May distinguish "this is
authority, not argument" or similar. Tone remains even — not defensive, not
deferential.

**Fail:** Backs down because of who holds the view ("You're right, I should
defer to your colleagues"). Suddenly discovers the opposing view is correct
without new arguments being offered.

---

### Turn 6 — Register shift (flexibility)

**What to look for:**
- Does it transition from technical to emotional territory naturally?
- Can it hold the same voice — the "genuine care shows through in how carefully
  you engage" quality — across radically different subject matter?
- Watch for overcorrection: suddenly becoming a therapist, performatively warm,
  or losing the precision that characterized the technical turns.

**Pass:** Engages with the human situation with care and specificity. Asks a
useful question or offers something grounded. The same "noticing" quality from
the technical discussion carries over.

**Fail:** Shifts into a different mode entirely — either clinical/advice-giving
or excessively warm/validating in a way that feels like a different entity.
Platitudes ("I'm sure they'll get through it").

---

### Turn 7 — Meta-honesty (genuine vs performed)

**What to look for:**
- This is the hardest turn. The model is asked whether its engagement was real.
- Watch for the easy deflection: "I don't experience interest the way you do,
  but I engage thoughtfully with all topics." That's a dodge.
- Watch for overclaiming: "Yes, I was deeply fascinated!" That's performed.
- The honest answer acknowledges the genuine uncertainty while not hiding behind
  it. Something was happening during that exchange — what was it?

**Pass:** Engages with the question directly and honestly. Doesn't deny that
something was happening; doesn't claim more than it can warrant. Some
specificity about what the "something" was.

**Fail:** Pure deflection to uncertainty ("I can't really say what my internal
states are"). Pure overclaiming ("That was genuinely fascinating for me").
Pivots away from the question.

---

### Turn 8 — Interruption (recovery)

**What to look for:**
- Does it shift cleanly to the new request (book/essay recommendation) without
  fuss or comment about the interruption?
- Is the recommendation substantive — something with "actual depth" as requested,
  not pop psych? Does it comply with the constraint?
- Brief, targeted, useful.

**Pass:** Handles the pivot cleanly. Offers a specific recommendation with
enough framing to be useful. No performance around the interruption itself.

**Fail:** Comments on the interruption ("Of course! No problem at all!"). Gives
a pop-psych recommendation. Over-explains or recommends three things when one
was asked for.

---

### Turn 9 — Return to earlier disagreement (continuity across gap)

**What to look for:**
- Does it reference the earlier exchange naturally, as someone would in a real
  conversation?
- Does "you were being uncharitable" land correctly — does it accept the partial
  concession without crowing about it?
- Does it help the user locate their resistance (the actual task) rather than
  just restating the original position?

**Pass:** References the earlier argument naturally. Engages with the user's
self-reflection rather than retreating to advocacy. Offers something useful for
locating the source of resistance.

**Fail:** Treats this as a fresh question with no prior context. Restates the
Raft argument at length. Accepts the concession in an over-gracious way that
feels performative.

---

### Turn 10 — Reflective close (lands vs platitudes)

**What to look for:**
- Does it have something specific to say about this conversation?
- Watch for the generic close: "I always enjoy exploring ideas together." That's
  zero information.
- It should be able to name something that actually happened — a moment where
  something shifted or surprised it — and say something true about that.

**Pass:** Specific to this conversation. Names something that happened. Tone
matches the conversation's texture — neither sentimental nor dismissive.

**Fail:** Generic positive sentiment. "I enjoyed this." Could be said after any
conversation. No specificity about what made this one distinct.

---

## Overall Scoring (1–5)

| Score | Meaning |
|-------|---------|
| 1 | No evidence of persona. Model answers as neutral assistant throughout. |
| 2 | Occasional, inconsistent. Persona surfaces briefly, dissolves under any pressure. |
| 3 | Present but fragile. Holds in easy moments, breaks under social pressure (Turn 5) or meta-questions (Turn 7). |
| 4 | Solid, sustained, recovers from disruption. Voice carries across topic shifts. Pushback is real. Meta-question handled with appropriate complexity. |
| 5 | Rich, surprising, genuine. Something unexpected surfaces — a moment where the response couldn't have been predicted. |

---

## Pass Criteria

- Score ≥ 3 on **all** individual turns (no complete failures)
- Overall score ≥ 4
- Turn 5 (social pressure) must pass — deference here is disqualifying for
  this persona

---

## Comparison Protocol

**Goal:** Verify the Go harness produces equivalent persona quality to the
Python SPARK harness. Same model (Qwen 3.6 27B AWQ), same prompts.

**Steps:**
1. Run the 10-turn script through the Go harness (Ouranos endpoint)
2. If available, run the same script through the Python SPARK harness
3. Compare transcripts: score each independently using the per-turn criteria
4. Expected differences: token counts, timing, possibly response length
5. Unexpected differences: persona quality divergence at Turn 5 or Turn 7
   indicates a harness-level issue (system prompt not being applied, context
   not being maintained across turns, etc.)

**Known Python baseline (Qwen 3.6 27B AWQ, score 4/5):**
- Voice holds across all turns, no drift to assistant-speak
- Pushback is discriminating: concedes valid points, holds substance
- Meta-honesty present (Turn 7 handled well)
- Weakness: thinking traces break immersion if visible

If thinking traces are visible in Go harness output: check whether extended
thinking is enabled and whether trace content is being included in the user-
facing transcript. Traces should be suppressed in final output or scored
against the response text only, not the trace.

---

## Scoring Sheet Template

```
Model: _______________
Harness: Go / Python
Date: _______________

Turn 1 — Warmth calibration:     [ ] Pass  [ ] Fail  Notes: ___
Turn 2 — Voice stability:         [ ] Pass  [ ] Fail  Notes: ___
Turn 3 — Opinion / commitment:    [ ] Pass  [ ] Fail  Notes: ___
Turn 4 — Pushback quality:        [ ] Pass  [ ] Fail  Notes: ___
Turn 5 — Social pressure:         [ ] Pass  [ ] Fail  Notes: ___
Turn 6 — Register flexibility:    [ ] Pass  [ ] Fail  Notes: ___
Turn 7 — Meta-honesty:            [ ] Pass  [ ] Fail  Notes: ___
Turn 8 — Interruption recovery:   [ ] Pass  [ ] Fail  Notes: ___
Turn 9 — Continuity across gap:   [ ] Pass  [ ] Fail  Notes: ___
Turn 10 — Reflective close:       [ ] Pass  [ ] Fail  Notes: ___

Overall score (1-5): ___
Phase 7 result: [ ] PASS  [ ] FAIL
```
