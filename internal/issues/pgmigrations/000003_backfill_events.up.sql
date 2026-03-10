-- Backfill events from issue_posts
INSERT INTO events (id, kind, issue_id, created_by, created_at_unix_nano, body, participants_json)
SELECT
    p.id,
    CASE
        WHEN p.sequence = 1 AND p.kind = '' THEN 'report'
        WHEN p.kind = '' THEN 'refinement'
        ELSE p.kind
    END,
    p.issue_id,
    p.created_by,
    p.created_at_unix_nano,
    p.raw,
    '[]'::jsonb
FROM issue_posts p
ON CONFLICT (id) DO NOTHING;

-- Backfill events from issue_operations with aggregated participants
INSERT INTO events (id, kind, issue_id, created_by, created_at_unix_nano, body, participants_json)
SELECT
    o.id,
    o.kind,
    COALESCE(
        (SELECT p.issue_id FROM issue_operation_participants p
         WHERE p.operation_id = o.id
         ORDER BY p.sequence ASC
         LIMIT 1),
        ''
    ),
    o.created_by,
    o.created_at_unix_nano,
    o.note,
    COALESCE(
        (SELECT jsonb_agg(
            jsonb_build_object('issueId', p.issue_id, 'role', p.role)
            ORDER BY p.sequence ASC
        )
        FROM issue_operation_participants p
        WHERE p.operation_id = o.id),
        '[]'::jsonb
    )
FROM issue_operations o
ON CONFLICT (id) DO NOTHING;
