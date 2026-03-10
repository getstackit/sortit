-- name: ListIssueLinksForIssue :many
SELECT id, source_issue_id, target_issue_id, type, created_by, created_at_unix_nano, note, operation_id
FROM issue_links
WHERE source_issue_id = $1 OR target_issue_id = $2
ORDER BY created_at_unix_nano DESC, id DESC;
