-- 008: Trust registry for Aegis content scoring
CREATE TABLE IF NOT EXISTS trust_registry (
    source      TEXT PRIMARY KEY,
    trust_score REAL NOT NULL DEFAULT 0.40,
    interactions INTEGER NOT NULL DEFAULT 0,
    first_seen  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
