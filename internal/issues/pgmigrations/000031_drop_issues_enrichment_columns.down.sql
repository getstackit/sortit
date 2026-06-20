-- Re-add the legacy enrichment columns and backfill them from the canonical
-- enrichment projection so the issues table is restored to its prior shape.
ALTER TABLE issues
    ADD COLUMN IF NOT EXISTS enrichment_status TEXT NOT NULL DEFAULT 'complete',
    ADD COLUMN IF NOT EXISTS enrichment_error TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS enrichment_target_sequence BIGINT NOT NULL DEFAULT 1;

UPDATE issues
SET enrichment_status = COALESCE(p.status, 'complete'),
    enrichment_error = COALESCE(p.error, ''),
    enrichment_target_sequence = COALESCE(p.target_sequence, 1)
FROM issue_enrichment_projections p
WHERE p.issue_id = issues.id;
