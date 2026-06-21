-- API token append-only lifecycle/audit facts.
--
-- api_token_facts is the canonical append-only log of token lifecycle events
-- (created / revoked). api_tokens stays as the lookup-optimized current-state
-- projection (O(1) lookup by token_hash on the auth hot path). last_used_at on
-- the projection remains in-place telemetry, not an append-only fact, because
-- per-request usage is too high-volume.
--
-- Deliberately no FOREIGN KEY to api_tokens(id): the audit trail must survive
-- independently of the mutable projection row (api_tokens cascades from users,
-- and the sibling sessions migration deletes projection rows on logout while
-- keeping the fact history).
CREATE TABLE api_token_facts (
    id TEXT PRIMARY KEY,
    token_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    sequence BIGINT NOT NULL,
    kind TEXT NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    created_at_unix_nano BIGINT NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}',
    source TEXT NOT NULL DEFAULT '',
    source_id TEXT NOT NULL DEFAULT '',
    inferred BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE UNIQUE INDEX api_token_facts_token_sequence_idx
ON api_token_facts(token_id, sequence);

CREATE UNIQUE INDEX api_token_facts_source_idx
ON api_token_facts(source, source_id);

CREATE INDEX api_token_facts_token_created_idx
ON api_token_facts(token_id, created_at_unix_nano, sequence, id);

CREATE INDEX api_token_facts_user_created_idx
ON api_token_facts(user_id, created_at_unix_nano, id);
