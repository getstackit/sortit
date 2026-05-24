DROP INDEX IF EXISTS issue_links_unique_logical_idx;

ALTER TABLE issue_links
    DROP CONSTRAINT IF EXISTS issue_links_no_self_links;
