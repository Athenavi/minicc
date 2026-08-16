// MiniCC V2.0 — Atlas HCL schema (enterprise architecture)
// Based on docs/enterprise/data-layer.md + minicc0710 database

env "local" {
  url = "postgres://postgres:postgres@localhost:5432/minicc0710?sslmode=disable"
  migration {
    dir = "file://migrations"
    format = atlas
  }
}

env "prod" {
  url = env("DATABASE_URL")
  migration {
    dir = "file://migrations"
    format = atlas
  }
}

// ── Schema ──

schema "public" {
  comment = "MiniCC V2 enterprise schema"
}

// ── Tenants ──

table "tenants" {
  schema = schema.public
  column "id"         { type = varchar(32)    }
  column "name"       { type = varchar(255) }
  column "created_at" { type = timestamptz }
  primary_key { columns = [column.id] }
  index "idx_tenants_name" { columns = [column.name] }
}

// ── Users ──

table "users" {
  schema = schema.public
  column "id"            { type = varchar(32) }
  column "tenant_id"     { type = varchar(32) }
  column "email"         { type = varchar(255) }
  column "name"          { type = varchar(128) }
  column "password_hash" { type = varchar(255) }
  column "role"          { type = varchar(16), default = "user" }
  column "storage_id"    { type = varchar(64), null = true }
  column "credits"       { type = integer, default = 1000 }
  column "created_at"    { type = timestamptz }
  column "updated_at"    { type = timestamptz }
  primary_key { columns = [column.id] }
  foreign_key "fk_users_tenant" {
    columns     = [column.tenant_id]
    ref_columns = [table.tenants.column.id]
    on_delete   = CASCADE
  }
  unique "uniq_users_tenant_email" { columns = [column.tenant_id, column.email] }
  unique "uniq_users_storage_id"   { columns = [column.storage_id] }
  index "idx_users_tenant"  { columns = [column.tenant_id] }
  index "idx_users_email"   { columns = [column.email] }
}

// ── API Keys ──

table "api_keys" {
  schema = schema.public
  column "id"           { type = varchar(32) }
  column "user_id"      { type = varchar(32) }
  column "name"         { type = varchar(128) }
  column "key_hash"     { type = varchar(64) }
  column "last_used_at" { type = timestamptz, null = true }
  column "expires_at"   { type = timestamptz, null = true }
  column "created_at"   { type = timestamptz }
  primary_key { columns = [column.id] }
  foreign_key "fk_api_keys_user" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  unique "uniq_api_keys_hash" { columns = [column.key_hash] }
  index "idx_api_keys_user"   { columns = [column.user_id] }
}

// ── Guest Storage ──

table "guest_storage" {
  schema = schema.public
  column "client_id"  { type = varchar(64) }
  column "storage_id" { type = varchar(64) }
  column "created_at" { type = timestamptz }
  primary_key { columns = [column.client_id] }
  unique "uniq_guest_storage_id" { columns = [column.storage_id] }
}

// ── Agents ──

table "agents" {
  schema = schema.public
  column "id"              { type = varchar(32) }
  column "tenant_id"       { type = varchar(32) }
  column "name"            { type = varchar(255) }
  column "description"     { type = text, null = true }
  column "system_prompt"   { type = text, null = true }
  column "tools"           { type = jsonb, null = true }
  column "llm_config"      { type = jsonb, null = true }
  column "max_turns"       { type = integer, null = true }
  column "timeout_seconds" { type = integer, null = true }
  column "enabled"         { type = boolean, default = true }
  column "created_at"      { type = timestamptz }
  column "updated_at"      { type = timestamptz }
  primary_key { columns = [column.id] }
  foreign_key "fk_agents_tenant" {
    columns     = [column.tenant_id]
    ref_columns = [table.tenants.column.id]
    on_delete   = CASCADE
  }
  index "idx_agents_tenant" { columns = [column.tenant_id] }
}

// ── Agent Registry ──

table "agent_registry" {
  schema = schema.public
  column "agent_type"  { type = varchar(32) }
  column "name"        { type = varchar(128) }
  column "description" { type = text }
  column "enabled"     { type = boolean, default = true }
  column "config"      { type = jsonb, null = true }
  column "created_at"  { type = timestamptz }
  primary_key { columns = [column.agent_type] }
}

// ── Agent Sessions ──

table "agent_sessions" {
  schema = schema.public
  column "id"         { type = varchar(32) }
  column "user_id"    { type = varchar(32) }
  column "agent_id"   { type = varchar(32), null = true }
  column "name"       { type = varchar(128) }
  column "task"       { type = text }
  column "status"     { type = varchar(16), default = "pending" }
  column "result"     { type = text, null = true }
  column "created_at" { type = timestamptz }
  column "updated_at" { type = timestamptz }
  primary_key { columns = [column.id] }
  foreign_key "fk_agent_sessions_user" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  foreign_key "fk_agent_sessions_agent" {
    columns     = [column.agent_id]
    ref_columns = [table.agents.column.id]
    on_delete   = SET_NULL
  }
  index "idx_agent_sessions_user"   { columns = [column.user_id] }
  index "idx_agent_sessions_status" { columns = [column.status] }
}

// ── Episodes ──

table "episodes" {
  schema = schema.public
  column "id"         { type = varchar(32) }
  column "task"       { type = text }
  column "summary"    { type = text }
  column "tools_used" { type = sql("text[]") }
  column "success"    { type = boolean, default = true }
  column "duration_ms" { type = bigint }
  column "created_at" { type = timestamptz }
  primary_key { columns = [column.id] }
}

// ── Sessions ──

table "sessions" {
  schema = schema.public
  column "id"         { type = varchar(32) }
  column "tenant_id"  { type = varchar(32) }
  column "user_id"    { type = varchar(32), null = true }
  column "agent_id"   { type = varchar(32), null = true }
  column "title"      { type = varchar(255) }
  column "status"     { type = varchar(16), default = "active" }
  column "created_at" { type = timestamptz }
  column "updated_at" { type = timestamptz }
  primary_key { columns = [column.id] }
  foreign_key "fk_sessions_tenant" {
    columns     = [column.tenant_id]
    ref_columns = [table.tenants.column.id]
    on_delete   = CASCADE
  }
  foreign_key "fk_sessions_user" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = SET_NULL
  }
  foreign_key "fk_sessions_agent" {
    columns     = [column.agent_id]
    ref_columns = [table.agents.column.id]
    on_delete   = SET_NULL
  }
  index "idx_sessions_tenant" { columns = [column.tenant_id] }
  index "idx_sessions_user"   { columns = [column.user_id] }
  index "idx_sessions_agent"  { columns = [column.agent_id] }
  index "idx_sessions_updated" { columns = [column.updated_at] }
}

// ── Messages ──

table "messages" {
  schema = schema.public
  column "id"         { type = varchar(32) }
  column "session_id" { type = varchar(32) }
  column "role"       { type = varchar(16) }
  column "content"    { type = text }
  column "tool_calls" { type = jsonb, null = true }
  column "created_at" { type = timestamptz }
  primary_key { columns = [column.id] }
  foreign_key "fk_messages_session" {
    columns     = [column.session_id]
    ref_columns = [table.sessions.column.id]
    on_delete   = CASCADE
  }
  index "idx_messages_session" { columns = [column.session_id, column.created_at] }
}

// ── Tool Calls ──

table "tool_calls" {
  schema = schema.public
  column "id"         { type = varchar(32) }
  column "session_id" { type = varchar(32) }
  column "message_id" { type = varchar(32), null = true }
  column "tool_name"  { type = varchar(128) }
  column "input"      { type = jsonb }
  column "output"     { type = text }
  column "is_error"   { type = boolean, default = false }
  column "duration_ms" { type = bigint }
  column "created_at" { type = timestamptz }
  primary_key { columns = [column.id] }
  foreign_key "fk_tool_calls_session" {
    columns     = [column.session_id]
    ref_columns = [table.sessions.column.id]
    on_delete   = CASCADE
  }
  index "idx_tool_calls_session" { columns = [column.session_id] }
}

// ── Workflow Graphs ──

table "workflow_graphs" {
  schema = schema.public
  column "id"         { type = varchar(32) }
  column "name"       { type = varchar(255) }
  column "user_id"    { type = varchar(32), null = true }
  column "graph_json" { type = jsonb }
  column "created_at" { type = timestamptz }
  column "updated_at" { type = timestamptz }
  primary_key { columns = [column.id] }
  foreign_key "fk_workflow_graphs_user" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = SET_NULL
  }
  index "idx_workflow_graphs_user"    { columns = [column.user_id] }
  index "idx_workflow_graphs_updated" { columns = [column.updated_at] }
}

// ── Tasks ──

table "tasks" {
  schema = schema.public
  column "id"          { type = varchar(32) }
  column "user_id"     { type = varchar(32) }
  column "type"        { type = varchar(32) }
  column "status"      { type = varchar(16), default = "pending" }
  column "priority"    { type = integer }
  column "payload"     { type = jsonb }
  column "result"      { type = jsonb, null = true }
  column "error"       { type = text, null = true }
  column "retries"     { type = integer }
  column "max_retries" { type = integer }
  column "created_at"  { type = timestamptz }
  column "updated_at"  { type = timestamptz }
  primary_key { columns = [column.id] }
  foreign_key "fk_tasks_user" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "idx_tasks_status" { columns = [column.status] }
  index "idx_tasks_user"   { columns = [column.user_id] }
  index "idx_tasks_type"   { columns = [column.type] }
}

// ── Billing Records ──

table "billing_records" {
  schema = schema.public
  column "id"            { type = varchar(32) }
  column "tenant_id"     { type = varchar(32) }
  column "user_id"       { type = varchar(32) }
  column "session_id"    { type = varchar(32), null = true }
  column "input_tokens"  { type = bigint }
  column "output_tokens" { type = bigint }
  column "cost_cents"    { type = integer }
  column "created_at"    { type = timestamptz }
  primary_key { columns = [column.id] }
  foreign_key "fk_billing_tenant" {
    columns     = [column.tenant_id]
    ref_columns = [table.tenants.column.id]
    on_delete   = CASCADE
  }
  foreign_key "fk_billing_user" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "idx_billing_tenant"  { columns = [column.tenant_id] }
  index "idx_billing_user"    { columns = [column.user_id] }
  index "idx_billing_created" { columns = [column.created_at] }
}

// ── Stripe Payments ──

table "stripe_payments" {
  schema = schema.public
  column "session_id"   { type = varchar(128) }
  column "user_id"      { type = varchar(32) }
  column "credits"      { type = integer }
  column "amount_cents" { type = bigint }
  column "status"       { type = varchar(16), default = "pending" }
  column "created_at"   { type = timestamptz }
  column "completed_at" { type = timestamptz, null = true }
  primary_key { columns = [column.session_id] }
  index "idx_stripe_payments_user" { columns = [column.user_id] }
}

// ── Credit Transactions ──

table "credit_transactions" {
  schema = schema.public
  column "id"         { type = varchar(32) }
  column "user_id"    { type = varchar(32) }
  column "amount"     { type = integer }
  column "balance"    { type = integer }
  column "reason"     { type = varchar(64) }
  column "created_at" { type = timestamptz }
  primary_key { columns = [column.id] }
  foreign_key "fk_credit_tx_user" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "idx_credit_tx_user" { columns = [column.user_id, column.created_at] }
}

// ── Media Assets ──

table "media_assets" {
  schema = schema.public
  column "id"         { type = varchar(32) }
  column "tenant_id"  { type = varchar(32) }
  column "user_id"    { type = varchar(32) }
  column "type"       { type = varchar(16), default = "text" }
  column "name"       { type = varchar(255) }
  column "file_url"   { type = varchar(1024) }
  column "file_path"  { type = varchar(512) }
  column "mime_type"  { type = varchar(64) }
  column "thumbnail"  { type = varchar(512) }
  column "metadata"   { type = jsonb }
  column "tags"       { type = sql("text[]") }
  column "category"   { type = varchar(64) }
  column "size"       { type = bigint }
  column "created_at" { type = timestamptz }
  column "updated_at" { type = timestamptz }
  primary_key { columns = [column.id] }
  foreign_key "fk_media_tenant" {
    columns     = [column.tenant_id]
    ref_columns = [table.tenants.column.id]
    on_delete   = CASCADE
  }
  index "idx_media_tenant"  { columns = [column.tenant_id] }
  index "idx_media_user"    { columns = [column.user_id] }
  index "idx_media_type"    { columns = [column.type] }
  index "idx_media_category" { columns = [column.category] }
  index "idx_media_created" { columns = [column.created_at] }
}

// ── Enterprise Tables ──

table "enterprise_tasks" {
  schema = schema.public
  column "id"          { type = varchar(32) }
  column "tenant_id"   { type = varchar(32) }
  column "user_id"     { type = varchar(32) }
  column "title"       { type = varchar(255) }
  column "description" { type = text }
  column "project"     { type = varchar(128) }
  column "assignee"    { type = varchar(128) }
  column "priority"    { type = varchar(16), default = "medium" }
  column "status"      { type = varchar(16), default = "open" }
  column "created_at"  { type = timestamptz }
  column "updated_at"  { type = timestamptz }
  primary_key { columns = [column.id] }
  foreign_key "fk_enterprise_tasks_tenant" {
    columns     = [column.tenant_id]
    ref_columns = [table.tenants.column.id]
    on_delete   = CASCADE
  }
  index "idx_enterprise_tasks_tenant" { columns = [column.tenant_id] }
}

table "wiki_pages" {
  schema = schema.public
  column "id"         { type = varchar(32) }
  column "tenant_id"  { type = varchar(32) }
  column "user_id"    { type = varchar(32) }
  column "title"      { type = varchar(255) }
  column "content"    { type = text }
  column "tags"       { type = sql("text[]") }
  column "created_at" { type = timestamptz }
  column "updated_at" { type = timestamptz }
  primary_key { columns = [column.id] }
  foreign_key "fk_wiki_pages_tenant" {
    columns     = [column.tenant_id]
    ref_columns = [table.tenants.column.id]
    on_delete   = CASCADE
  }
  index "idx_wiki_pages_tenant" { columns = [column.tenant_id] }
}

table "okrs" {
  schema = schema.public
  column "id"          { type = varchar(32) }
  column "tenant_id"   { type = varchar(32) }
  column "user_id"     { type = varchar(32) }
  column "objective"   { type = varchar(255) }
  column "key_results" { type = jsonb }
  column "quarter"     { type = varchar(16) }
  column "status"      { type = varchar(16), default = "active" }
  column "created_at"  { type = timestamptz }
  column "updated_at"  { type = timestamptz }
  primary_key { columns = [column.id] }
  foreign_key "fk_okrs_tenant" {
    columns     = [column.tenant_id]
    ref_columns = [table.tenants.column.id]
    on_delete   = CASCADE
  }
  index "idx_okrs_tenant" { columns = [column.tenant_id] }
}

table "meeting_notes" {
  schema = schema.public
  column "id"           { type = varchar(32) }
  column "tenant_id"    { type = varchar(32) }
  column "user_id"      { type = varchar(32) }
  column "title"        { type = varchar(255) }
  column "notes"        { type = text }
  column "summary"      { type = text }
  column "participants" { type = sql("text[]") }
  column "date"         { type = date }
  column "created_at"   { type = timestamptz }
  primary_key { columns = [column.id] }
  foreign_key "fk_meeting_notes_tenant" {
    columns     = [column.tenant_id]
    ref_columns = [table.tenants.column.id]
    on_delete   = CASCADE
  }
  index "idx_meeting_notes_tenant" { columns = [column.tenant_id] }
}

table "support_tickets" {
  schema = schema.public
  column "id"          { type = varchar(32) }
  column "tenant_id"   { type = varchar(32) }
  column "user_id"     { type = varchar(32) }
  column "subject"     { type = varchar(255) }
  column "description" { type = text }
  column "priority"    { type = varchar(16), default = "medium" }
  column "status"      { type = varchar(16), default = "open" }
  column "assignee"    { type = varchar(128) }
  column "created_at"  { type = timestamptz }
  column "updated_at"  { type = timestamptz }
  primary_key { columns = [column.id] }
  foreign_key "fk_support_tickets_tenant" {
    columns     = [column.tenant_id]
    ref_columns = [table.tenants.column.id]
    on_delete   = CASCADE
  }
  index "idx_support_tickets_tenant" { columns = [column.tenant_id] }
}

table "kb_articles" {
  schema = schema.public
  column "id"         { type = varchar(32) }
  column "tenant_id"  { type = varchar(32) }
  column "user_id"    { type = varchar(32) }
  column "title"      { type = varchar(255) }
  column "content"    { type = text }
  column "tags"       { type = sql("text[]") }
  column "category"   { type = varchar(64) }
  column "created_at" { type = timestamptz }
  column "updated_at" { type = timestamptz }
  primary_key { columns = [column.id] }
  foreign_key "fk_kb_articles_tenant" {
    columns     = [column.tenant_id]
    ref_columns = [table.tenants.column.id]
    on_delete   = CASCADE
  }
  index "idx_kb_articles_tenant" { columns = [column.tenant_id] }
}

table "marketing_campaigns" {
  schema = schema.public
  column "id"            { type = varchar(32) }
  column "tenant_id"     { type = varchar(32) }
  column "user_id"       { type = varchar(32) }
  column "name"          { type = varchar(255) }
  column "description"   { type = text }
  column "campaign_type" { type = varchar(32), default = "email" }
  column "config"        { type = jsonb }
  column "status"        { type = varchar(16), default = "draft" }
  column "created_at"    { type = timestamptz }
  column "updated_at"    { type = timestamptz }
  primary_key { columns = [column.id] }
  foreign_key "fk_marketing_campaigns_tenant" {
    columns     = [column.tenant_id]
    ref_columns = [table.tenants.column.id]
    on_delete   = CASCADE
  }
  index "idx_marketing_campaigns_tenant" { columns = [column.tenant_id] }
}

// ── Audit Logs ──

table "audit_logs" {
  schema = schema.public
  column "id"            { type = varchar(32) }
  column "tenant_id"     { type = varchar(32) }
  column "user_id"       { type = varchar(32), null = true }
  column "action"        { type = varchar(64) }
  column "resource_type" { type = varchar(64) }
  column "resource_id"   { type = varchar(64), null = true }
  column "details"       { type = jsonb }
  column "ip_address"    { type = varchar(45), null = true }
  column "created_at"    { type = timestamptz }
  primary_key { columns = [column.id] }
  foreign_key "fk_audit_logs_tenant" {
    columns     = [column.tenant_id]
    ref_columns = [table.tenants.column.id]
    on_delete   = CASCADE
  }
  foreign_key "fk_audit_logs_user" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = SET_NULL
  }
  index "idx_audit_logs_tenant"  { columns = [column.tenant_id] }
  index "idx_audit_logs_user"    { columns = [column.user_id] }
  index "idx_audit_logs_action"  { columns = [column.action] }
  index "idx_audit_logs_created" { columns = [column.created_at] }
}
