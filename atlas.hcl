env "local" {
  url = "postgres://minicc:minicc@localhost:5432/minicc?sslmode=disable"
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
  comment = "MiniCC V2 schema"
}

// ── Users ──

table "users" {
  schema = schema.public
  column "id"      { type = varchar(32) }
  column "email"   { type = varchar(255) }
  column "name"    { type = varchar(128) }
  column "password_hash" { type = varchar(255) }
  column "role"    { type = varchar(16), default = "user" }
  column "created_at" { type = timestamptz }
  column "updated_at" { type = timestamptz }

  primary_key { columns = [column.id] }
  unique "uniq_users_email" { columns = [column.email] }
  index "idx_users_email"   { columns = [column.email] }
}

// ── Sessions ──

table "sessions" {
  schema = schema.public
  column "id"         { type = varchar(32) }
  column "user_id"    { type = varchar(32) }
  column "title"      { type = varchar(255), default = "" }
  column "created_at" { type = timestamptz }
  column "updated_at" { type = timestamptz }

  primary_key { columns = [column.id] }
  foreign_key "fk_sessions_user" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index "idx_sessions_user"   { columns = [column.user_id] }
  index "idx_sessions_updated" { columns = [column.updated_at] }
}

// ── Messages ──

table "messages" {
  schema = schema.public
  column "id"         { type = varchar(32) }
  column "session_id" { type = varchar(32) }
  column "role"       { type = varchar(16) }
  column "content"    { type = text }
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
  column "duration_ms" { type = bigint, default = 0 }
  column "created_at" { type = timestamptz }

  primary_key { columns = [column.id] }
  foreign_key "fk_toolcalls_session" {
    columns     = [column.session_id]
    ref_columns = [table.sessions.column.id]
    on_delete   = CASCADE
  }
  foreign_key "fk_toolcalls_message" {
    columns     = [column.message_id]
    ref_columns = [table.messages.column.id]
    on_delete   = SET_NULL
  }
  index "idx_toolcalls_session" { columns = [column.session_id] }
}

// ── Tasks ──

table "tasks" {
  schema = schema.public
  column "id"          { type = varchar(32) }
  column "user_id"     { type = varchar(32) }
  column "type"        { type = varchar(32) }
  column "status"      { type = varchar(16), default = "pending" }
  column "priority"    { type = int, default = 0 }
  column "payload"     { type = jsonb }
  column "result"      { type = jsonb, null = true }
  column "error"       { type = text, null = true }
  column "retries"     { type = int, default = 0 }
  column "max_retries" { type = int, default = 3 }
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

// ── API Keys ──

table "api_keys" {
  schema = schema.public
  column "id"          { type = varchar(32) }
  column "user_id"     { type = varchar(32) }
  column "name"        { type = varchar(128) }
  column "key_hash"    { type = varchar(64) }
  column "last_used_at" { type = timestamptz, null = true }
  column "expires_at"  { type = timestamptz, null = true }
  column "created_at"  { type = timestamptz }

  primary_key { columns = [column.id] }
  foreign_key "fk_apikeys_user" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = CASCADE
  }
  index  "idx_apikeys_user" { columns = [column.user_id] }
  unique "uniq_apikeys_hash" { columns = [column.key_hash] }
}

// ── Audit Logs ──

table "audit_logs" {
  schema = schema.public
  column "id"            { type = varchar(32) }
  column "user_id"       { type = varchar(32), null = true }
  column "action"        { type = varchar(64) }
  column "resource_type" { type = varchar(64) }
  column "resource_id"   { type = varchar(32), null = true }
  column "details"       { type = jsonb }
  column "ip_address"    { type = varchar(45), null = true }
  column "created_at"    { type = timestamptz }

  primary_key { columns = [column.id] }
  foreign_key "fk_audit_user" {
    columns     = [column.user_id]
    ref_columns = [table.users.column.id]
    on_delete   = SET_NULL
  }
  index "idx_audit_user"    { columns = [column.user_id] }
  index "idx_audit_action"  { columns = [column.action] }
  index "idx_audit_created" { columns = [column.created_at] }
}

// ── Schema Migrations (Atlas internal) ──

table "schema_migrations" {
  schema = schema.public
  column "version"   { type = bigint }
  column "applied_at" { type = timestamptz }

  primary_key { columns = [column.version] }
}
