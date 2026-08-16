-- Database baseline migration (atlas single-file)
-- Generated from production schema (PostgreSQL 18) after cleanup:
--   - tool_calls 外键已移除（日志型表，应用层保证有效性）
--   - media_assets.parent_id 已加入（文件夹支持）
--   - RLS 已全部禁用（安全由应用层托底）
-- New environments: applies full schema. Existing environments: already applied (version 20260709000001).


CREATE FUNCTION knowledge_chunks_search_vector_update() RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
NEW.search_vector := to_tsvector('chinese', COALESCE(NEW.content, ''));
RETURN NEW;
END;
$$;

CREATE FUNCTION update_updated_at_column() RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
NEW.updated_at = NOW();
RETURN NEW;
END;
$$;

CREATE TABLE agent_registry (
agent_type character varying(32) NOT NULL,
name character varying(128) NOT NULL,
description text DEFAULT ''::text NOT NULL,
enabled boolean DEFAULT true NOT NULL,
config jsonb DEFAULT '{}'::jsonb,
created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE agent_sessions (
id character varying(128) NOT NULL,
user_id uuid NOT NULL,
agent_id uuid,
name character varying(128) NOT NULL,
task text NOT NULL,
status character varying(16) DEFAULT 'pending'::character varying NOT NULL,
result text,
created_at timestamp with time zone DEFAULT now() NOT NULL,
updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE agents (
id uuid DEFAULT gen_random_uuid() NOT NULL,
tenant_id uuid NOT NULL,
name character varying(255) NOT NULL,
description text,
system_prompt text,
tools jsonb DEFAULT '[]'::jsonb,
llm_config jsonb DEFAULT '{}'::jsonb,
max_turns integer DEFAULT 10,
timeout_seconds integer DEFAULT 120,
enabled boolean DEFAULT true NOT NULL,
created_at timestamp with time zone DEFAULT now() NOT NULL,
updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE api_keys (
id uuid DEFAULT gen_random_uuid() NOT NULL,
user_id uuid NOT NULL,
name character varying(128) NOT NULL,
key_hash character varying(64) NOT NULL,
last_used_at timestamp with time zone,
expires_at timestamp with time zone,
created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE audit_logs (
id uuid DEFAULT gen_random_uuid() NOT NULL,
tenant_id uuid NOT NULL,
user_id uuid,
action character varying(64) NOT NULL,
resource_type character varying(64) NOT NULL,
resource_id character varying(64),
details jsonb DEFAULT '{}'::jsonb,
ip_address character varying(45),
created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE billing_records (
id uuid DEFAULT gen_random_uuid() NOT NULL,
tenant_id uuid NOT NULL,
user_id uuid NOT NULL,
session_id uuid,
input_tokens bigint DEFAULT 0 NOT NULL,
output_tokens bigint DEFAULT 0 NOT NULL,
cost_cents integer DEFAULT 0 NOT NULL,
created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE credit_transactions (
id uuid DEFAULT gen_random_uuid() NOT NULL,
user_id uuid NOT NULL,
amount integer NOT NULL,
balance integer NOT NULL,
reason character varying(64) NOT NULL,
created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE enterprise_tasks (
id uuid DEFAULT gen_random_uuid() NOT NULL,
tenant_id uuid NOT NULL,
user_id character varying(32) DEFAULT ''::character varying NOT NULL,
title character varying(255) NOT NULL,
description text DEFAULT ''::text,
project character varying(128) DEFAULT ''::character varying,
assignee character varying(128) DEFAULT ''::character varying,
priority character varying(16) DEFAULT 'medium'::character varying NOT NULL,
status character varying(16) DEFAULT 'open'::character varying NOT NULL,
created_at timestamp with time zone DEFAULT now() NOT NULL,
updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE episodes (
id uuid DEFAULT gen_random_uuid() NOT NULL,
task text DEFAULT ''::text NOT NULL,
summary text DEFAULT ''::text NOT NULL,
tools_used text[] DEFAULT '{}'::text[] NOT NULL,
success boolean DEFAULT true NOT NULL,
duration_ms bigint DEFAULT 0 NOT NULL,
created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE guest_storage (
client_id character varying(64) NOT NULL,
storage_id character varying(64) NOT NULL,
created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE kb_articles (
id uuid DEFAULT gen_random_uuid() NOT NULL,
tenant_id uuid NOT NULL,
user_id character varying(32) DEFAULT ''::character varying NOT NULL,
title character varying(255) NOT NULL,
content text DEFAULT ''::text NOT NULL,
tags text[] DEFAULT '{}'::text[],
category character varying(64) DEFAULT ''::character varying,
created_at timestamp with time zone DEFAULT now() NOT NULL,
updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE knowledge_bases (
id uuid DEFAULT gen_random_uuid() NOT NULL,
tenant_id uuid NOT NULL,
user_id uuid NOT NULL,
name character varying(255) NOT NULL,
description text,
type character varying(32) DEFAULT 'rag'::character varying NOT NULL,
visibility character varying(32) DEFAULT 'private'::character varying NOT NULL,
status character varying(32) DEFAULT 'active'::character varying NOT NULL,
document_count integer DEFAULT 0,
total_size_bytes bigint DEFAULT 0,
credits_consumed integer DEFAULT 0,
config jsonb DEFAULT '{}'::jsonb,
created_at timestamp with time zone DEFAULT now(),
updated_at timestamp with time zone DEFAULT now(),
doc_count integer DEFAULT 0
);

CREATE TABLE knowledge_chunks (
id uuid DEFAULT gen_random_uuid() NOT NULL,
document_id uuid NOT NULL,
knowledge_base_id uuid NOT NULL,
tenant_id uuid NOT NULL,
chunk_index integer NOT NULL,
content text NOT NULL,
metadata jsonb DEFAULT '{}'::jsonb,
search_vector tsvector,
created_at timestamp with time zone DEFAULT now()
);

CREATE TABLE knowledge_documents (
id uuid DEFAULT gen_random_uuid() NOT NULL,
knowledge_base_id uuid NOT NULL,
tenant_id uuid NOT NULL,
user_id uuid NOT NULL,
name character varying(255) NOT NULL,
file_url character varying(1024),
file_type character varying(32),
file_size_bytes bigint DEFAULT 0,
chunk_count integer DEFAULT 0,
status character varying(32) DEFAULT 'pending'::character varying NOT NULL,
error_message text,
metadata jsonb DEFAULT '{}'::jsonb,
created_at timestamp with time zone DEFAULT now(),
updated_at timestamp with time zone DEFAULT now(),
content bytea
);

CREATE TABLE marketing_campaigns (
id uuid DEFAULT gen_random_uuid() NOT NULL,
tenant_id uuid NOT NULL,
user_id character varying(32) DEFAULT ''::character varying NOT NULL,
name character varying(255) NOT NULL,
description text DEFAULT ''::text,
campaign_type character varying(32) DEFAULT 'email'::character varying NOT NULL,
config jsonb DEFAULT '{}'::jsonb,
status character varying(16) DEFAULT 'draft'::character varying NOT NULL,
created_at timestamp with time zone DEFAULT now() NOT NULL,
updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE media_assets (
id uuid DEFAULT gen_random_uuid() NOT NULL,
tenant_id uuid NOT NULL,
user_id uuid NOT NULL,
type character varying(16) DEFAULT 'text'::character varying NOT NULL,
name character varying(255) NOT NULL,
file_url character varying(1024) DEFAULT ''::character varying,
file_path character varying(512) DEFAULT ''::character varying,
mime_type character varying(64) DEFAULT ''::character varying,
thumbnail character varying(512) DEFAULT ''::character varying,
metadata jsonb DEFAULT '{}'::jsonb,
tags text[] DEFAULT '{}'::text[],
category character varying(64) DEFAULT ''::character varying,
size bigint DEFAULT 0 NOT NULL,
created_at timestamp with time zone DEFAULT now() NOT NULL,
updated_at timestamp with time zone DEFAULT now() NOT NULL,
parent_id character varying(64) DEFAULT ''::character varying
);

CREATE TABLE meeting_notes (
id uuid DEFAULT gen_random_uuid() NOT NULL,
tenant_id uuid NOT NULL,
user_id character varying(32) DEFAULT ''::character varying NOT NULL,
title character varying(255) NOT NULL,
notes text DEFAULT ''::text NOT NULL,
summary text DEFAULT ''::text,
participants text[] DEFAULT '{}'::text[],
date date DEFAULT CURRENT_DATE,
created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE messages (
id uuid DEFAULT gen_random_uuid() NOT NULL,
session_id uuid NOT NULL,
role character varying(16) NOT NULL,
content text DEFAULT ''::text NOT NULL,
tool_calls jsonb,
created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE okrs (
id uuid DEFAULT gen_random_uuid() NOT NULL,
tenant_id uuid NOT NULL,
user_id character varying(32) DEFAULT ''::character varying NOT NULL,
objective character varying(255) NOT NULL,
key_results jsonb DEFAULT '[]'::jsonb,
quarter character varying(16) DEFAULT ''::character varying,
status character varying(16) DEFAULT 'active'::character varying NOT NULL,
created_at timestamp with time zone DEFAULT now() NOT NULL,
updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE schema_migrations (
version bigint NOT NULL,
name character varying(255) NOT NULL,
checksum character varying(128),
applied_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE sessions (
id uuid DEFAULT gen_random_uuid() NOT NULL,
tenant_id uuid NOT NULL,
user_id uuid,
agent_id uuid,
title character varying(255) DEFAULT ''::character varying NOT NULL,
status character varying(16) DEFAULT 'active'::character varying NOT NULL,
created_at timestamp with time zone DEFAULT now() NOT NULL,
updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE stripe_payments (
session_id character varying(128) NOT NULL,
user_id uuid NOT NULL,
credits integer DEFAULT 1000 NOT NULL,
amount_cents bigint DEFAULT 0 NOT NULL,
status character varying(16) DEFAULT 'pending'::character varying NOT NULL,
created_at timestamp with time zone DEFAULT now() NOT NULL,
completed_at timestamp with time zone
);

CREATE TABLE support_tickets (
id uuid DEFAULT gen_random_uuid() NOT NULL,
tenant_id uuid NOT NULL,
user_id character varying(32) DEFAULT ''::character varying NOT NULL,
subject character varying(255) NOT NULL,
description text DEFAULT ''::text,
priority character varying(16) DEFAULT 'medium'::character varying NOT NULL,
status character varying(16) DEFAULT 'open'::character varying NOT NULL,
assignee character varying(128) DEFAULT ''::character varying,
created_at timestamp with time zone DEFAULT now() NOT NULL,
updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE tasks (
id uuid DEFAULT gen_random_uuid() NOT NULL,
user_id uuid NOT NULL,
type character varying(32) NOT NULL,
status character varying(16) DEFAULT 'pending'::character varying NOT NULL,
priority integer DEFAULT 0 NOT NULL,
payload jsonb DEFAULT '{}'::jsonb NOT NULL,
result jsonb,
error text,
retries integer DEFAULT 0 NOT NULL,
max_retries integer DEFAULT 3 NOT NULL,
created_at timestamp with time zone DEFAULT now() NOT NULL,
updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE tenants (
id uuid DEFAULT gen_random_uuid() NOT NULL,
name character varying(255) NOT NULL,
created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE tool_calls (
id uuid DEFAULT gen_random_uuid() NOT NULL,
session_id uuid NOT NULL,
message_id uuid,
tool_name character varying(128) NOT NULL,
input jsonb DEFAULT '{}'::jsonb NOT NULL,
output text DEFAULT ''::text NOT NULL,
is_error boolean DEFAULT false NOT NULL,
duration_ms bigint DEFAULT 0 NOT NULL,
created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE users (
id uuid DEFAULT gen_random_uuid() NOT NULL,
tenant_id uuid NOT NULL,
email character varying(255) NOT NULL,
name character varying(128) NOT NULL,
password_hash character varying(255) NOT NULL,
role character varying(16) DEFAULT 'user'::character varying NOT NULL,
storage_id character varying(64),
credits integer DEFAULT 1000 NOT NULL,
created_at timestamp with time zone DEFAULT now() NOT NULL,
updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE wiki_pages (
id uuid DEFAULT gen_random_uuid() NOT NULL,
tenant_id uuid NOT NULL,
user_id character varying(32) DEFAULT ''::character varying NOT NULL,
title character varying(255) NOT NULL,
content text DEFAULT ''::text NOT NULL,
tags text[] DEFAULT '{}'::text[],
created_at timestamp with time zone DEFAULT now() NOT NULL,
updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE workflow_graphs (
id character varying(32) NOT NULL,
name character varying(255) NOT NULL,
user_id uuid,
graph_json jsonb DEFAULT '{}'::jsonb NOT NULL,
created_at timestamp with time zone DEFAULT now() NOT NULL,
updated_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY agent_registry
ADD CONSTRAINT agent_registry_pkey PRIMARY KEY (agent_type);

ALTER TABLE ONLY agent_sessions
ADD CONSTRAINT agent_sessions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY agents
ADD CONSTRAINT agents_pkey PRIMARY KEY (id);

ALTER TABLE ONLY api_keys
ADD CONSTRAINT api_keys_pkey PRIMARY KEY (id);

ALTER TABLE ONLY audit_logs
ADD CONSTRAINT audit_logs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY billing_records
ADD CONSTRAINT billing_records_pkey PRIMARY KEY (id);

ALTER TABLE ONLY credit_transactions
ADD CONSTRAINT credit_transactions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY enterprise_tasks
ADD CONSTRAINT enterprise_tasks_pkey PRIMARY KEY (id);

ALTER TABLE ONLY episodes
ADD CONSTRAINT episodes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY guest_storage
ADD CONSTRAINT guest_storage_pkey PRIMARY KEY (client_id);

ALTER TABLE ONLY guest_storage
ADD CONSTRAINT guest_storage_storage_id_key UNIQUE (storage_id);

ALTER TABLE ONLY kb_articles
ADD CONSTRAINT kb_articles_pkey PRIMARY KEY (id);

ALTER TABLE ONLY knowledge_bases
ADD CONSTRAINT knowledge_bases_pkey PRIMARY KEY (id);

ALTER TABLE ONLY knowledge_chunks
ADD CONSTRAINT knowledge_chunks_pkey PRIMARY KEY (id);

ALTER TABLE ONLY knowledge_documents
ADD CONSTRAINT knowledge_documents_pkey PRIMARY KEY (id);

ALTER TABLE ONLY marketing_campaigns
ADD CONSTRAINT marketing_campaigns_pkey PRIMARY KEY (id);

ALTER TABLE ONLY media_assets
ADD CONSTRAINT media_assets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY meeting_notes
ADD CONSTRAINT meeting_notes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY messages
ADD CONSTRAINT messages_pkey PRIMARY KEY (id);

ALTER TABLE ONLY okrs
ADD CONSTRAINT okrs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY schema_migrations
ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);

ALTER TABLE ONLY sessions
ADD CONSTRAINT sessions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY stripe_payments
ADD CONSTRAINT stripe_payments_pkey PRIMARY KEY (session_id);

ALTER TABLE ONLY support_tickets
ADD CONSTRAINT support_tickets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY tasks
ADD CONSTRAINT tasks_pkey PRIMARY KEY (id);

ALTER TABLE ONLY tenants
ADD CONSTRAINT tenants_pkey PRIMARY KEY (id);

ALTER TABLE ONLY tool_calls
ADD CONSTRAINT tool_calls_pkey PRIMARY KEY (id);

ALTER TABLE ONLY users
ADD CONSTRAINT users_pkey PRIMARY KEY (id);

ALTER TABLE ONLY users
ADD CONSTRAINT users_storage_id_key UNIQUE (storage_id);

ALTER TABLE ONLY users
ADD CONSTRAINT users_tenant_id_email_key UNIQUE (tenant_id, email);

ALTER TABLE ONLY wiki_pages
ADD CONSTRAINT wiki_pages_pkey PRIMARY KEY (id);

ALTER TABLE ONLY workflow_graphs
ADD CONSTRAINT workflow_graphs_pkey PRIMARY KEY (id);

CREATE INDEX idx_agent_sessions_status ON agent_sessions USING btree (status);

CREATE INDEX idx_agent_sessions_user ON agent_sessions USING btree (user_id);

CREATE INDEX idx_agents_name ON agents USING btree (tenant_id, name);

CREATE INDEX idx_agents_tenant ON agents USING btree (tenant_id);

CREATE INDEX idx_api_keys_user ON api_keys USING btree (user_id);

CREATE INDEX idx_audit_action ON audit_logs USING btree (action);

CREATE INDEX idx_audit_created ON audit_logs USING btree (created_at);

CREATE INDEX idx_audit_tenant ON audit_logs USING btree (tenant_id);

CREATE INDEX idx_audit_user ON audit_logs USING btree (user_id);

CREATE INDEX idx_billing_created ON billing_records USING btree (created_at);

CREATE INDEX idx_billing_session ON billing_records USING btree (session_id);

CREATE INDEX idx_billing_tenant ON billing_records USING btree (tenant_id);

CREATE INDEX idx_billing_user ON billing_records USING btree (user_id);

CREATE INDEX idx_credit_tx_user ON credit_transactions USING btree (user_id, created_at DESC);

CREATE INDEX idx_enterprise_tasks_tenant ON enterprise_tasks USING btree (tenant_id);

CREATE INDEX idx_guest_storage_id ON guest_storage USING btree (storage_id);

CREATE INDEX idx_kb_articles_tenant ON kb_articles USING btree (tenant_id);

CREATE INDEX idx_knowledge_bases_status ON knowledge_bases USING btree (status);

CREATE INDEX idx_knowledge_bases_tenant ON knowledge_bases USING btree (tenant_id);

CREATE INDEX idx_knowledge_bases_user ON knowledge_bases USING btree (user_id);

CREATE INDEX idx_knowledge_bases_visibility ON knowledge_bases USING btree (visibility);

CREATE INDEX idx_knowledge_chunks_doc ON knowledge_chunks USING btree (document_id);

CREATE INDEX idx_knowledge_chunks_kb ON knowledge_chunks USING btree (knowledge_base_id);

CREATE INDEX idx_knowledge_chunks_search ON knowledge_chunks USING gin (search_vector);

CREATE INDEX idx_knowledge_chunks_tenant ON knowledge_chunks USING btree (tenant_id);

CREATE INDEX idx_knowledge_documents_kb ON knowledge_documents USING btree (knowledge_base_id);

CREATE INDEX idx_knowledge_documents_status ON knowledge_documents USING btree (status);

CREATE INDEX idx_knowledge_documents_tenant ON knowledge_documents USING btree (tenant_id);

CREATE INDEX idx_marketing_campaigns_tenant ON marketing_campaigns USING btree (tenant_id);

CREATE INDEX idx_media_category ON media_assets USING btree (category);

CREATE INDEX idx_media_created ON media_assets USING btree (created_at);

CREATE INDEX idx_media_parent ON media_assets USING btree (parent_id);

CREATE INDEX idx_media_tenant ON media_assets USING btree (tenant_id);

CREATE INDEX idx_media_type ON media_assets USING btree (type);

CREATE INDEX idx_media_user ON media_assets USING btree (user_id);

CREATE INDEX idx_meeting_notes_tenant ON meeting_notes USING btree (tenant_id);

CREATE INDEX idx_messages_session ON messages USING btree (session_id, created_at);

CREATE INDEX idx_okrs_tenant ON okrs USING btree (tenant_id);

CREATE INDEX idx_sessions_agent ON sessions USING btree (agent_id);

CREATE INDEX idx_sessions_tenant ON sessions USING btree (tenant_id);

CREATE INDEX idx_sessions_updated ON sessions USING btree (updated_at);

CREATE INDEX idx_sessions_user ON sessions USING btree (user_id);

CREATE INDEX idx_stripe_payments_user ON stripe_payments USING btree (user_id);

CREATE INDEX idx_support_tickets_tenant ON support_tickets USING btree (tenant_id);

CREATE INDEX idx_tasks_status ON tasks USING btree (status);

CREATE INDEX idx_tasks_type ON tasks USING btree (type);

CREATE INDEX idx_tasks_user ON tasks USING btree (user_id);

CREATE INDEX idx_tool_calls_session ON tool_calls USING btree (session_id);

CREATE INDEX idx_users_email ON users USING btree (email);

CREATE INDEX idx_users_tenant ON users USING btree (tenant_id);

CREATE INDEX idx_wiki_pages_tenant ON wiki_pages USING btree (tenant_id);

CREATE INDEX idx_workflow_graphs_updated ON workflow_graphs USING btree (updated_at);

CREATE INDEX idx_workflow_graphs_user ON workflow_graphs USING btree (user_id);

CREATE TRIGGER knowledge_chunks_search_vector_trigger BEFORE INSERT OR UPDATE ON knowledge_chunks FOR EACH ROW EXECUTE FUNCTION knowledge_chunks_search_vector_update();

CREATE TRIGGER update_knowledge_bases_updated_at BEFORE UPDATE ON knowledge_bases FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_knowledge_documents_updated_at BEFORE UPDATE ON knowledge_documents FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE ONLY agent_sessions
ADD CONSTRAINT agent_sessions_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY agent_sessions
ADD CONSTRAINT agent_sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY agents
ADD CONSTRAINT agents_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE ONLY api_keys
ADD CONSTRAINT api_keys_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY audit_logs
ADD CONSTRAINT audit_logs_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE ONLY audit_logs
ADD CONSTRAINT audit_logs_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE ONLY billing_records
ADD CONSTRAINT billing_records_session_id_fkey FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE SET NULL;

ALTER TABLE ONLY billing_records
ADD CONSTRAINT billing_records_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE ONLY billing_records
ADD CONSTRAINT billing_records_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY credit_transactions
ADD CONSTRAINT credit_transactions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY enterprise_tasks
ADD CONSTRAINT enterprise_tasks_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE ONLY kb_articles
ADD CONSTRAINT kb_articles_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE ONLY knowledge_bases
ADD CONSTRAINT knowledge_bases_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE ONLY knowledge_bases
ADD CONSTRAINT knowledge_bases_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY knowledge_chunks
ADD CONSTRAINT knowledge_chunks_document_id_fkey FOREIGN KEY (document_id) REFERENCES knowledge_documents(id) ON DELETE CASCADE;

ALTER TABLE ONLY knowledge_chunks
ADD CONSTRAINT knowledge_chunks_knowledge_base_id_fkey FOREIGN KEY (knowledge_base_id) REFERENCES knowledge_bases(id) ON DELETE CASCADE;

ALTER TABLE ONLY knowledge_chunks
ADD CONSTRAINT knowledge_chunks_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE ONLY knowledge_documents
ADD CONSTRAINT knowledge_documents_knowledge_base_id_fkey FOREIGN KEY (knowledge_base_id) REFERENCES knowledge_bases(id) ON DELETE CASCADE;

ALTER TABLE ONLY knowledge_documents
ADD CONSTRAINT knowledge_documents_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE ONLY knowledge_documents
ADD CONSTRAINT knowledge_documents_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY marketing_campaigns
ADD CONSTRAINT marketing_campaigns_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE ONLY media_assets
ADD CONSTRAINT media_assets_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE ONLY meeting_notes
ADD CONSTRAINT meeting_notes_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE ONLY messages
ADD CONSTRAINT messages_session_id_fkey FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE;

ALTER TABLE ONLY okrs
ADD CONSTRAINT okrs_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE ONLY sessions
ADD CONSTRAINT sessions_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY sessions
ADD CONSTRAINT sessions_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE ONLY sessions
ADD CONSTRAINT sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE ONLY support_tickets
ADD CONSTRAINT support_tickets_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE ONLY tasks
ADD CONSTRAINT tasks_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY users
ADD CONSTRAINT users_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE ONLY wiki_pages
ADD CONSTRAINT wiki_pages_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

ALTER TABLE ONLY workflow_graphs
ADD CONSTRAINT workflow_graphs_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;
