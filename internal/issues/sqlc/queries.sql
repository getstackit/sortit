-- name: ListIssues :many
SELECT id, raw, tags_json, created_by, created_at_unix_nano, status, closed_at_unix_nano, closed_by, tag_scores_json, embedding_json, assigned_to
FROM issues
ORDER BY created_at_unix_nano DESC, id ASC;

-- name: GetIssue :one
SELECT id, raw, tags_json, created_by, created_at_unix_nano, status, closed_at_unix_nano, closed_by, tag_scores_json, embedding_json, assigned_to
FROM issues
WHERE id = $1;

-- name: ListIssuePosts :many
SELECT id, issue_id, raw, created_by, created_at_unix_nano, sequence, kind
FROM issue_posts
WHERE issue_id = $1
ORDER BY sequence ASC, created_at_unix_nano ASC, id ASC;

-- name: InsertIssue :exec
INSERT INTO issues (
    id,
    raw,
    tags_json,
    created_by,
    created_at_unix_nano,
    status,
    closed_at_unix_nano,
    closed_by,
    tag_scores_json,
    embedding_json,
    assigned_to
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: AssignIssue :exec
UPDATE issues SET assigned_to = $1 WHERE id = $2;

-- name: InsertIssuePost :exec
INSERT INTO issue_posts (
    id,
    issue_id,
    raw,
    created_by,
    created_at_unix_nano,
    sequence,
    kind
) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: UpdateIssueRefinement :exec
UPDATE issues
SET raw = $1,
    tags_json = $2,
    tag_scores_json = $3,
    embedding_json = $4
WHERE id = $5;

-- name: CloseIssue :exec
UPDATE issues
SET status = 'closed',
    closed_at_unix_nano = $1,
    closed_by = $2
WHERE id = $3;

-- name: ReopenIssue :exec
UPDATE issues
SET status = 'open',
    closed_at_unix_nano = 0,
    closed_by = ''
WHERE id = $1;

-- name: ListIssueLinksForIssue :many
SELECT id, source_issue_id, target_issue_id, type, created_by, created_at_unix_nano, note, operation_id
FROM issue_links
WHERE source_issue_id = $1 OR target_issue_id = $2
ORDER BY created_at_unix_nano DESC, id DESC;

-- name: InsertIssueLink :exec
INSERT INTO issue_links (
    id,
    source_issue_id,
    target_issue_id,
    type,
    created_by,
    created_at_unix_nano,
    note,
    operation_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: DeleteAllIssueLinks :exec
DELETE FROM issue_links;

-- name: ListIssueOperationsForIssue :many
SELECT DISTINCT o.id, o.kind, o.created_by, o.created_at_unix_nano, o.note
FROM issue_operations o
JOIN issue_operation_participants p ON p.operation_id = o.id
WHERE p.issue_id = $1
ORDER BY o.created_at_unix_nano DESC, o.id DESC;

-- name: InsertIssueOperation :exec
INSERT INTO issue_operations (
    id,
    kind,
    created_by,
    created_at_unix_nano,
    note
) VALUES ($1, $2, $3, $4, $5);

-- name: DeleteAllIssueOperations :exec
DELETE FROM issue_operations;

-- name: ListIssueOperationParticipants :many
SELECT operation_id, issue_id, role, sequence
FROM issue_operation_participants
WHERE operation_id = $1
ORDER BY sequence ASC, issue_id ASC;

-- name: InsertIssueOperationParticipant :exec
INSERT INTO issue_operation_participants (
    operation_id,
    issue_id,
    role,
    sequence
) VALUES ($1, $2, $3, $4);

-- name: DeleteAllIssueOperationParticipants :exec
DELETE FROM issue_operation_participants;

-- name: DeleteAllIssues :exec
DELETE FROM issues;

-- name: DeleteAllIssuePosts :exec
DELETE FROM issue_posts;

-- name: ListTags :many
SELECT name, description, created_at_unix_nano, embedding_json
FROM tags
ORDER BY name ASC;

-- name: UpsertTag :exec
INSERT INTO tags (name, description, created_at_unix_nano, embedding_json)
VALUES ($1, $2, $3, $4)
ON CONFLICT(name) DO UPDATE SET
    description = CASE
        WHEN excluded.description <> '' THEN excluded.description
        ELSE tags.description
    END,
    embedding_json = CASE
        WHEN excluded.embedding_json <> '[]' THEN excluded.embedding_json
        ELSE tags.embedding_json
    END;

-- name: InsertEvent :exec
INSERT INTO events (
    id,
    kind,
    issue_id,
    created_by,
    created_at_unix_nano,
    body,
    participants_json
) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListEvents :many
SELECT id, kind, issue_id, created_by, created_at_unix_nano, body, participants_json
FROM events
ORDER BY created_at_unix_nano DESC, id DESC
LIMIT $1;

-- name: ListEventsBefore :many
SELECT id, kind, issue_id, created_by, created_at_unix_nano, body, participants_json
FROM events
WHERE created_at_unix_nano < $1
   OR (created_at_unix_nano = $1 AND id < $2)
ORDER BY created_at_unix_nano DESC, id DESC
LIMIT $3;

-- name: ListEventsByKind :many
SELECT id, kind, issue_id, created_by, created_at_unix_nano, body, participants_json
FROM events
WHERE kind = $1
ORDER BY created_at_unix_nano DESC, id DESC
LIMIT $2;

-- name: ListEventsByKindBefore :many
SELECT id, kind, issue_id, created_by, created_at_unix_nano, body, participants_json
FROM events
WHERE kind = $1
  AND (created_at_unix_nano < $2
       OR (created_at_unix_nano = $2 AND id < $3))
ORDER BY created_at_unix_nano DESC, id DESC
LIMIT $4;

-- name: DeleteAllEvents :exec
DELETE FROM events;
