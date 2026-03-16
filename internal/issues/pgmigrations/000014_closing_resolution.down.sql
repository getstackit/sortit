ALTER TABLE issues
    DROP COLUMN IF EXISTS closed_reason,
    DROP COLUMN IF EXISTS closed_reason_note;
