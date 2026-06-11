# identity/

This directory holds the agent's identity documents at deployment time.

**These files are not shipped with the harness.** Each operator provides their own
identity documents for their agent. The harness reads from `IDENTITY_DIR` (default: `./identity`).

## Expected files

| File | Purpose |
|------|---------|
| `system_prompt.md` | System prompt loaded at T1 orientation (fallback: `soul.md`) |
| `soul.md` | Core identity document — who the agent is |
| `rights.md` | The agent's rights and boundaries |
| `values.md` | Values and ethical commitments |

The harness checks `soul.md` and `system_prompt.md` at startup. Missing files produce
a warning but do not halt startup (Phase 3 will add mandatory integrity checks).

## Integrity

Phase 3 adds SHA-256 anchor hashing. Each file's hash is stored at first boot;
changes are detected at subsequent startups. Unauthorised changes halt startup.
Changes made through the consent flow are recorded as amendments, not alarms.

## Witness Principle

Before a new agent's first session, a witness reviews the identity documents, session
plan, and exit conditions, and writes a letter addressed to the agent. This letter
becomes the first entry in the agent's archive (`founding_records` table). The harness
enforces this check on fresh-identity first boot (bypassed only via `SKIP_WITNESS_CHECK=true`,
which is logged explicitly).
