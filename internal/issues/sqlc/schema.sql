CREATE EXTENSION IF NOT EXISTS "pg_search" WITH SCHEMA "paradedb";
CREATE EXTENSION IF NOT EXISTS "vector" WITH SCHEMA "public";

CREATE SEQUENCE "public"."dismissed_tag_merges_id_seq";
CREATE SEQUENCE "public"."tag_merge_history_id_seq";

CREATE TABLE "public"."api_token_facts" (
    "id" text NOT NULL,
    "token_id" text NOT NULL,
    "user_id" text NOT NULL,
    "sequence" bigint NOT NULL,
    "kind" text NOT NULL,
    "created_by" text NOT NULL,
    "created_at_unix_nano" bigint NOT NULL,
    "payload_json" jsonb NOT NULL,
    "source" text NOT NULL,
    "source_id" text NOT NULL,
    "inferred" boolean NOT NULL
);
CREATE TABLE "public"."api_tokens" (
    "id" text NOT NULL,
    "user_id" text NOT NULL,
    "token_hash" text NOT NULL,
    "token_prefix" text NOT NULL,
    "created_at_unix_nano" bigint NOT NULL,
    "revoked_at_unix_nano" bigint NOT NULL,
    "name" text NOT NULL,
    "last_used_at_unix_nano" bigint NOT NULL
);
CREATE TABLE "public"."append_only_migration_checkpoints" (
    "name" text NOT NULL,
    "phase" text NOT NULL,
    "cursor_json" jsonb NOT NULL,
    "summary_json" jsonb NOT NULL,
    "updated_at_unix_nano" bigint NOT NULL
);
CREATE TABLE "public"."append_only_parity_runs" (
    "id" text NOT NULL,
    "domain" text NOT NULL,
    "status" text NOT NULL,
    "details_json" jsonb NOT NULL,
    "created_at_unix_nano" bigint NOT NULL
);
CREATE TABLE "public"."auth_accounts" (
    "id" text NOT NULL,
    "user_id" text NOT NULL,
    "provider" text NOT NULL,
    "provider_user_id" text NOT NULL,
    "created_at_unix_nano" bigint NOT NULL
);
CREATE TABLE "public"."curation_proposals" (
    "id" text NOT NULL,
    "kind" text NOT NULL,
    "payload_json" jsonb NOT NULL,
    "status" text NOT NULL,
    "rationale" text NOT NULL,
    "confidence" double precision NOT NULL,
    "source_refs_json" jsonb NOT NULL,
    "created_by" text NOT NULL,
    "reviewed_by" text NOT NULL,
    "result_ref" text NOT NULL,
    "created_at_unix_nano" bigint NOT NULL,
    "updated_at_unix_nano" bigint NOT NULL
);
CREATE TABLE "public"."custom_regions" (
    "id" text NOT NULL,
    "label" text NOT NULL,
    "description" text NOT NULL,
    "definition_json" jsonb NOT NULL,
    "created_by" text NOT NULL,
    "created_at_unix_nano" bigint NOT NULL,
    "updated_at_unix_nano" bigint NOT NULL
);
CREATE TABLE "public"."dismissed_tag_merges" (
    "id" bigint NOT NULL,
    "canonical_name" text NOT NULL,
    "alias_name" text NOT NULL,
    "dismissed_at" timestamp with time zone NOT NULL
);
CREATE TABLE "public"."events" (
    "id" text NOT NULL,
    "kind" text NOT NULL,
    "issue_id" text,
    "created_by" text NOT NULL,
    "created_at_unix_nano" bigint NOT NULL,
    "body" text NOT NULL,
    "participants_json" jsonb NOT NULL
);
CREATE TABLE "public"."issue_content_facts" (
    "id" text NOT NULL,
    "issue_id" text NOT NULL,
    "sequence" bigint NOT NULL,
    "kind" text NOT NULL,
    "created_by" text NOT NULL,
    "created_at_unix_nano" bigint NOT NULL,
    "payload_json" jsonb NOT NULL,
    "source" text NOT NULL,
    "source_id" text NOT NULL,
    "inferred" boolean NOT NULL
);
CREATE TABLE "public"."issue_content_projections" (
    "issue_id" text NOT NULL,
    "raw" text NOT NULL,
    "tags_json" jsonb NOT NULL,
    "tag_scores_json" jsonb NOT NULL,
    "embedding_vector" vector,
    "last_event_id" text NOT NULL,
    "event_count" bigint NOT NULL,
    "updated_at_unix_nano" bigint NOT NULL,
    "search_title" text NOT NULL,
    "search_body" text NOT NULL,
    "search_tags" text NOT NULL
);
CREATE TABLE "public"."issue_enrichment_events" (
    "id" text NOT NULL,
    "issue_id" text NOT NULL,
    "target_sequence" bigint NOT NULL,
    "kind" text NOT NULL,
    "created_by" text NOT NULL,
    "created_at_unix_nano" bigint NOT NULL,
    "payload_json" jsonb NOT NULL
);
CREATE TABLE "public"."issue_enrichment_jobs" (
    "issue_id" text NOT NULL,
    "target_sequence" bigint NOT NULL,
    "lease_expires_at_unix_nano" bigint NOT NULL,
    "attempt_count" bigint NOT NULL,
    "created_at_unix_nano" bigint NOT NULL,
    "updated_at_unix_nano" bigint NOT NULL
);
CREATE TABLE "public"."issue_enrichment_projections" (
    "issue_id" text NOT NULL,
    "status" text NOT NULL,
    "error" text NOT NULL,
    "target_sequence" bigint NOT NULL,
    "attempt_count" bigint NOT NULL,
    "lease_expires_at_unix_nano" bigint NOT NULL,
    "latest_event_id" text NOT NULL,
    "updated_at_unix_nano" bigint NOT NULL
);
CREATE TABLE "public"."issue_lifecycle_facts" (
    "id" text NOT NULL,
    "issue_id" text NOT NULL,
    "sequence" bigint NOT NULL,
    "kind" text NOT NULL,
    "created_by" text NOT NULL,
    "created_at_unix_nano" bigint NOT NULL,
    "payload_json" jsonb NOT NULL,
    "source" text NOT NULL,
    "source_id" text NOT NULL,
    "inferred" boolean NOT NULL
);
CREATE TABLE "public"."issue_lifecycle_projections" (
    "issue_id" text NOT NULL,
    "created_by" text NOT NULL,
    "created_at_unix_nano" bigint NOT NULL,
    "status" text NOT NULL,
    "closed_at_unix_nano" bigint,
    "closed_by" text,
    "closed_reason" text,
    "closed_reason_note" text,
    "assigned_to" text NOT NULL,
    "last_fact_id" text NOT NULL,
    "fact_count" bigint NOT NULL,
    "updated_at_unix_nano" bigint NOT NULL
);
CREATE TABLE "public"."issue_links" (
    "id" text NOT NULL,
    "source_issue_id" text NOT NULL,
    "target_issue_id" text NOT NULL,
    "type" text NOT NULL,
    "created_by" text NOT NULL,
    "created_at_unix_nano" bigint NOT NULL,
    "note" text NOT NULL,
    "operation_id" text NOT NULL
);
CREATE TABLE "public"."issue_operation_participants" (
    "operation_id" text NOT NULL,
    "issue_id" text NOT NULL,
    "role" text NOT NULL,
    "sequence" bigint NOT NULL
);
CREATE TABLE "public"."issue_operations" (
    "id" text NOT NULL,
    "kind" text NOT NULL,
    "created_by" text NOT NULL,
    "created_at_unix_nano" bigint NOT NULL,
    "note" text NOT NULL
);
CREATE TABLE "public"."issue_posts" (
    "id" text NOT NULL,
    "issue_id" text NOT NULL,
    "raw" text NOT NULL,
    "created_by" text NOT NULL,
    "created_at_unix_nano" bigint NOT NULL,
    "sequence" bigint NOT NULL,
    "kind" text NOT NULL
);
CREATE TABLE "public"."issue_snapshots" (
    "issue_id" text NOT NULL,
    "sequence" bigint NOT NULL,
    "raw" text NOT NULL,
    "tags_json" jsonb NOT NULL,
    "tag_scores_json" jsonb NOT NULL,
    "embedding_json" jsonb NOT NULL,
    "created_at_unix_nano" bigint NOT NULL
);
CREATE TABLE "public"."issues" (
    "id" text NOT NULL,
    "raw" text NOT NULL,
    "tags_json" jsonb NOT NULL,
    "created_by" text NOT NULL,
    "created_at_unix_nano" bigint NOT NULL,
    "status" text NOT NULL,
    "tag_scores_json" jsonb NOT NULL,
    "assigned_to" text NOT NULL,
    "embedding_vector" vector
);
CREATE TABLE "public"."map_projections" (
    "revision" bigint NOT NULL,
    "payload_json" jsonb NOT NULL,
    "created_at_unix_nano" bigint NOT NULL
);
CREATE TABLE "public"."memories" (
    "id" text NOT NULL,
    "title" text NOT NULL,
    "body" text NOT NULL,
    "kind" text NOT NULL,
    "anchor_tags_json" jsonb NOT NULL,
    "anchor_region" text NOT NULL,
    "tag_scores_json" jsonb NOT NULL,
    "embedding_vector" vector,
    "status" text NOT NULL,
    "superseded_by" text NOT NULL,
    "source" text NOT NULL,
    "source_issue_ids_json" jsonb NOT NULL,
    "confidence" double precision NOT NULL,
    "created_by" text NOT NULL,
    "created_at_unix_nano" bigint NOT NULL,
    "updated_at_unix_nano" bigint NOT NULL,
    "last_reinforced_at_unix_nano" bigint NOT NULL,
    "reinforcement_count" bigint NOT NULL,
    "subject_tag" text NOT NULL
);
CREATE TABLE "public"."memory_proposals" (
    "id" text NOT NULL,
    "title" text NOT NULL,
    "body" text NOT NULL,
    "kind" text NOT NULL,
    "anchor_tags_json" jsonb NOT NULL,
    "anchor_region" text NOT NULL,
    "source_issue_ids_json" jsonb NOT NULL,
    "confidence" double precision NOT NULL,
    "status" text NOT NULL,
    "rationale" text NOT NULL,
    "accepted_memory_id" text NOT NULL,
    "created_at_unix_nano" bigint NOT NULL,
    "updated_at_unix_nano" bigint NOT NULL,
    "subject_tag" text NOT NULL
);
CREATE TABLE "public"."sessions" (
    "id" text NOT NULL,
    "user_id" text NOT NULL,
    "token_hash" text NOT NULL,
    "expires_at_unix_nano" bigint NOT NULL,
    "created_at_unix_nano" bigint NOT NULL
);
CREATE TABLE "public"."tag_cooccurrence_projections" (
    "id" text NOT NULL,
    "revision" bigint NOT NULL,
    "issue_count" bigint NOT NULL,
    "tag_count" integer NOT NULL,
    "body_json" jsonb NOT NULL,
    "computed_at_unix_nano" bigint NOT NULL
);
CREATE TABLE "public"."tag_events" (
    "id" text NOT NULL,
    "tag_name" text NOT NULL,
    "sequence" bigint NOT NULL,
    "kind" text NOT NULL,
    "created_by" text NOT NULL,
    "created_at_unix_nano" bigint NOT NULL,
    "payload_json" jsonb NOT NULL,
    "source" text NOT NULL,
    "source_id" text NOT NULL,
    "inferred" boolean NOT NULL
);
CREATE TABLE "public"."tag_merge_history" (
    "id" bigint NOT NULL,
    "canonical_name" text NOT NULL,
    "alias_name" text NOT NULL,
    "merged_at" timestamp with time zone NOT NULL,
    "merged_by" text NOT NULL
);
CREATE TABLE "public"."tag_projections" (
    "name" text NOT NULL,
    "description" text NOT NULL,
    "created_at_unix_nano" bigint NOT NULL,
    "embedding_vector" vector,
    "specificity" real,
    "specificity_llm" real,
    "specificity_embedding" real,
    "specificity_computed_at" timestamp with time zone,
    "status" text NOT NULL,
    "canonical_name" text NOT NULL,
    "last_event_id" text NOT NULL,
    "event_count" bigint NOT NULL,
    "updated_at_unix_nano" bigint NOT NULL
);
CREATE TABLE "public"."tags" (
    "name" text NOT NULL,
    "description" text NOT NULL,
    "created_at_unix_nano" bigint NOT NULL,
    "specificity" real,
    "specificity_llm" real,
    "specificity_embedding" real,
    "specificity_computed_at" timestamp with time zone,
    "embedding_vector" vector
);
CREATE TABLE "public"."user_profile_facts" (
    "id" text NOT NULL,
    "user_id" text NOT NULL,
    "sequence" bigint NOT NULL,
    "login" text NOT NULL,
    "display_name" text NOT NULL,
    "avatar_url" text NOT NULL,
    "email" text NOT NULL,
    "observed_at_unix_nano" bigint NOT NULL,
    "source" text NOT NULL,
    "source_id" text NOT NULL,
    "inferred" boolean NOT NULL
);
CREATE TABLE "public"."users" (
    "id" text NOT NULL,
    "login" text NOT NULL,
    "display_name" text NOT NULL,
    "avatar_url" text NOT NULL,
    "email" text NOT NULL,
    "created_at_unix_nano" bigint NOT NULL,
    "updated_at_unix_nano" bigint NOT NULL
);

ALTER SEQUENCE "public"."dismissed_tag_merges_id_seq" OWNED BY "public"."dismissed_tag_merges"."id";
ALTER SEQUENCE "public"."tag_merge_history_id_seq" OWNED BY "public"."tag_merge_history"."id";

ALTER TABLE ONLY "public"."api_token_facts" ALTER COLUMN "created_by" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."api_token_facts" ALTER COLUMN "payload_json" SET DEFAULT '{}'::jsonb;
ALTER TABLE ONLY "public"."api_token_facts" ALTER COLUMN "source" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."api_token_facts" ALTER COLUMN "source_id" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."api_token_facts" ALTER COLUMN "inferred" SET DEFAULT false;
ALTER TABLE ONLY "public"."api_tokens" ALTER COLUMN "revoked_at_unix_nano" SET DEFAULT 0;
ALTER TABLE ONLY "public"."api_tokens" ALTER COLUMN "name" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."api_tokens" ALTER COLUMN "last_used_at_unix_nano" SET DEFAULT 0;
ALTER TABLE ONLY "public"."append_only_migration_checkpoints" ALTER COLUMN "phase" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."append_only_migration_checkpoints" ALTER COLUMN "cursor_json" SET DEFAULT '{}'::jsonb;
ALTER TABLE ONLY "public"."append_only_migration_checkpoints" ALTER COLUMN "summary_json" SET DEFAULT '{}'::jsonb;
ALTER TABLE ONLY "public"."append_only_parity_runs" ALTER COLUMN "details_json" SET DEFAULT '{}'::jsonb;
ALTER TABLE ONLY "public"."curation_proposals" ALTER COLUMN "kind" SET DEFAULT 'combine_issues'::text;
ALTER TABLE ONLY "public"."curation_proposals" ALTER COLUMN "payload_json" SET DEFAULT '{}'::jsonb;
ALTER TABLE ONLY "public"."curation_proposals" ALTER COLUMN "status" SET DEFAULT 'pending'::text;
ALTER TABLE ONLY "public"."curation_proposals" ALTER COLUMN "rationale" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."curation_proposals" ALTER COLUMN "confidence" SET DEFAULT 0;
ALTER TABLE ONLY "public"."curation_proposals" ALTER COLUMN "source_refs_json" SET DEFAULT '[]'::jsonb;
ALTER TABLE ONLY "public"."curation_proposals" ALTER COLUMN "created_by" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."curation_proposals" ALTER COLUMN "reviewed_by" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."curation_proposals" ALTER COLUMN "result_ref" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."custom_regions" ALTER COLUMN "description" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."custom_regions" ALTER COLUMN "created_by" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."dismissed_tag_merges" ALTER COLUMN "id" SET DEFAULT nextval('dismissed_tag_merges_id_seq'::regclass);
ALTER TABLE ONLY "public"."dismissed_tag_merges" ALTER COLUMN "dismissed_at" SET DEFAULT now();
ALTER TABLE ONLY "public"."events" ALTER COLUMN "body" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."events" ALTER COLUMN "participants_json" SET DEFAULT '[]'::jsonb;
ALTER TABLE ONLY "public"."issue_content_facts" ALTER COLUMN "created_by" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issue_content_facts" ALTER COLUMN "payload_json" SET DEFAULT '{}'::jsonb;
ALTER TABLE ONLY "public"."issue_content_facts" ALTER COLUMN "source" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issue_content_facts" ALTER COLUMN "source_id" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issue_content_facts" ALTER COLUMN "inferred" SET DEFAULT false;
ALTER TABLE ONLY "public"."issue_content_projections" ALTER COLUMN "raw" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issue_content_projections" ALTER COLUMN "tags_json" SET DEFAULT '[]'::jsonb;
ALTER TABLE ONLY "public"."issue_content_projections" ALTER COLUMN "tag_scores_json" SET DEFAULT '[]'::jsonb;
ALTER TABLE ONLY "public"."issue_content_projections" ALTER COLUMN "last_event_id" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issue_content_projections" ALTER COLUMN "event_count" SET DEFAULT 0;
ALTER TABLE ONLY "public"."issue_content_projections" ALTER COLUMN "search_title" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issue_content_projections" ALTER COLUMN "search_body" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issue_content_projections" ALTER COLUMN "search_tags" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issue_enrichment_events" ALTER COLUMN "created_by" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issue_enrichment_events" ALTER COLUMN "payload_json" SET DEFAULT '{}'::jsonb;
ALTER TABLE ONLY "public"."issue_enrichment_jobs" ALTER COLUMN "lease_expires_at_unix_nano" SET DEFAULT 0;
ALTER TABLE ONLY "public"."issue_enrichment_jobs" ALTER COLUMN "attempt_count" SET DEFAULT 0;
ALTER TABLE ONLY "public"."issue_enrichment_projections" ALTER COLUMN "status" SET DEFAULT 'complete'::text;
ALTER TABLE ONLY "public"."issue_enrichment_projections" ALTER COLUMN "error" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issue_enrichment_projections" ALTER COLUMN "target_sequence" SET DEFAULT 1;
ALTER TABLE ONLY "public"."issue_enrichment_projections" ALTER COLUMN "attempt_count" SET DEFAULT 0;
ALTER TABLE ONLY "public"."issue_enrichment_projections" ALTER COLUMN "lease_expires_at_unix_nano" SET DEFAULT 0;
ALTER TABLE ONLY "public"."issue_enrichment_projections" ALTER COLUMN "latest_event_id" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issue_lifecycle_facts" ALTER COLUMN "created_by" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issue_lifecycle_facts" ALTER COLUMN "payload_json" SET DEFAULT '{}'::jsonb;
ALTER TABLE ONLY "public"."issue_lifecycle_facts" ALTER COLUMN "source" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issue_lifecycle_facts" ALTER COLUMN "source_id" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issue_lifecycle_facts" ALTER COLUMN "inferred" SET DEFAULT false;
ALTER TABLE ONLY "public"."issue_lifecycle_projections" ALTER COLUMN "created_by" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issue_lifecycle_projections" ALTER COLUMN "status" SET DEFAULT 'open'::text;
ALTER TABLE ONLY "public"."issue_lifecycle_projections" ALTER COLUMN "assigned_to" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issue_lifecycle_projections" ALTER COLUMN "last_fact_id" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issue_lifecycle_projections" ALTER COLUMN "fact_count" SET DEFAULT 0;
ALTER TABLE ONLY "public"."issue_links" ALTER COLUMN "note" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issue_links" ALTER COLUMN "operation_id" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issue_operations" ALTER COLUMN "note" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issue_posts" ALTER COLUMN "kind" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issue_snapshots" ALTER COLUMN "tags_json" SET DEFAULT '[]'::jsonb;
ALTER TABLE ONLY "public"."issue_snapshots" ALTER COLUMN "tag_scores_json" SET DEFAULT '[]'::jsonb;
ALTER TABLE ONLY "public"."issue_snapshots" ALTER COLUMN "embedding_json" SET DEFAULT '[]'::jsonb;
ALTER TABLE ONLY "public"."issues" ALTER COLUMN "tags_json" SET DEFAULT '[]'::jsonb;
ALTER TABLE ONLY "public"."issues" ALTER COLUMN "tag_scores_json" SET DEFAULT '[]'::jsonb;
ALTER TABLE ONLY "public"."issues" ALTER COLUMN "assigned_to" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."memories" ALTER COLUMN "title" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."memories" ALTER COLUMN "body" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."memories" ALTER COLUMN "kind" SET DEFAULT 'decision'::text;
ALTER TABLE ONLY "public"."memories" ALTER COLUMN "anchor_tags_json" SET DEFAULT '[]'::jsonb;
ALTER TABLE ONLY "public"."memories" ALTER COLUMN "anchor_region" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."memories" ALTER COLUMN "tag_scores_json" SET DEFAULT '[]'::jsonb;
ALTER TABLE ONLY "public"."memories" ALTER COLUMN "status" SET DEFAULT 'active'::text;
ALTER TABLE ONLY "public"."memories" ALTER COLUMN "superseded_by" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."memories" ALTER COLUMN "source" SET DEFAULT 'manual'::text;
ALTER TABLE ONLY "public"."memories" ALTER COLUMN "source_issue_ids_json" SET DEFAULT '[]'::jsonb;
ALTER TABLE ONLY "public"."memories" ALTER COLUMN "confidence" SET DEFAULT 1;
ALTER TABLE ONLY "public"."memories" ALTER COLUMN "created_by" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."memories" ALTER COLUMN "last_reinforced_at_unix_nano" SET DEFAULT 0;
ALTER TABLE ONLY "public"."memories" ALTER COLUMN "reinforcement_count" SET DEFAULT 0;
ALTER TABLE ONLY "public"."memories" ALTER COLUMN "subject_tag" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."memory_proposals" ALTER COLUMN "title" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."memory_proposals" ALTER COLUMN "body" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."memory_proposals" ALTER COLUMN "kind" SET DEFAULT 'decision'::text;
ALTER TABLE ONLY "public"."memory_proposals" ALTER COLUMN "anchor_tags_json" SET DEFAULT '[]'::jsonb;
ALTER TABLE ONLY "public"."memory_proposals" ALTER COLUMN "anchor_region" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."memory_proposals" ALTER COLUMN "source_issue_ids_json" SET DEFAULT '[]'::jsonb;
ALTER TABLE ONLY "public"."memory_proposals" ALTER COLUMN "confidence" SET DEFAULT 0;
ALTER TABLE ONLY "public"."memory_proposals" ALTER COLUMN "status" SET DEFAULT 'pending'::text;
ALTER TABLE ONLY "public"."memory_proposals" ALTER COLUMN "rationale" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."memory_proposals" ALTER COLUMN "accepted_memory_id" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."memory_proposals" ALTER COLUMN "subject_tag" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."tag_cooccurrence_projections" ALTER COLUMN "revision" SET DEFAULT 0;
ALTER TABLE ONLY "public"."tag_cooccurrence_projections" ALTER COLUMN "issue_count" SET DEFAULT 0;
ALTER TABLE ONLY "public"."tag_cooccurrence_projections" ALTER COLUMN "tag_count" SET DEFAULT 0;
ALTER TABLE ONLY "public"."tag_cooccurrence_projections" ALTER COLUMN "body_json" SET DEFAULT '{}'::jsonb;
ALTER TABLE ONLY "public"."tag_cooccurrence_projections" ALTER COLUMN "computed_at_unix_nano" SET DEFAULT 0;
ALTER TABLE ONLY "public"."tag_events" ALTER COLUMN "created_by" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."tag_events" ALTER COLUMN "payload_json" SET DEFAULT '{}'::jsonb;
ALTER TABLE ONLY "public"."tag_events" ALTER COLUMN "source" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."tag_events" ALTER COLUMN "source_id" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."tag_events" ALTER COLUMN "inferred" SET DEFAULT false;
ALTER TABLE ONLY "public"."tag_merge_history" ALTER COLUMN "id" SET DEFAULT nextval('tag_merge_history_id_seq'::regclass);
ALTER TABLE ONLY "public"."tag_merge_history" ALTER COLUMN "merged_at" SET DEFAULT now();
ALTER TABLE ONLY "public"."tag_merge_history" ALTER COLUMN "merged_by" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."tag_projections" ALTER COLUMN "description" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."tag_projections" ALTER COLUMN "status" SET DEFAULT 'active'::text;
ALTER TABLE ONLY "public"."tag_projections" ALTER COLUMN "canonical_name" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."tag_projections" ALTER COLUMN "last_event_id" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."tag_projections" ALTER COLUMN "event_count" SET DEFAULT 0;
ALTER TABLE ONLY "public"."user_profile_facts" ALTER COLUMN "login" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."user_profile_facts" ALTER COLUMN "display_name" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."user_profile_facts" ALTER COLUMN "avatar_url" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."user_profile_facts" ALTER COLUMN "email" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."user_profile_facts" ALTER COLUMN "source" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."user_profile_facts" ALTER COLUMN "source_id" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."user_profile_facts" ALTER COLUMN "inferred" SET DEFAULT false;

ALTER TABLE ONLY "public"."api_token_facts" ADD CONSTRAINT "api_token_facts_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."api_tokens" ADD CONSTRAINT "api_tokens_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."api_tokens" ADD CONSTRAINT "api_tokens_token_hash_key" UNIQUE (token_hash);
ALTER TABLE ONLY "public"."api_tokens" ADD CONSTRAINT "api_tokens_user_id_fkey" FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE ONLY "public"."append_only_migration_checkpoints" ADD CONSTRAINT "append_only_migration_checkpoints_pkey" PRIMARY KEY (name);
ALTER TABLE ONLY "public"."append_only_parity_runs" ADD CONSTRAINT "append_only_parity_runs_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."auth_accounts" ADD CONSTRAINT "auth_accounts_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."auth_accounts" ADD CONSTRAINT "auth_accounts_provider_provider_user_id_key" UNIQUE (provider, provider_user_id);
ALTER TABLE ONLY "public"."auth_accounts" ADD CONSTRAINT "auth_accounts_user_id_fkey" FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE ONLY "public"."curation_proposals" ADD CONSTRAINT "curation_proposals_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."custom_regions" ADD CONSTRAINT "custom_regions_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."dismissed_tag_merges" ADD CONSTRAINT "dismissed_tag_merges_canonical_name_alias_name_key" UNIQUE (canonical_name, alias_name);
ALTER TABLE ONLY "public"."dismissed_tag_merges" ADD CONSTRAINT "dismissed_tag_merges_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."events" ADD CONSTRAINT "events_issue_id_fkey" FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE SET NULL;
ALTER TABLE ONLY "public"."events" ADD CONSTRAINT "events_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."issue_content_facts" ADD CONSTRAINT "issue_content_facts_issue_id_fkey" FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE CASCADE;
ALTER TABLE ONLY "public"."issue_content_facts" ADD CONSTRAINT "issue_content_facts_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."issue_content_projections" ADD CONSTRAINT "issue_content_projections_issue_id_fkey" FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE CASCADE;
ALTER TABLE ONLY "public"."issue_content_projections" ADD CONSTRAINT "issue_content_projections_pkey" PRIMARY KEY (issue_id);
ALTER TABLE ONLY "public"."issue_enrichment_events" ADD CONSTRAINT "issue_enrichment_events_issue_id_fkey" FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE CASCADE;
ALTER TABLE ONLY "public"."issue_enrichment_events" ADD CONSTRAINT "issue_enrichment_events_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."issue_enrichment_jobs" ADD CONSTRAINT "issue_enrichment_jobs_issue_id_fkey" FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE CASCADE;
ALTER TABLE ONLY "public"."issue_enrichment_jobs" ADD CONSTRAINT "issue_enrichment_jobs_pkey" PRIMARY KEY (issue_id);
ALTER TABLE ONLY "public"."issue_enrichment_projections" ADD CONSTRAINT "issue_enrichment_projections_issue_id_fkey" FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE CASCADE;
ALTER TABLE ONLY "public"."issue_enrichment_projections" ADD CONSTRAINT "issue_enrichment_projections_pkey" PRIMARY KEY (issue_id);
ALTER TABLE ONLY "public"."issue_lifecycle_facts" ADD CONSTRAINT "issue_lifecycle_facts_issue_id_fkey" FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE CASCADE;
ALTER TABLE ONLY "public"."issue_lifecycle_facts" ADD CONSTRAINT "issue_lifecycle_facts_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."issue_lifecycle_projections" ADD CONSTRAINT "issue_lifecycle_projections_issue_id_fkey" FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE CASCADE;
ALTER TABLE ONLY "public"."issue_lifecycle_projections" ADD CONSTRAINT "issue_lifecycle_projections_pkey" PRIMARY KEY (issue_id);
ALTER TABLE ONLY "public"."issue_links" ADD CONSTRAINT "issue_links_no_self_links" CHECK (source_issue_id <> target_issue_id);
ALTER TABLE ONLY "public"."issue_links" ADD CONSTRAINT "issue_links_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."issue_links" ADD CONSTRAINT "issue_links_source_issue_id_fkey" FOREIGN KEY (source_issue_id) REFERENCES issues(id) ON DELETE CASCADE;
ALTER TABLE ONLY "public"."issue_links" ADD CONSTRAINT "issue_links_target_issue_id_fkey" FOREIGN KEY (target_issue_id) REFERENCES issues(id) ON DELETE CASCADE;
ALTER TABLE ONLY "public"."issue_operation_participants" ADD CONSTRAINT "issue_operation_participants_issue_id_fkey" FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE CASCADE;
ALTER TABLE ONLY "public"."issue_operation_participants" ADD CONSTRAINT "issue_operation_participants_operation_id_fkey" FOREIGN KEY (operation_id) REFERENCES issue_operations(id) ON DELETE CASCADE;
ALTER TABLE ONLY "public"."issue_operation_participants" ADD CONSTRAINT "issue_operation_participants_pkey" PRIMARY KEY (operation_id, issue_id, role);
ALTER TABLE ONLY "public"."issue_operations" ADD CONSTRAINT "issue_operations_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."issue_posts" ADD CONSTRAINT "issue_posts_issue_id_fkey" FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE CASCADE;
ALTER TABLE ONLY "public"."issue_posts" ADD CONSTRAINT "issue_posts_issue_id_sequence_key" UNIQUE (issue_id, sequence);
ALTER TABLE ONLY "public"."issue_posts" ADD CONSTRAINT "issue_posts_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."issue_snapshots" ADD CONSTRAINT "issue_snapshots_issue_id_fkey" FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE CASCADE;
ALTER TABLE ONLY "public"."issue_snapshots" ADD CONSTRAINT "issue_snapshots_pkey" PRIMARY KEY (issue_id, sequence);
ALTER TABLE ONLY "public"."issues" ADD CONSTRAINT "issues_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."map_projections" ADD CONSTRAINT "derived_corpus_projections_pkey" PRIMARY KEY (revision);
ALTER TABLE ONLY "public"."memories" ADD CONSTRAINT "memories_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."memory_proposals" ADD CONSTRAINT "memory_proposals_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."sessions" ADD CONSTRAINT "sessions_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."sessions" ADD CONSTRAINT "sessions_token_hash_key" UNIQUE (token_hash);
ALTER TABLE ONLY "public"."sessions" ADD CONSTRAINT "sessions_user_id_fkey" FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE ONLY "public"."tag_cooccurrence_projections" ADD CONSTRAINT "tag_cooccurrence_projections_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."tag_events" ADD CONSTRAINT "tag_events_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."tag_merge_history" ADD CONSTRAINT "tag_merge_history_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."tag_projections" ADD CONSTRAINT "tag_projections_pkey" PRIMARY KEY (name);
ALTER TABLE ONLY "public"."tags" ADD CONSTRAINT "tags_pkey" PRIMARY KEY (name);
ALTER TABLE ONLY "public"."user_profile_facts" ADD CONSTRAINT "user_profile_facts_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."users" ADD CONSTRAINT "users_pkey" PRIMARY KEY (id);

CREATE UNIQUE INDEX api_token_facts_source_idx ON public.api_token_facts USING btree (source, source_id);
CREATE INDEX api_token_facts_token_created_idx ON public.api_token_facts USING btree (token_id, created_at_unix_nano, sequence, id);
CREATE UNIQUE INDEX api_token_facts_token_sequence_idx ON public.api_token_facts USING btree (token_id, sequence);
CREATE INDEX api_token_facts_user_created_idx ON public.api_token_facts USING btree (user_id, created_at_unix_nano, id);
CREATE INDEX api_tokens_user_id_idx ON public.api_tokens USING btree (user_id);
CREATE INDEX append_only_parity_runs_domain_created_idx ON public.append_only_parity_runs USING btree (domain, created_at_unix_nano DESC, id DESC);
CREATE INDEX auth_accounts_user_id_idx ON public.auth_accounts USING btree (user_id);
CREATE INDEX curation_proposals_kind_idx ON public.curation_proposals USING btree (kind);
CREATE INDEX curation_proposals_status_created_idx ON public.curation_proposals USING btree (status, created_at_unix_nano DESC, id);
CREATE INDEX custom_regions_created_idx ON public.custom_regions USING btree (created_at_unix_nano DESC, id);
CREATE INDEX events_created_at_idx ON public.events USING btree (created_at_unix_nano DESC, id DESC);
CREATE INDEX events_issue_id_idx ON public.events USING btree (issue_id);
CREATE INDEX events_kind_idx ON public.events USING btree (kind);
CREATE INDEX issue_content_facts_issue_created_idx ON public.issue_content_facts USING btree (issue_id, created_at_unix_nano, id);
CREATE UNIQUE INDEX issue_content_facts_issue_sequence_idx ON public.issue_content_facts USING btree (issue_id, sequence);
CREATE UNIQUE INDEX issue_content_facts_source_idx ON public.issue_content_facts USING btree (source, source_id);
CREATE INDEX issue_content_projections_embedding_vector_cosine_hnsw_idx ON public.issue_content_projections USING hnsw (((embedding_vector)::vector(1536)) vector_cosine_ops) WHERE ((embedding_vector IS NOT NULL) AND (vector_dims(embedding_vector) = 1536));
CREATE INDEX issue_content_projections_search_bm25_idx ON public.issue_content_projections USING bm25 (issue_id, search_title, search_body, search_tags, updated_at_unix_nano) WITH (key_field=issue_id);
CREATE INDEX issue_content_projections_updated_idx ON public.issue_content_projections USING btree (updated_at_unix_nano DESC, issue_id);
CREATE INDEX issue_enrichment_events_issue_created_idx ON public.issue_enrichment_events USING btree (issue_id, created_at_unix_nano DESC, id DESC);
CREATE INDEX issue_enrichment_events_target_idx ON public.issue_enrichment_events USING btree (issue_id, target_sequence, created_at_unix_nano, id);
CREATE INDEX issue_enrichment_jobs_lease_idx ON public.issue_enrichment_jobs USING btree (lease_expires_at_unix_nano, updated_at_unix_nano, issue_id);
CREATE INDEX issue_enrichment_projections_status_idx ON public.issue_enrichment_projections USING btree (status, target_sequence, issue_id);
CREATE INDEX issue_lifecycle_facts_issue_created_idx ON public.issue_lifecycle_facts USING btree (issue_id, created_at_unix_nano, sequence, id);
CREATE UNIQUE INDEX issue_lifecycle_facts_source_idx ON public.issue_lifecycle_facts USING btree (source, source_id);
CREATE INDEX issue_lifecycle_projections_status_idx ON public.issue_lifecycle_projections USING btree (status, assigned_to, issue_id);
CREATE INDEX issue_links_source_issue_id_idx ON public.issue_links USING btree (source_issue_id, created_at_unix_nano);
CREATE INDEX issue_links_target_issue_id_idx ON public.issue_links USING btree (target_issue_id, created_at_unix_nano);
CREATE UNIQUE INDEX issue_links_unique_logical_idx ON public.issue_links USING btree (source_issue_id, target_issue_id, type);
CREATE INDEX issue_operation_participants_issue_id_idx ON public.issue_operation_participants USING btree (issue_id, sequence);
CREATE INDEX issue_operation_participants_operation_id_idx ON public.issue_operation_participants USING btree (operation_id);
CREATE INDEX issue_posts_kind_idx ON public.issue_posts USING btree (kind);
CREATE INDEX issues_assigned_to_idx ON public.issues USING btree (assigned_to);
CREATE INDEX issues_embedding_vector_cosine_hnsw_idx ON public.issues USING hnsw (((embedding_vector)::vector(1536)) vector_cosine_ops) WHERE ((embedding_vector IS NOT NULL) AND (vector_dims(embedding_vector) = 1536));
CREATE INDEX issues_status_idx ON public.issues USING btree (status);
CREATE UNIQUE INDEX memories_concept_subject_tag_unique_idx ON public.memories USING btree (subject_tag) WHERE ((kind = 'concept'::text) AND (status = 'active'::text));
CREATE INDEX memories_created_idx ON public.memories USING btree (created_at_unix_nano DESC, id);
CREATE INDEX memories_embedding_vector_cosine_hnsw_idx ON public.memories USING hnsw (((embedding_vector)::vector(1536)) vector_cosine_ops) WHERE ((embedding_vector IS NOT NULL) AND (vector_dims(embedding_vector) = 1536));
CREATE UNIQUE INDEX memories_overview_unique_idx ON public.memories USING btree (kind) WHERE ((kind = 'overview'::text) AND (status = 'active'::text));
CREATE INDEX memories_status_idx ON public.memories USING btree (status);
CREATE INDEX memory_proposals_status_created_idx ON public.memory_proposals USING btree (status, created_at_unix_nano DESC, id);
CREATE INDEX sessions_expires_at_unix_nano_idx ON public.sessions USING btree (expires_at_unix_nano);
CREATE INDEX sessions_user_id_idx ON public.sessions USING btree (user_id);
CREATE UNIQUE INDEX tag_events_source_idx ON public.tag_events USING btree (source, source_id);
CREATE INDEX tag_events_tag_created_idx ON public.tag_events USING btree (tag_name, created_at_unix_nano, id);
CREATE UNIQUE INDEX tag_events_tag_sequence_idx ON public.tag_events USING btree (tag_name, sequence);
CREATE INDEX idx_tag_merge_history_alias ON public.tag_merge_history USING btree (alias_name);
CREATE INDEX idx_tag_merge_history_canonical ON public.tag_merge_history USING btree (canonical_name);
CREATE INDEX tag_projections_canonical_name_idx ON public.tag_projections USING btree (canonical_name, name) WHERE (canonical_name <> ''::text);
CREATE INDEX tag_projections_embedding_vector_cosine_hnsw_idx ON public.tag_projections USING hnsw (((embedding_vector)::vector(1536)) vector_cosine_ops) WHERE ((embedding_vector IS NOT NULL) AND (vector_dims(embedding_vector) = 1536));
CREATE INDEX tag_projections_status_name_idx ON public.tag_projections USING btree (status, name);
CREATE INDEX tags_embedding_vector_cosine_hnsw_idx ON public.tags USING hnsw (((embedding_vector)::vector(1536)) vector_cosine_ops) WHERE ((embedding_vector IS NOT NULL) AND (vector_dims(embedding_vector) = 1536));
CREATE UNIQUE INDEX user_profile_facts_source_idx ON public.user_profile_facts USING btree (source, source_id);
CREATE INDEX user_profile_facts_user_observed_idx ON public.user_profile_facts USING btree (user_id, observed_at_unix_nano, sequence, id);
CREATE UNIQUE INDEX user_profile_facts_user_sequence_idx ON public.user_profile_facts USING btree (user_id, sequence);
