DROP INDEX IF EXISTS issue_content_projections_search_bm25_idx;

ALTER TABLE issue_content_projections
    DROP COLUMN IF EXISTS search_tags,
    DROP COLUMN IF EXISTS search_body,
    DROP COLUMN IF EXISTS search_title;

DROP EXTENSION IF EXISTS pg_search;
