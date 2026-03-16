CREATE EXTENSION IF NOT EXISTS "vector" WITH SCHEMA "public";

CREATE SEQUENCE "public"."dismissed_tag_merges_id_seq";
CREATE SEQUENCE "public"."tag_merge_history_id_seq";

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
CREATE TABLE "public"."auth_accounts" (
    "id" text NOT NULL,
    "user_id" text NOT NULL,
    "provider" text NOT NULL,
    "provider_user_id" text NOT NULL,
    "created_at_unix_nano" bigint NOT NULL
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
CREATE TABLE "public"."issue_enrichment_jobs" (
    "issue_id" text NOT NULL,
    "target_sequence" bigint NOT NULL,
    "lease_expires_at_unix_nano" bigint NOT NULL,
    "attempt_count" bigint NOT NULL,
    "created_at_unix_nano" bigint NOT NULL,
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
    "closed_at_unix_nano" bigint NOT NULL,
    "closed_by" text NOT NULL,
    "tag_scores_json" jsonb NOT NULL,
    "assigned_to" text NOT NULL,
    "embedding_vector" vector,
    "enrichment_status" text NOT NULL,
    "enrichment_error" text NOT NULL,
    "enrichment_target_sequence" bigint NOT NULL,
    "closed_reason" text NOT NULL,
    "closed_reason_note" text NOT NULL
);
CREATE TABLE "public"."map_projections" (
    "revision" bigint NOT NULL,
    "payload_json" jsonb NOT NULL,
    "created_at_unix_nano" bigint NOT NULL
);
CREATE TABLE "public"."sessions" (
    "id" text NOT NULL,
    "user_id" text NOT NULL,
    "token_hash" text NOT NULL,
    "expires_at_unix_nano" bigint NOT NULL,
    "created_at_unix_nano" bigint NOT NULL
);
CREATE TABLE "public"."tag_merge_history" (
    "id" bigint NOT NULL,
    "canonical_name" text NOT NULL,
    "alias_name" text NOT NULL,
    "merged_at" timestamp with time zone NOT NULL,
    "merged_by" text NOT NULL
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

ALTER TABLE ONLY "public"."api_tokens" ALTER COLUMN "revoked_at_unix_nano" SET DEFAULT 0;
ALTER TABLE ONLY "public"."api_tokens" ALTER COLUMN "name" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."api_tokens" ALTER COLUMN "last_used_at_unix_nano" SET DEFAULT 0;
ALTER TABLE ONLY "public"."dismissed_tag_merges" ALTER COLUMN "id" SET DEFAULT nextval('dismissed_tag_merges_id_seq'::regclass);
ALTER TABLE ONLY "public"."dismissed_tag_merges" ALTER COLUMN "dismissed_at" SET DEFAULT now();
ALTER TABLE ONLY "public"."events" ALTER COLUMN "body" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."events" ALTER COLUMN "participants_json" SET DEFAULT '[]'::jsonb;
ALTER TABLE ONLY "public"."issue_enrichment_jobs" ALTER COLUMN "lease_expires_at_unix_nano" SET DEFAULT 0;
ALTER TABLE ONLY "public"."issue_enrichment_jobs" ALTER COLUMN "attempt_count" SET DEFAULT 0;
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
ALTER TABLE ONLY "public"."issues" ALTER COLUMN "enrichment_status" SET DEFAULT 'complete'::text;
ALTER TABLE ONLY "public"."issues" ALTER COLUMN "enrichment_error" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issues" ALTER COLUMN "enrichment_target_sequence" SET DEFAULT 1;
ALTER TABLE ONLY "public"."issues" ALTER COLUMN "closed_reason" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."issues" ALTER COLUMN "closed_reason_note" SET DEFAULT ''::text;
ALTER TABLE ONLY "public"."tag_merge_history" ALTER COLUMN "id" SET DEFAULT nextval('tag_merge_history_id_seq'::regclass);
ALTER TABLE ONLY "public"."tag_merge_history" ALTER COLUMN "merged_at" SET DEFAULT now();
ALTER TABLE ONLY "public"."tag_merge_history" ALTER COLUMN "merged_by" SET DEFAULT ''::text;

ALTER TABLE ONLY "public"."api_tokens" ADD CONSTRAINT "api_tokens_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."api_tokens" ADD CONSTRAINT "api_tokens_token_hash_key" UNIQUE (token_hash);
ALTER TABLE ONLY "public"."api_tokens" ADD CONSTRAINT "api_tokens_user_id_fkey" FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE ONLY "public"."auth_accounts" ADD CONSTRAINT "auth_accounts_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."auth_accounts" ADD CONSTRAINT "auth_accounts_provider_provider_user_id_key" UNIQUE (provider, provider_user_id);
ALTER TABLE ONLY "public"."auth_accounts" ADD CONSTRAINT "auth_accounts_user_id_fkey" FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE ONLY "public"."dismissed_tag_merges" ADD CONSTRAINT "dismissed_tag_merges_canonical_name_alias_name_key" UNIQUE (canonical_name, alias_name);
ALTER TABLE ONLY "public"."dismissed_tag_merges" ADD CONSTRAINT "dismissed_tag_merges_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."events" ADD CONSTRAINT "events_issue_id_fkey" FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE SET NULL;
ALTER TABLE ONLY "public"."events" ADD CONSTRAINT "events_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."issue_enrichment_jobs" ADD CONSTRAINT "issue_enrichment_jobs_issue_id_fkey" FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE CASCADE;
ALTER TABLE ONLY "public"."issue_enrichment_jobs" ADD CONSTRAINT "issue_enrichment_jobs_pkey" PRIMARY KEY (issue_id);
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
ALTER TABLE ONLY "public"."sessions" ADD CONSTRAINT "sessions_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."sessions" ADD CONSTRAINT "sessions_token_hash_key" UNIQUE (token_hash);
ALTER TABLE ONLY "public"."sessions" ADD CONSTRAINT "sessions_user_id_fkey" FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE ONLY "public"."tag_merge_history" ADD CONSTRAINT "tag_merge_history_pkey" PRIMARY KEY (id);
ALTER TABLE ONLY "public"."tags" ADD CONSTRAINT "tags_pkey" PRIMARY KEY (name);
ALTER TABLE ONLY "public"."users" ADD CONSTRAINT "users_pkey" PRIMARY KEY (id);

CREATE INDEX api_tokens_user_id_idx ON public.api_tokens USING btree (user_id);
CREATE INDEX auth_accounts_user_id_idx ON public.auth_accounts USING btree (user_id);
CREATE INDEX events_created_at_idx ON public.events USING btree (created_at_unix_nano DESC, id DESC);
CREATE INDEX events_issue_id_idx ON public.events USING btree (issue_id);
CREATE INDEX events_kind_idx ON public.events USING btree (kind);
CREATE INDEX issue_enrichment_jobs_lease_idx ON public.issue_enrichment_jobs USING btree (lease_expires_at_unix_nano, updated_at_unix_nano, issue_id);
CREATE INDEX issue_links_source_issue_id_idx ON public.issue_links USING btree (source_issue_id, created_at_unix_nano);
CREATE INDEX issue_links_target_issue_id_idx ON public.issue_links USING btree (target_issue_id, created_at_unix_nano);
CREATE UNIQUE INDEX issue_links_unique_logical_idx ON public.issue_links USING btree (source_issue_id, target_issue_id, type);
CREATE INDEX issue_operation_participants_issue_id_idx ON public.issue_operation_participants USING btree (issue_id, sequence);
CREATE INDEX issue_operation_participants_operation_id_idx ON public.issue_operation_participants USING btree (operation_id);
CREATE INDEX issue_posts_kind_idx ON public.issue_posts USING btree (kind);
CREATE INDEX issues_assigned_to_idx ON public.issues USING btree (assigned_to);
CREATE INDEX issues_embedding_vector_cosine_hnsw_idx ON public.issues USING hnsw (((embedding_vector)::vector(1536)) vector_cosine_ops) WHERE ((embedding_vector IS NOT NULL) AND (vector_dims(embedding_vector) = 1536));
CREATE INDEX issues_status_idx ON public.issues USING btree (status);
CREATE INDEX sessions_expires_at_unix_nano_idx ON public.sessions USING btree (expires_at_unix_nano);
CREATE INDEX sessions_user_id_idx ON public.sessions USING btree (user_id);
CREATE INDEX idx_tag_merge_history_alias ON public.tag_merge_history USING btree (alias_name);
CREATE INDEX idx_tag_merge_history_canonical ON public.tag_merge_history USING btree (canonical_name);
CREATE INDEX tags_embedding_vector_cosine_hnsw_idx ON public.tags USING hnsw (((embedding_vector)::vector(1536)) vector_cosine_ops) WHERE ((embedding_vector IS NOT NULL) AND (vector_dims(embedding_vector) = 1536));
