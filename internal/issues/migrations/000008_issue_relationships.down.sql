DROP INDEX IF EXISTS issue_links_target_issue_id_idx;
DROP INDEX IF EXISTS issue_links_source_issue_id_idx;
DROP TABLE IF EXISTS issue_links;

DROP INDEX IF EXISTS issue_operation_participants_issue_id_idx;
DROP TABLE IF EXISTS issue_operation_participants;
DROP TABLE IF EXISTS issue_operations;

DELETE FROM metadata
WHERE key = 'next_issue_operation_seq';
