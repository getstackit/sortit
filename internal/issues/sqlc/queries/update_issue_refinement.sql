-- name: UpdateIssueRefinement :exec
UPDATE issues
SET raw = $1,
    tags_json = $2,
    tag_scores_json = $3,
    embedding_json = $4
WHERE id = $5;
