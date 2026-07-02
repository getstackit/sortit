---
name: sortit-wrap-up
description: "End a coding, review, planning, or debugging turn by reinforcing the Sortit workflow — record progress, create follow-on issues, preserve durable memories, and surface curation or proposal work for humans. Trigger phrases include \"wrap up\", \"wrap up this turn\", \"finish up and update Sortit\", \"run the end-of-turn checklist\", \"I'm done, log progress\", \"before we finish, capture follow-ups\", and \"close out and record what we learned\"."
---

# Sortit Wrap-Up

Use this before the final response for non-trivial work. The goal is to keep
Sortit current without turning the user's chat into the only record.

## Checklist

1. Issue(s) worked on this turn? For each one, don't stop at a progress log — drive it to its right next state:
   - **Progress** — record what changed, was verified, or was deferred: `command sortit issues progress <id> --raw "..."`.
   - **Refine** — if the work changed *what the issue is* (scope, root cause, understanding), update the canonical description with `$sortit-refine`.
   - **Close** — if the work resolved or obsoleted it, close it with `$sortit-close`. Don't leave finished work open.
   - **Iterate** — if distinct work remains, split it out or file follow-ons (step 2) and link them back.

2. Follow-on work discovered?
   - Search first: `command sortit issues search "<follow-on work>" --status all`.
   - If no strong match exists, create it with `command sortit issues create "<raw follow-on>"`.
   - If a match exists, refine or progress that issue instead.

3. Durable knowledge learned?
   - Recall first with `$sortit-recall` (or `command sortit memory search "<topic>"`) so you reinforce or supersede an existing memory instead of creating a near-duplicate.
   - Then use `$sortit-memory`.
   - Create a memory for clear decisions, lessons, constraints, patterns, or references.
   - Prefer a proposal/synthesis path when the knowledge is inferred or needs human review.

4. Related work found?
   - Use `command sortit issues explore <id>` (or `$sortit-explore`).
   - Link, split, combine, or draft curation proposals when the relationship is clear.

5. Human review needed?
   - Mention pending curation or memory proposals in the final response.

## Rules

1. Do not create noise. If nothing durable changed, say nothing about Sortit.
2. Search before creating follow-on issues.
3. Match the action to the change: progress for work done, refine for changes to what the issue is, close for work that's finished or obsolete.
4. Create memories only when future humans or agents should retrieve the fact.
5. Recall before you decide. If this turn made a decision without recalling
   related memories, recall now — and surface any prior decision it contradicts.
6. Keep final responses concise: mention the Sortit issue IDs or memory IDs that
   matter, not every command run.

<!-- sortit-version: dev -->
