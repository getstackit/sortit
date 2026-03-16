-- name: ListIssueReferencesByIDs :many
SELECT i.id, COALESCE(c.raw, i.raw) AS raw, COALESCE(p.status, i.status) AS status
FROM issues i
LEFT JOIN issue_content_projections c ON c.issue_id = i.id
LEFT JOIN issue_lifecycle_projections p ON p.issue_id = i.id
WHERE i.id = ANY($1::text[])
ORDER BY i.id ASC;
