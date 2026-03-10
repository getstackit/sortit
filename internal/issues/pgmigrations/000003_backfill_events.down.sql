-- Remove backfilled events (only those derived from posts and operations)
DELETE FROM events WHERE id IN (SELECT id FROM issue_posts);
DELETE FROM events WHERE id IN (SELECT id FROM issue_operations);
