-- Retire the legacy enrichment state columns on the issues table. Enrichment
-- state is now canonical in the append-only facts + issue_enrichment_projections
-- (migration 000022); the issues columns are dead compatibility fallback that is
-- no longer written and has drifted from the projection.
ALTER TABLE issues
    DROP COLUMN IF EXISTS enrichment_status,
    DROP COLUMN IF EXISTS enrichment_error,
    DROP COLUMN IF EXISTS enrichment_target_sequence;
