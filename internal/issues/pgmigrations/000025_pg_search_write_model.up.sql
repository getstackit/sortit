CREATE EXTENSION IF NOT EXISTS pg_search;

ALTER TABLE issue_content_projections
    ADD COLUMN IF NOT EXISTS search_title TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS search_body TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS search_tags TEXT NOT NULL DEFAULT '';

UPDATE issue_content_projections AS c
SET search_title = BTRIM(c.raw),
    search_body = COALESCE(
        NULLIF((
            SELECT string_agg(BTRIM(p.raw), E'\n\n' ORDER BY p.sequence, p.id)
            FROM issue_posts AS p
            WHERE p.issue_id = c.issue_id
              AND BTRIM(p.raw) <> ''
        ), ''),
        BTRIM(c.raw)
    ),
    search_tags = COALESCE((
        SELECT string_agg(term, ' ' ORDER BY term)
        FROM (
            SELECT DISTINCT LOWER(BTRIM(tag)) AS term
            FROM jsonb_array_elements_text(c.tags_json) AS tags(tag)
            WHERE BTRIM(tag) <> ''

            UNION

            SELECT DISTINCT LOWER(BTRIM(score.tag)) AS term
            FROM jsonb_to_recordset(c.tag_scores_json) AS score(tag text, relevance double precision)
            WHERE BTRIM(score.tag) <> ''
        ) AS terms
    ), '');

CREATE INDEX IF NOT EXISTS issue_content_projections_search_bm25_idx
ON issue_content_projections
USING bm25 (issue_id, search_title, search_body, search_tags, updated_at_unix_nano)
WITH (key_field = 'issue_id');
