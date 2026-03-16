-- name: ListIssueReferencesByIDs :many
SELECT id, raw, status
FROM issues
WHERE id = ANY($1::text[])
ORDER BY id ASC;
