-- FTS5 is SQLite's local full-text index. The application refreshes each row
-- transactionally after issue/content mutations; this initial backfill covers
-- databases created before the search migration existed.
CREATE VIRTUAL TABLE issue_search USING fts5(
    issue_id UNINDEXED,
    title,
    body,
    tags,
    tokenize = 'unicode61'
);

INSERT INTO issue_search (issue_id, title, body, tags)
SELECT i.id,
       i.raw,
       COALESCE((
           SELECT group_concat(p.raw, ' ')
           FROM issue_posts p
           WHERE p.issue_id = i.id
       ), i.raw),
       i.tags_json
FROM issues i;
