-- name: ListIssueOperationParticipantsForOperations :many
SELECT operation_id, issue_id, role, sequence
FROM issue_operation_participants
WHERE operation_id = ANY($1::text[])
ORDER BY operation_id ASC, sequence ASC, issue_id ASC;
