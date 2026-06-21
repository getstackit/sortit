-- A concept memory (kind='concept') is the rich, recallable profile of a single
-- tag — the noun the rest of the corpus orbits. subject_tag binds it 1:1 to that
-- tag: the tag stays the lightweight label/node, the concept memory is its
-- definition. The partial unique index enforces at most one active concept per
-- tag and doubles as the synthesis idempotency guard (concurrent synth runs
-- cannot mint duplicate concepts for the same subject).
ALTER TABLE memories ADD COLUMN subject_tag TEXT NOT NULL DEFAULT '';
ALTER TABLE memory_proposals ADD COLUMN subject_tag TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX memories_concept_subject_tag_unique_idx
    ON memories (subject_tag)
    WHERE kind = 'concept' AND status = 'active';
