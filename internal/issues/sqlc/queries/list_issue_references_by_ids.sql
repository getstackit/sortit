-- name: ListIssueReferencesByIDs :many
SELECT i.id, i.raw, COALESCE(p.status, i.status) AS status
FROM issues i
LEFT JOIN issue_lifecycle_projections p ON p.issue_id = i.id
WHERE i.id = ANY($1::text[])
ORDER BY i.id ASC;
