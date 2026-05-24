-- name: ListIssueOperationsForIssue :many
SELECT DISTINCT o.id, o.kind, o.created_by, o.created_at_unix_nano, o.note
FROM issue_operations o
JOIN issue_operation_participants p ON p.operation_id = o.id
WHERE p.issue_id = $1
ORDER BY o.created_at_unix_nano DESC, o.id DESC;
