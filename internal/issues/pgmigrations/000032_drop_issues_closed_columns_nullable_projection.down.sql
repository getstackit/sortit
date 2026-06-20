-- Restore sentinel-based closed_* on the lifecycle projection before re-applying
-- NOT NULL.
UPDATE issue_lifecycle_projections
SET closed_at_unix_nano = COALESCE(closed_at_unix_nano, 0),
    closed_by = COALESCE(closed_by, ''),
    closed_reason = COALESCE(closed_reason, ''),
    closed_reason_note = COALESCE(closed_reason_note, '');

ALTER TABLE issue_lifecycle_projections
    ALTER COLUMN closed_at_unix_nano SET DEFAULT 0,
    ALTER COLUMN closed_at_unix_nano SET NOT NULL,
    ALTER COLUMN closed_by SET DEFAULT '',
    ALTER COLUMN closed_by SET NOT NULL,
    ALTER COLUMN closed_reason SET DEFAULT '',
    ALTER COLUMN closed_reason SET NOT NULL,
    ALTER COLUMN closed_reason_note SET DEFAULT '',
    ALTER COLUMN closed_reason_note SET NOT NULL;

-- Re-add the legacy issues.closed_* columns and backfill from the canonical
-- lifecycle projection so the issues table is restored to its prior shape.
ALTER TABLE issues
    ADD COLUMN IF NOT EXISTS closed_at_unix_nano BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS closed_by TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS closed_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS closed_reason_note TEXT NOT NULL DEFAULT '';

UPDATE issues
SET closed_at_unix_nano = COALESCE(p.closed_at_unix_nano, 0),
    closed_by = COALESCE(p.closed_by, ''),
    closed_reason = COALESCE(p.closed_reason, ''),
    closed_reason_note = COALESCE(p.closed_reason_note, '')
FROM issue_lifecycle_projections p
WHERE p.issue_id = issues.id;
