-- name: ListIssueOperationParticipants :many
SELECT operation_id, issue_id, role, sequence
FROM issue_operation_participants
WHERE operation_id = $1
ORDER BY sequence ASC, issue_id ASC;
