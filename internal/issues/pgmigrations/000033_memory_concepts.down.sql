DROP INDEX IF EXISTS memories_concept_subject_tag_unique_idx;
ALTER TABLE memory_proposals DROP COLUMN IF EXISTS subject_tag;
ALTER TABLE memories DROP COLUMN IF EXISTS subject_tag;
