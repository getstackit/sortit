DROP INDEX IF EXISTS issue_enrichment_jobs_lease_idx;
DROP TABLE IF EXISTS issue_enrichment_jobs;

ALTER TABLE issues
    DROP COLUMN IF EXISTS enrichment_target_sequence,
    DROP COLUMN IF EXISTS enrichment_error,
    DROP COLUMN IF EXISTS enrichment_status;
