-- name: InsertIssueOperationParticipant :exec
INSERT INTO issue_operation_participants (
    operation_id,
    issue_id,
    role,
    sequence
) VALUES ($1, $2, $3, $4);
