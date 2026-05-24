DELETE FROM issue_links
WHERE source_issue_id = target_issue_id;

WITH ranked_links AS (
    SELECT ctid,
           ROW_NUMBER() OVER (
               PARTITION BY source_issue_id, target_issue_id, type
               ORDER BY created_at_unix_nano, id
           ) AS row_num
    FROM issue_links
)
DELETE FROM issue_links
WHERE ctid IN (
    SELECT ctid
    FROM ranked_links
    WHERE row_num > 1
);

DROP INDEX IF EXISTS issue_links_unique_logical_idx;

ALTER TABLE issue_links
    DROP CONSTRAINT IF EXISTS issue_links_no_self_links;

ALTER TABLE issue_links
    ADD CONSTRAINT issue_links_no_self_links
    CHECK (source_issue_id <> target_issue_id);

CREATE UNIQUE INDEX issue_links_unique_logical_idx
    ON issue_links (source_issue_id, target_issue_id, type);
