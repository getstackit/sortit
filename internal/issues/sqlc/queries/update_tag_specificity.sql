-- name: UpdateTagSpecificity :exec
UPDATE tags SET
    specificity = $2,
    specificity_llm = $3,
    specificity_embedding = $4,
    specificity_computed_at = $5
WHERE name = $1;
