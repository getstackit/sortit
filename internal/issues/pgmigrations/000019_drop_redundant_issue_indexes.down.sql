CREATE INDEX IF NOT EXISTS issue_posts_issue_id_sequence_idx
ON issue_posts(issue_id, sequence);

CREATE INDEX IF NOT EXISTS issue_snapshots_issue_id_sequence_idx
ON issue_snapshots(issue_id, sequence);
