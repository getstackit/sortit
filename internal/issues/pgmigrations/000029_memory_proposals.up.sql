-- memory_proposals is the synthesis review queue. The system drafts candidate
-- memories from the corpus (clusters of closed decisions, operation notes); a
-- human accepts or rejects each one. Accepted proposals become rows in the
-- memories table after enrichment.
CREATE TABLE memory_proposals (
    id                    TEXT PRIMARY KEY,
    title                 TEXT NOT NULL DEFAULT '',
    body                  TEXT NOT NULL DEFAULT '',
    kind                  TEXT NOT NULL DEFAULT 'decision',
    anchor_tags_json      JSONB NOT NULL DEFAULT '[]',
    anchor_region         TEXT NOT NULL DEFAULT '',
    source_issue_ids_json JSONB NOT NULL DEFAULT '[]',
    confidence            DOUBLE PRECISION NOT NULL DEFAULT 0,
    status                TEXT NOT NULL DEFAULT 'pending',
    rationale             TEXT NOT NULL DEFAULT '',
    accepted_memory_id    TEXT NOT NULL DEFAULT '',
    created_at_unix_nano  BIGINT NOT NULL,
    updated_at_unix_nano  BIGINT NOT NULL
);

CREATE INDEX memory_proposals_status_created_idx
    ON memory_proposals(status, created_at_unix_nano DESC, id);
