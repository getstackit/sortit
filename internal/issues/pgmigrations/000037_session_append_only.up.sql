-- Session append-only lifecycle facts.
--
-- session_facts is the canonical append-only log of session lifecycle events
-- (created / invalidated). sessions stays as the active-session projection,
-- holding only currently-active sessions for fast lookup by token_hash on the auth
-- read path (LookupSession). Logout no longer DELETEs history destructively: it
-- appends an 'invalidated' fact and removes the projection row.
--
-- Deliberately no FOREIGN KEY to sessions(id): the projection row is deleted on
-- logout, so the audit trail must live independently of it. (This is the case the
-- no-FK convention from api_token_facts / user_profile_facts was designed for.)
CREATE TABLE session_facts (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    sequence BIGINT NOT NULL,
    kind TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_at_unix_nano BIGINT NOT NULL,
    source TEXT NOT NULL DEFAULT '',
    source_id TEXT NOT NULL DEFAULT '',
    inferred BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE UNIQUE INDEX session_facts_session_sequence_idx
ON session_facts(session_id, sequence);

CREATE UNIQUE INDEX session_facts_source_idx
ON session_facts(source, source_id);

CREATE INDEX session_facts_session_created_idx
ON session_facts(session_id, created_at_unix_nano, sequence, id);

CREATE INDEX session_facts_user_created_idx
ON session_facts(user_id, created_at_unix_nano, id);
