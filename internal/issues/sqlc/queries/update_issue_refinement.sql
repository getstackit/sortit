-- name: UpdateIssueRefinement :exec
UPDATE issues
SET raw = $1,
    tags_json = $2,
    tag_scores_json = $3,
    embedding_vector = $4::vector
WHERE id = $5;
