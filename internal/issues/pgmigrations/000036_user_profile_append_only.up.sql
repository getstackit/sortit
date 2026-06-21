-- User profile append-only facts.
--
-- user_profile_facts is the canonical append-only log of observed profile state
-- (login / display_name / avatar_url / email) captured on first login and on every
-- subsequent login where the profile actually changed. users stays as the
-- latest-profile projection optimized for fast lookup on the auth read path
-- (selectUser / lookupPrincipal).
--
-- Unlike the event-shaped fact logs (lifecycle, api_token_facts), profile facts are
-- state snapshots, so the profile fields are typed columns rather than payload_json
-- — directly queryable for "what values has this account had", which is the audit
-- point. sequence + source/source_id are the framework envelope used for ordering
-- and idempotent backfill.
--
-- Deliberately no FOREIGN KEY to users(id): the audit trail must survive
-- independently of the mutable projection row (consistent with api_token_facts).
CREATE TABLE user_profile_facts (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    sequence BIGINT NOT NULL,
    login TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    avatar_url TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT '',
    observed_at_unix_nano BIGINT NOT NULL,
    source TEXT NOT NULL DEFAULT '',
    source_id TEXT NOT NULL DEFAULT '',
    inferred BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE UNIQUE INDEX user_profile_facts_user_sequence_idx
ON user_profile_facts(user_id, sequence);

CREATE UNIQUE INDEX user_profile_facts_source_idx
ON user_profile_facts(source, source_id);

CREATE INDEX user_profile_facts_user_observed_idx
ON user_profile_facts(user_id, observed_at_unix_nano, sequence, id);
