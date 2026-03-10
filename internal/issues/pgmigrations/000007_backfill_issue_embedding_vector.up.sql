UPDATE issues
SET embedding_vector = (
    '[' || (
        SELECT string_agg(element.value, ',' ORDER BY element.ord)
        FROM jsonb_array_elements_text(issues.embedding_json) WITH ORDINALITY AS element(value, ord)
    ) || ']'
)::vector
WHERE embedding_vector IS NULL
  AND jsonb_typeof(embedding_json) = 'array'
  AND jsonb_array_length(embedding_json) > 0;
