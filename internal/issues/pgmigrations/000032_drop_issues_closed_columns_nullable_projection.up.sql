-- Retire the legacy closed_* columns on the issues table. Lifecycle state is
-- canonical in issue_lifecycle_projections (migration 000021); the issues
-- columns are dead compatibility fallback: no longer written and already
-- drifted from the projection.
ALTER TABLE issues
    DROP COLUMN IF EXISTS closed_at_unix_nano,
    DROP COLUMN IF EXISTS closed_by,
    DROP COLUMN IF EXISTS closed_reason,
    DROP COLUMN IF EXISTS closed_reason_note;

-- Express "not closed" naturally as NULL on the canonical lifecycle projection
-- instead of sentinel defaults (0 / ''). The Go code already guards closed_*
-- behind status, and all read paths COALESCE these to defaults.
ALTER TABLE issue_lifecycle_projections
    ALTER COLUMN closed_at_unix_nano DROP DEFAULT,
    ALTER COLUMN closed_at_unix_nano DROP NOT NULL,
    ALTER COLUMN closed_by DROP DEFAULT,
    ALTER COLUMN closed_by DROP NOT NULL,
    ALTER COLUMN closed_reason DROP DEFAULT,
    ALTER COLUMN closed_reason DROP NOT NULL,
    ALTER COLUMN closed_reason_note DROP DEFAULT,
    ALTER COLUMN closed_reason_note DROP NOT NULL;

UPDATE issue_lifecycle_projections
SET closed_at_unix_nano = NULL,
    closed_by = NULL,
    closed_reason = NULL,
    closed_reason_note = NULL
WHERE closed_at_unix_nano = 0;
