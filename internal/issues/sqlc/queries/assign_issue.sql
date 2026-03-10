-- name: AssignIssue :exec
UPDATE issues SET assigned_to = $1 WHERE id = $2;
