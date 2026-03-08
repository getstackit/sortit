CREATE TABLE issues_v2 (
    id TEXT PRIMARY KEY,
    raw TEXT NOT NULL,
    tags_json TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at_unix_nano INTEGER NOT NULL,
    tag_scores_json TEXT NOT NULL,
    embedding_json TEXT NOT NULL
);

INSERT INTO issues_v2 (id, raw, tags_json, created_by, created_at_unix_nano, tag_scores_json, embedding_json)
SELECT
    id,
    raw,
    tags_json,
    created_by,
    (CAST(strftime('%s', created_at) AS INTEGER) * 1000000000) +
        CASE
            WHEN instr(created_at, '.') = 0 THEN 0
            ELSE CAST(substr((substr(created_at, instr(created_at, '.') + 1, instr(created_at, 'Z') - instr(created_at, '.') - 1) || '000000000'), 1, 9) AS INTEGER)
        END,
    tag_scores_json,
    embedding_json
FROM issues;

DROP TABLE issues;
ALTER TABLE issues_v2 RENAME TO issues;
