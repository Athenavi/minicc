create table if not exists schema_migrations
(
    version    bigint                                 not null
        primary key,
    name       varchar(255)                           not null,
    checksum   varchar(128),
    applied_at timestamp with time zone default now() not null
);

create table if not exists agent_registry
(
    agent_type  varchar(32)                               not null
        primary key,
    name        varchar(128)                              not null,
    description text                     default ''::text not null,
    enabled     boolean                  default true     not null,
    config      jsonb                    default '{}'::jsonb,
    created_at  timestamp with time zone default now()    not null
);

create table if not exists guest_storage
(
    client_id  varchar(64)                            not null
        primary key,
    storage_id varchar(64)                            not null
        unique,
    created_at timestamp with time zone default now() not null
);

create index if not exists idx_guest_storage_id
    on guest_storage (storage_id);

create table if not exists stripe_payments
(
    session_id   varchar(128)                                                  not null
        primary key,
    user_id      uuid                                                          not null,
    credits      integer                  default 1000                         not null,
    amount_cents bigint                   default 0                            not null,
    status       varchar(16)              default 'pending'::character varying not null,
    created_at   timestamp with time zone default now()                        not null,
    completed_at timestamp with time zone
);

create index if not exists idx_stripe_payments_user
    on stripe_payments (user_id);

create table if not exists tenants
(
    id         uuid                     default gen_random_uuid()           not null
        primary key,
    name       varchar(255)                                                 not null,
    created_at timestamp with time zone default now()                       not null,
    status     varchar(16)              default 'active'::character varying not null
);

create table if not exists agents
(
    id              uuid                     default gen_random_uuid()            not null
        primary key,
    tenant_id       uuid                                                          not null
        references tenants
            on delete cascade,
    name            varchar(255)                                                  not null,
    description     text,
    system_prompt   text,
    tools           jsonb                    default '[]'::jsonb,
    llm_config      jsonb                    default '{}'::jsonb,
    max_turns       integer                  default 10,
    timeout_seconds integer                  default 120,
    enabled         boolean                  default true                         not null,
    created_at      timestamp with time zone default now()                        not null,
    updated_at      timestamp with time zone default now()                        not null,
    user_id         uuid                                                          not null,
    visibility      varchar(16)              default 'private'::character varying not null
);

create index if not exists idx_agents_name
    on agents (tenant_id, name);

create index if not exists idx_agents_tenant
    on agents (tenant_id);

create index if not exists idx_agents_tenant_user
    on agents (tenant_id, user_id);

create table if not exists enterprise_tasks
(
    id          uuid                     default gen_random_uuid()           not null
        primary key,
    tenant_id   uuid                                                         not null
        references tenants
            on delete cascade,
    user_id     varchar(32)              default ''::character varying       not null,
    title       varchar(255)                                                 not null,
    description text                     default ''::text,
    project     varchar(128)             default ''::character varying,
    assignee    varchar(128)             default ''::character varying,
    priority    varchar(16)              default 'medium'::character varying not null,
    status      varchar(16)              default 'open'::character varying   not null,
    created_at  timestamp with time zone default now()                       not null,
    updated_at  timestamp with time zone default now()                       not null
);

create index if not exists idx_enterprise_tasks_tenant
    on enterprise_tasks (tenant_id);

create table if not exists kb_articles
(
    id         uuid                     default gen_random_uuid()     not null
        primary key,
    tenant_id  uuid                                                   not null
        references tenants
            on delete cascade,
    user_id    varchar(32)              default ''::character varying not null,
    title      varchar(255)                                           not null,
    content    text                     default ''::text              not null,
    tags       text[]                   default '{}'::text[],
    category   varchar(64)              default ''::character varying,
    created_at timestamp with time zone default now()                 not null,
    updated_at timestamp with time zone default now()                 not null
);

create index if not exists idx_kb_articles_tenant
    on kb_articles (tenant_id);

create table if not exists marketing_campaigns
(
    id            uuid                     default gen_random_uuid()          not null
        primary key,
    tenant_id     uuid                                                        not null
        references tenants
            on delete cascade,
    user_id       varchar(32)              default ''::character varying      not null,
    name          varchar(255)                                                not null,
    description   text                     default ''::text,
    campaign_type varchar(32)              default 'email'::character varying not null,
    config        jsonb                    default '{}'::jsonb,
    status        varchar(16)              default 'draft'::character varying not null,
    created_at    timestamp with time zone default now()                      not null,
    updated_at    timestamp with time zone default now()                      not null
);

create index if not exists idx_marketing_campaigns_tenant
    on marketing_campaigns (tenant_id);

create table if not exists media_assets
(
    id         uuid                     default gen_random_uuid()         not null
        primary key,
    tenant_id  uuid                                                       not null
        references tenants
            on delete cascade,
    user_id    uuid                                                       not null,
    type       varchar(16)              default 'text'::character varying not null,
    name       varchar(255)                                               not null,
    file_url   varchar(1024)            default ''::character varying,
    file_path  varchar(512)             default ''::character varying,
    mime_type  varchar(64)              default ''::character varying,
    thumbnail  varchar(512)             default ''::character varying,
    metadata   jsonb                    default '{}'::jsonb,
    tags       text[]                   default '{}'::text[],
    category   varchar(64)              default ''::character varying,
    size       bigint                   default 0                         not null,
    created_at timestamp with time zone default now()                     not null,
    updated_at timestamp with time zone default now()                     not null,
    parent_id  varchar(64)              default ''::character varying
);

create index if not exists idx_media_category
    on media_assets (category);

create index if not exists idx_media_created
    on media_assets (created_at);

create index if not exists idx_media_parent
    on media_assets (parent_id);

create index if not exists idx_media_tenant
    on media_assets (tenant_id);

create index if not exists idx_media_type
    on media_assets (type);

create index if not exists idx_media_user
    on media_assets (user_id);


create table if not exists meeting_notes
(
    id           uuid                     default gen_random_uuid()     not null
        primary key,
    tenant_id    uuid                                                   not null
        references tenants
            on delete cascade,
    user_id      varchar(32)              default ''::character varying not null,
    title        varchar(255)                                           not null,
    notes        text                     default ''::text              not null,
    summary      text                     default ''::text,
    participants text[]                   default '{}'::text[],
    date         date                     default CURRENT_DATE,
    created_at   timestamp with time zone default now()                 not null
);

create index if not exists idx_meeting_notes_tenant
    on meeting_notes (tenant_id);

create table if not exists okrs
(
    id          uuid                     default gen_random_uuid()           not null
        primary key,
    tenant_id   uuid                                                         not null
        references tenants
            on delete cascade,
    user_id     varchar(32)              default ''::character varying       not null,
    objective   varchar(255)                                                 not null,
    key_results jsonb                    default '[]'::jsonb,
    quarter     varchar(16)              default ''::character varying,
    status      varchar(16)              default 'active'::character varying not null,
    created_at  timestamp with time zone default now()                       not null,
    updated_at  timestamp with time zone default now()                       not null
);

create index if not exists idx_okrs_tenant
    on okrs (tenant_id);

create table if not exists support_tickets
(
    id          uuid                     default gen_random_uuid()           not null
        primary key,
    tenant_id   uuid                                                         not null
        references tenants
            on delete cascade,
    user_id     varchar(32)              default ''::character varying       not null,
    subject     varchar(255)                                                 not null,
    description text                     default ''::text,
    priority    varchar(16)              default 'medium'::character varying not null,
    status      varchar(16)              default 'open'::character varying   not null,
    assignee    varchar(128)             default ''::character varying,
    created_at  timestamp with time zone default now()                       not null,
    updated_at  timestamp with time zone default now()                       not null
);

create index if not exists idx_support_tickets_tenant
    on support_tickets (tenant_id);

create table if not exists tool_calls
(
    id          text                     default (gen_random_uuid())::text not null
        primary key,
    session_id  uuid                                                       not null,
    message_id  uuid,
    tool_name   varchar(128)                                               not null,
    input       jsonb                    default '{}'::jsonb               not null,
    output      text                     default ''::text                  not null,
    is_error    boolean                  default false                     not null,
    duration_ms bigint                   default 0                         not null,
    created_at  timestamp with time zone default now()                     not null
);

create index if not exists idx_tool_calls_session
    on tool_calls (session_id);

create index if not exists idx_tool_calls_session_created
    on tool_calls (session_id, created_at);

create index if not exists idx_tool_calls_created
    on tool_calls (created_at);

create table if not exists users
(
    id            uuid                     default gen_random_uuid()         not null
        primary key,
    tenant_id     uuid                                                       not null
        references tenants
            on delete cascade,
    email         varchar(255)                                               not null,
    name          varchar(128)                                               not null,
    password_hash varchar(255)                                               not null,
    role          varchar(16)              default 'user'::character varying not null,
    storage_id    varchar(64)
        unique,
    credits       integer                  default 1000                      not null,
    created_at    timestamp with time zone default now()                     not null,
    updated_at    timestamp with time zone default now()                     not null,
    phone         varchar(32),
    password_set  boolean                  default true                      not null,
    settings      jsonb                    default '{}'::jsonb               not null,
    unique (tenant_id, email)
);

create index if not exists idx_users_email
    on users (email);

create index if not exists idx_users_tenant
    on users (tenant_id);

create unique index if not exists uq_users_tenant_phone
    on users (tenant_id, phone)
    where (phone IS NOT NULL);

create table if not exists agent_sessions
(
    id         varchar(128)                                                  not null
        primary key,
    user_id    uuid                                                          not null
        references users
            on delete cascade,
    agent_id   uuid
                                                                             references agents
                                                                                 on delete set null,
    name       varchar(128)                                                  not null,
    task       text                                                          not null,
    status     varchar(16)              default 'pending'::character varying not null,
    result     text,
    created_at timestamp with time zone default now()                        not null,
    updated_at timestamp with time zone default now()                        not null,
    tenant_id  uuid                                                          not null
        constraint fk_agent_sessions_tenant
            references tenants
            on delete cascade
);

create index if not exists idx_agent_sessions_status
    on agent_sessions (status);

create index if not exists idx_agent_sessions_user
    on agent_sessions (user_id);

create index if not exists idx_agent_sessions_tenant
    on agent_sessions (tenant_id);

create index if not exists idx_agent_sessions_tenant_user
    on agent_sessions (tenant_id, user_id);

create table if not exists api_keys
(
    id           uuid                     default gen_random_uuid() not null
        primary key,
    user_id      uuid                                               not null
        references users
            on delete cascade,
    name         varchar(128)                                       not null,
    key_hash     varchar(64)                                        not null,
    last_used_at timestamp with time zone,
    expires_at   timestamp with time zone,
    created_at   timestamp with time zone default now()             not null,
    revoked      boolean                  default false             not null
);

create index if not exists idx_api_keys_user
    on api_keys (user_id);

create index if not exists idx_api_keys_key_hash
    on api_keys (key_hash);

create table if not exists audit_logs
(
    id            uuid                     default gen_random_uuid() not null
        primary key,
    tenant_id     uuid                                               not null
        references tenants
            on delete cascade,
    user_id       uuid
                                                                     references users
                                                                         on delete set null,
    action        varchar(64)                                        not null,
    resource_type varchar(64)                                        not null,
    resource_id   varchar(64),
    details       jsonb                    default '{}'::jsonb,
    ip_address    varchar(45),
    created_at    timestamp with time zone default now()             not null
);

create index if not exists idx_audit_action
    on audit_logs (action);

create index if not exists idx_audit_created
    on audit_logs (created_at);

create index if not exists idx_audit_tenant
    on audit_logs (tenant_id);

create index if not exists idx_audit_user
    on audit_logs (user_id);

create index if not exists idx_audit_logs_tenant_time
    on audit_logs (tenant_id asc, created_at desc);

create index if not exists idx_audit_logs_user_time
    on audit_logs (user_id, created_at);

create table if not exists credit_transactions
(
    id         uuid                     default gen_random_uuid() not null
        primary key,
    user_id    uuid                                               not null
        references users
            on delete cascade,
    amount     integer                                            not null,
    balance    integer                                            not null,
    reason     varchar(64)                                        not null,
    created_at timestamp with time zone default now()             not null
);

create index if not exists idx_credit_tx_user
    on credit_transactions (user_id asc, created_at desc);

create table if not exists knowledge_bases
(
    id               uuid                     default gen_random_uuid()            not null
        primary key,
    tenant_id        uuid                                                          not null
        references tenants
            on delete cascade,
    user_id          uuid                                                          not null
        references users
            on delete cascade,
    name             varchar(255)                                                  not null,
    description      text,
    type             varchar(32)              default 'rag'::character varying     not null,
    visibility       varchar(32)              default 'private'::character varying not null,
    status           varchar(32)              default 'active'::character varying  not null,
    document_count   integer                  default 0,
    total_size_bytes bigint                   default 0,
    credits_consumed integer                  default 0,
    config           jsonb                    default '{}'::jsonb,
    created_at       timestamp with time zone default now(),
    updated_at       timestamp with time zone default now(),
    doc_count        integer                  default 0
);

create index if not exists idx_knowledge_bases_status
    on knowledge_bases (status);

create index if not exists idx_knowledge_bases_tenant
    on knowledge_bases (tenant_id);

create index if not exists idx_knowledge_bases_user
    on knowledge_bases (user_id);

create index if not exists idx_knowledge_bases_visibility
    on knowledge_bases (visibility);

create trigger update_knowledge_bases_updated_at
    before update
    on knowledge_bases
    for each row
execute procedure update_updated_at_column();

create table if not exists knowledge_documents
(
    id                uuid                     default gen_random_uuid()            not null
        primary key,
    knowledge_base_id uuid                                                          not null
        references knowledge_bases
            on delete cascade,
    tenant_id         uuid                                                          not null
        references tenants
            on delete cascade,
    user_id           uuid                                                          not null
        references users
            on delete cascade,
    name              varchar(255)                                                  not null,
    file_url          varchar(1024),
    file_type         varchar(32),
    file_size_bytes   bigint                   default 0,
    chunk_count       integer                  default 0,
    status            varchar(32)              default 'pending'::character varying not null,
    error_message     text,
    metadata          jsonb                    default '{}'::jsonb,
    created_at        timestamp with time zone default now(),
    updated_at        timestamp with time zone default now(),
    content           bytea
);

create index if not exists idx_knowledge_documents_kb
    on knowledge_documents (knowledge_base_id);

create index if not exists idx_knowledge_documents_status
    on knowledge_documents (status);

create index if not exists idx_knowledge_documents_tenant
    on knowledge_documents (tenant_id);

create trigger update_knowledge_documents_updated_at
    before update
    on knowledge_documents
    for each row
execute procedure update_updated_at_column();

create table if not exists knowledge_chunks
(
    id                uuid                     default gen_random_uuid() not null
        primary key,
    document_id       uuid                                               not null
        references knowledge_documents
            on delete cascade,
    knowledge_base_id uuid                                               not null
        references knowledge_bases
            on delete cascade,
    tenant_id         uuid                                               not null
        references tenants
            on delete cascade,
    chunk_index       integer                                            not null,
    content           text                                               not null,
    metadata          jsonb                    default '{}'::jsonb,
    search_vector     tsvector,
    created_at        timestamp with time zone default now()
);

create index if not exists idx_knowledge_chunks_doc
    on knowledge_chunks (document_id);

create index if not exists idx_knowledge_chunks_kb
    on knowledge_chunks (knowledge_base_id);

create index if not exists idx_knowledge_chunks_search
    on knowledge_chunks using gin (search_vector);

create index if not exists idx_knowledge_chunks_tenant
    on knowledge_chunks (tenant_id);

create trigger knowledge_chunks_search_vector_trigger
    before insert or update
    on knowledge_chunks
    for each row
execute procedure knowledge_chunks_search_vector_update();

create table if not exists sessions
(
    id         uuid                     default gen_random_uuid()           not null
        primary key,
    tenant_id  uuid                                                         not null
        references tenants
            on delete cascade,
    user_id    uuid
                                                                            references users
                                                                                on delete set null,
    agent_id   uuid
                                                                            references agents
                                                                                on delete set null,
    title      varchar(255)             default ''::character varying       not null,
    status     varchar(16)              default 'active'::character varying not null,
    created_at timestamp with time zone default now()                       not null,
    updated_at timestamp with time zone default now()                       not null,
    pinned     boolean                  default false                       not null
);

create index if not exists idx_sessions_agent
    on sessions (agent_id);

create index if not exists idx_sessions_tenant
    on sessions (tenant_id);

create index if not exists idx_sessions_updated
    on sessions (updated_at);

create index if not exists idx_sessions_user
    on sessions (user_id);

create index if not exists idx_sessions_user_updated
    on sessions (user_id asc, updated_at desc);

create table if not exists billing_records
(
    id            uuid                     default gen_random_uuid() not null
        primary key,
    tenant_id     uuid                                               not null
        references tenants
            on delete cascade,
    user_id       uuid                                               not null
        references users
            on delete cascade,
    session_id    uuid
                                                                     references sessions
                                                                         on delete set null,
    input_tokens  bigint                   default 0                 not null,
    output_tokens bigint                   default 0                 not null,
    cost_cents    integer                  default 0                 not null,
    created_at    timestamp with time zone default now()             not null,
    group_id      uuid
);

create index if not exists idx_billing_created
    on billing_records (created_at);

create index if not exists idx_billing_session
    on billing_records (session_id);

create index if not exists idx_billing_tenant
    on billing_records (tenant_id);

create index if not exists idx_billing_user
    on billing_records (user_id);

create index if not exists idx_billing_records_group
    on billing_records (group_id);

create table if not exists messages
(
    id         uuid                     default gen_random_uuid() not null
        primary key,
    session_id uuid                                               not null
        references sessions
            on delete cascade,
    role       varchar(16)                                        not null,
    content    text                     default ''::text          not null,
    tool_calls jsonb,
    created_at timestamp with time zone default now()             not null
);

create index if not exists idx_messages_session
    on messages (session_id, created_at);


create table if not exists tasks
(
    id          uuid                     default gen_random_uuid()            not null
        primary key,
    user_id     uuid                                                          not null
        references users
            on delete cascade,
    type        varchar(32)                                                   not null,
    status      varchar(16)              default 'pending'::character varying not null,
    priority    integer                  default 0                            not null,
    payload     jsonb                    default '{}'::jsonb                  not null,
    result      jsonb,
    error       text,
    retries     integer                  default 0                            not null,
    max_retries integer                  default 3                            not null,
    created_at  timestamp with time zone default now()                        not null,
    updated_at  timestamp with time zone default now()                        not null
);

create index if not exists idx_tasks_status
    on tasks (status);

create index if not exists idx_tasks_type
    on tasks (type);

create index if not exists idx_tasks_user
    on tasks (user_id);

create table if not exists wiki_pages
(
    id         uuid                     default gen_random_uuid()     not null
        primary key,
    tenant_id  uuid                                                   not null
        references tenants
            on delete cascade,
    user_id    varchar(32)              default ''::character varying not null,
    title      varchar(255)                                           not null,
    content    text                     default ''::text              not null,
    tags       text[]                   default '{}'::text[],
    created_at timestamp with time zone default now()                 not null,
    updated_at timestamp with time zone default now()                 not null
);

create index if not exists idx_wiki_pages_tenant
    on wiki_pages (tenant_id);

create table if not exists workflow_graphs
(
    id         varchar(32)                                  not null
        primary key,
    name       varchar(255)                                 not null,
    user_id    uuid
                                                            references users
                                                                on delete set null,
    graph_json jsonb                    default '{}'::jsonb not null,
    created_at timestamp with time zone default now()       not null,
    updated_at timestamp with time zone default now()       not null
);

create index if not exists idx_workflow_graphs_updated
    on workflow_graphs (updated_at);

create index if not exists idx_workflow_graphs_user
    on workflow_graphs (user_id);

create table if not exists conversation_shares
(
    id          varchar(32)                                            not null
        primary key,
    session_id  varchar(128)                                           not null,
    user_id     uuid                                                   not null,
    title       varchar(255)             default ''::character varying not null,
    message_ids text[]                   default '{}'::text[]          not null,
    created_at  timestamp with time zone default now()                 not null,
    revoked_at  timestamp with time zone
);

create index if not exists idx_conversation_shares_session
    on conversation_shares (session_id)
    where (revoked_at IS NULL);

create table if not exists workflow_instances
(
    id            varchar(64)                                                   not null
        primary key,
    user_id       varchar(64)              default ''::character varying        not null,
    workflow_id   varchar(64)              default ''::character varying        not null,
    workflow_name varchar(255)             default ''::character varying        not null,
    status        varchar(16)              default 'running'::character varying not null,
    results       jsonb                    default '{}'::jsonb                  not null,
    error         text,
    created_at    timestamp with time zone default now()                        not null,
    updated_at    timestamp with time zone default now()                        not null
);

create index if not exists idx_workflow_instances_user
    on workflow_instances (user_id asc, created_at desc);

create table if not exists uploads
(
    id              varchar(64)                                                   not null
        primary key,
    user_id         varchar(64)              default ''::character varying        not null,
    name            varchar(255)                                                  not null,
    size            bigint                   default 0                            not null,
    mime_type       varchar(64)              default ''::character varying        not null,
    purpose         varchar(16)              default 'generic'::character varying not null,
    parent_id       varchar(64)              default ''::character varying        not null,
    category        varchar(64)              default ''::character varying        not null,
    chunk_size      integer                  default 2097152                      not null,
    chunk_count     integer                  default 0                            not null,
    chunks_received text[]                   default '{}'::text[]                 not null,
    status          varchar(16)              default 'pending'::character varying not null,
    created_at      timestamp with time zone default now()                        not null,
    updated_at      timestamp with time zone default now()                        not null,
    tenant_id       uuid                                                          not null
        constraint fk_uploads_tenant
            references tenants
            on delete cascade
);

create index if not exists idx_uploads_user
    on uploads (user_id asc, created_at desc);

create index if not exists idx_uploads_tenant
    on uploads (tenant_id);

create index if not exists idx_uploads_tenant_user
    on uploads (tenant_id, user_id);

create table if not exists admin_api_keys
(
    id             uuid                     default gen_random_uuid() not null
        primary key,
    key_hash       varchar(64)                                        not null
        unique,
    name           varchar(100)                                       not null,
    tenant_id      varchar(50)                                        not null,
    user_id        varchar(50),
    monthly_quota  integer                  default 0
        constraint chk_quota
            check (monthly_quota >= 0),
    used_count     bigint                   default 0,
    used_credits   bigint                   default 0,
    status         varchar(20)              default 'active'::character varying
        constraint chk_status
            check ((status)::text = ANY
                   (ARRAY [('active'::character varying)::text, ('expired'::character varying)::text, ('suspended'::character varying)::text])),
    expires_at     timestamp with time zone,
    created_at     timestamp with time zone default now(),
    updated_at     timestamp with time zone default now(),
    created_by     varchar(50),
    description    text,
    allowed_models text[],
    rate_limit_qps integer                  default 10
);

comment on table admin_api_keys is 'API Key management with quota and lifecycle control';

create index if not exists idx_api_keys_hash
    on admin_api_keys (key_hash);

create index if not exists idx_api_keys_tenant_status
    on admin_api_keys (tenant_id, status);

create index if not exists idx_api_keys_expires
    on admin_api_keys (expires_at)
    where ((status)::text = 'active'::text);

create index if not exists idx_api_keys_created
    on admin_api_keys (created_at desc);

create table if not exists admin_model_configs
(
    id                 uuid                     default gen_random_uuid() not null
        primary key,
    model_id           varchar(50)                                        not null
        unique,
    display_name       varchar(100)                                       not null,
    provider           varchar(50)                                        not null,
    priority           integer                  default 0,
    weight             integer                  default 100
        constraint chk_weight
            check ((weight >= 1) AND (weight <= 100)),
    fallback_chain     text[],
    max_rpm            integer                  default 1000,
    max_tpm            integer                  default 500000,
    concurrent_limit   integer                  default 50,
    status             varchar(20)              default 'active'::character varying
        constraint chk_model_status
            check ((status)::text = ANY
                   (ARRAY [('active'::character varying)::text, ('deprecated'::character varying)::text, ('maintenance'::character varying)::text])),
    is_default         boolean                  default false,
    input_cost_per_1m  double precision         default 0,
    output_cost_per_1m double precision         default 0,
    config_json        jsonb                    default '{}'::jsonb,
    created_at         timestamp with time zone default now(),
    updated_at         timestamp with time zone default now()
);

comment on table admin_model_configs is 'Model configuration with routing strategies';

create index if not exists idx_model_configs_status
    on admin_model_configs (status);

create index if not exists idx_model_configs_provider
    on admin_model_configs (provider);

create table if not exists admin_workflows
(
    id                      uuid                     default gen_random_uuid() not null
        primary key,
    workflow_id             varchar(50)                                        not null
        unique,
    name                    varchar(100)                                       not null,
    description             text,
    nodes                   jsonb                                              not null,
    edges                   jsonb                                              not null,
    error_handling_strategy varchar(20)              default 'fail_fast'::character varying
        constraint chk_error_strategy
            check ((error_handling_strategy)::text = ANY
                   (ARRAY [('fail_fast'::character varying)::text, ('continue'::character varying)::text, ('skip'::character varying)::text])),
    timeout_ms              integer                  default 30000,
    max_retries             integer                  default 3,
    version                 integer                  default 1,
    published_version       integer                  default 0,
    status                  varchar(20)              default 'draft'::character varying
        constraint chk_workflow_status
            check ((status)::text = ANY
                   (ARRAY [('draft'::character varying)::text, ('testing'::character varying)::text, ('published'::character varying)::text, ('archived'::character varying)::text])),
    created_by              varchar(50),
    created_at              timestamp with time zone default now(),
    updated_at              timestamp with time zone default now(),
    published_at            timestamp with time zone
);

comment on table admin_workflows is 'Workflow DAG orchestration with version control';

create index if not exists idx_workflows_status
    on admin_workflows (status);

create index if not exists idx_workflows_created
    on admin_workflows (created_at desc);

create table if not exists admin_workflow_executions
(
    id               uuid                     default gen_random_uuid() not null
        primary key,
    workflow_id      varchar(50)                                        not null,
    workflow_version integer                                            not null,
    status           varchar(20)              default 'running'::character varying
        constraint chk_execution_status
            check ((status)::text = ANY
                   (ARRAY [('running'::character varying)::text, ('completed'::character varying)::text, ('failed'::character varying)::text, ('cancelled'::character varying)::text])),
    started_at       timestamp with time zone default now(),
    completed_at     timestamp with time zone,
    duration_ms      integer,
    input_data       jsonb,
    output_data      jsonb,
    error_message    text,
    triggered_by     varchar(50),
    node_results     jsonb                    default '[]'::jsonb
);

comment on table admin_workflow_executions is 'Workflow execution history with node-level results';

create index if not exists idx_workflow_executions_workflow
    on admin_workflow_executions (workflow_id, workflow_version);

create index if not exists idx_workflow_executions_status
    on admin_workflow_executions (status);

create index if not exists idx_workflow_executions_started
    on admin_workflow_executions (started_at desc);

create table if not exists admin_api_call_logs
(
    id                  uuid                     default gen_random_uuid() not null
        primary key,
    api_key_id          uuid
        references admin_api_keys,
    model_id            varchar(50),
    workflow_id         varchar(50),
    endpoint            varchar(100)                                       not null,
    method              varchar(10)              default 'POST'::character varying,
    request_size_bytes  integer,
    response_size_bytes integer,
    duration_ms         integer,
    status_code         integer,
    retry_count         integer                  default 0,
    input_tokens        integer                  default 0,
    output_tokens       integer                  default 0,
    credits_consumed    bigint                   default 0,
    created_at          timestamp with time zone default now()
);

comment on table admin_api_call_logs is 'API call audit log with performance and cost metrics';

create index if not exists idx_api_logs_key
    on admin_api_call_logs (api_key_id)
    where (api_key_id IS NOT NULL);

create index if not exists idx_api_logs_model
    on admin_api_call_logs (model_id)
    where (model_id IS NOT NULL);

create index if not exists idx_api_logs_workflow
    on admin_api_call_logs (workflow_id)
    where (workflow_id IS NOT NULL);

create index if not exists idx_api_logs_created
    on admin_api_call_logs (created_at desc);

create index if not exists idx_api_logs_date
    on admin_api_call_logs (((created_at AT TIME ZONE 'UTC'::text)::date));

create table if not exists admin_cron_jobs
(
    id              uuid                     default gen_random_uuid() not null
        primary key,
    job_id          varchar(50)                                        not null
        unique,
    name            varchar(100)                                       not null,
    schedule        varchar(50)                                        not null,
    last_run_at     timestamp with time zone,
    last_run_status varchar(20),
    last_error      text,
    next_run_at     timestamp with time zone,
    enabled         boolean                  default true,
    metadata        jsonb                    default '{}'::jsonb,
    created_at      timestamp with time zone default now(),
    updated_at      timestamp with time zone default now()
);

comment on table admin_cron_jobs is 'Cron job scheduler registry';

create unique index if not exists idx_cron_jobs_id
    on admin_cron_jobs (job_id);

create table if not exists admin_tenants
(
    id                      uuid                     default gen_random_uuid() not null
        primary key,
    tenant_id               varchar(50)                                        not null
        unique,
    name                    varchar(100)                                       not null,
    company_name            varchar(200),
    contact_email           varchar(100),
    contact_phone           varchar(20),
    max_api_keys            integer                  default 10,
    max_models              integer                  default 5,
    monthly_quota           bigint                   default 0
        constraint chk_quota
            check (monthly_quota >= 0),
    max_concurrent_sessions integer                  default 10,
    status                  varchar(20)              default 'active'::character varying
        constraint chk_tenant_status
            check ((status)::text = ANY
                   (ARRAY [('active'::character varying)::text, ('suspended'::character varying)::text, ('expired'::character varying)::text])),
    expires_at              timestamp with time zone,
    created_at              timestamp with time zone default now(),
    updated_at              timestamp with time zone default now(),
    created_by              varchar(50),
    features                jsonb                    default '{}'::jsonb
);

comment on table admin_tenants is 'Multi-tenant management with quota control';

create index if not exists idx_tenants_status
    on admin_tenants (status);

create index if not exists idx_tenants_created
    on admin_tenants (created_at desc);

create table if not exists admin_tenant_usage
(
    id               uuid                     default gen_random_uuid() not null
        primary key,
    tenant_id        varchar(50)                                        not null
        references admin_tenants (tenant_id),
    stat_date        date                                               not null,
    api_calls        bigint                   default 0,
    tokens_used      bigint                   default 0,
    credits_consumed bigint                   default 0,
    storage_mb       double precision         default 0,
    created_at       timestamp with time zone default now(),
    unique (tenant_id, stat_date)
);

comment on table admin_tenant_usage is 'Daily usage statistics per tenant';

create index if not exists idx_tenant_usage_date
    on admin_tenant_usage (stat_date desc);

create table if not exists admin_domains
(
    id             uuid                     default gen_random_uuid() not null
        primary key,
    domain         varchar(100)                                       not null
        unique,
    tenant_id      varchar(50)                                        not null
        references admin_tenants (tenant_id),
    dns_provider   varchar(50),
    dns_record_id  varchar(100),
    cname_target   varchar(200),
    ssl_status     varchar(20)              default 'pending'::character varying
        constraint chk_ssl_status
            check ((ssl_status)::text = ANY
                   (ARRAY [('pending'::character varying)::text, ('active'::character varying)::text, ('expired'::character varying)::text, ('failed'::character varying)::text])),
    ssl_expires_at timestamp with time zone,
    auto_renew     boolean                  default true,
    status         varchar(20)              default 'active'::character varying
        constraint chk_domain_status
            check ((status)::text = ANY
                   (ARRAY [('active'::character varying)::text, ('inactive'::character varying)::text, ('verifying'::character varying)::text])),
    verified_at    timestamp with time zone,
    verified_by    varchar(50),
    created_at     timestamp with time zone default now(),
    updated_at     timestamp with time zone default now()
);

comment on table admin_domains is 'Domain management with SSL certificate tracking';

create index if not exists idx_domains_tenant
    on admin_domains (tenant_id);

create index if not exists idx_domains_status
    on admin_domains (status);

create table if not exists admin_redis_configs
(
    id                   uuid                     default gen_random_uuid() not null
        primary key,
    host                 varchar(100)                                       not null,
    port                 integer                  default 6379,
    password_hash        varchar(256),
    db_index             integer                  default 0,
    pool_size            integer                  default 100,
    min_idle_connections integer                  default 10,
    max_conn_age         interval                 default '00:05:00'::interval,
    status               varchar(20)              default 'active'::character varying,
    last_health_check    timestamp with time zone,
    avg_latency_ms       double precision         default 0,
    memory_used_mb       double precision         default 0,
    connected_clients    integer                  default 0,
    hits                 bigint                   default 0,
    misses               bigint                   default 0,
    created_at           timestamp with time zone default now(),
    updated_at           timestamp with time zone default now(),
    unique (host, port)
);

comment on table admin_redis_configs is 'Redis connection and pool configuration';

create table if not exists admin_db_configs
(
    id                   uuid                     default gen_random_uuid() not null
        primary key,
    dsn                  varchar(500)                                       not null,
    host                 varchar(100)                                       not null,
    port                 integer                  default 5432,
    dbname               varchar(100)                                       not null,
    max_open_connections integer                  default 25,
    max_idle_connections integer                  default 5,
    conn_max_lifetime    interval                 default '00:05:00'::interval,
    status               varchar(20)              default 'active'::character varying,
    last_health_check    timestamp with time zone,
    avg_query_time_ms    double precision         default 0,
    database_size_mb     double precision         default 0,
    total_tables         integer                  default 0,
    sequential_scans     bigint                   default 0,
    created_at           timestamp with time zone default now(),
    updated_at           timestamp with time zone default now(),
    unique (host, port, dbname)
);

comment on table admin_db_configs is 'PostgreSQL connection and pool configuration';

create table if not exists admin_database_backups
(
    id               uuid                     default gen_random_uuid() not null
        primary key,
    backup_type      varchar(20)              default 'manual'::character varying
        constraint chk_backup_type
            check ((backup_type)::text = ANY
                   (ARRAY [('manual'::character varying)::text, ('scheduled'::character varying)::text])),
    description      text,
    file_path        varchar(500),
    file_size_mb     double precision,
    status           varchar(20)              default 'running'::character varying
        constraint chk_backup_status
            check ((status)::text = ANY
                   (ARRAY [('running'::character varying)::text, ('completed'::character varying)::text, ('failed'::character varying)::text, ('deleted'::character varying)::text])),
    error_message    text,
    started_at       timestamp with time zone default now(),
    completed_at     timestamp with time zone,
    duration_seconds integer,
    created_by       varchar(50)
);

comment on table admin_database_backups is 'Database backup history with status tracking';

create index if not exists idx_backups_status
    on admin_database_backups (status);

create index if not exists idx_backups_created
    on admin_database_backups (started_at desc);

create table if not exists ent_roles
(
    id           uuid                     default gen_random_uuid() not null
        primary key,
    tenant_id    uuid                                               not null
        references tenants,
    name         varchar(64)                                        not null,
    display_name varchar(128),
    is_builtin   boolean                  default false,
    permissions  text[]                   default '{}'::text[],
    created_at   timestamp with time zone default now(),
    updated_at   timestamp with time zone default now(),
    unique (tenant_id, name)
);

comment on table ent_roles is 'Enterprise RBAC roles with permission points (resource:action)';

create table if not exists ent_user_roles
(
    user_id uuid not null
        references users
            on delete cascade,
    role_id uuid not null
        references ent_roles
            on delete cascade,
    primary key (user_id, role_id)
);

comment on table ent_user_roles is 'User-to-role assignments';

create index if not exists idx_ent_user_roles_role
    on ent_user_roles (role_id);

create table if not exists ent_groups
(
    id          uuid                     default gen_random_uuid() not null
        primary key,
    tenant_id   uuid                                               not null
        references tenants,
    name        varchar(128)                                       not null,
    description text,
    created_at  timestamp with time zone default now(),
    unique (tenant_id, name)
);

comment on table ent_groups is 'Enterprise user groups';

create table if not exists ent_group_members
(
    group_id uuid not null
        references ent_groups
            on delete cascade,
    user_id  uuid not null
        references users
            on delete cascade,
    primary key (group_id, user_id)
);

comment on table ent_group_members is 'Group membership (group -> user)';

create index if not exists idx_ent_group_members_user
    on ent_group_members (user_id);

create table if not exists ent_group_roles
(
    group_id uuid not null
        references ent_groups
            on delete cascade,
    role_id  uuid not null
        references ent_roles
            on delete cascade,
    primary key (group_id, role_id)
);

comment on table ent_group_roles is 'Group-to-role assignments (roles granted to all group members)';

create index if not exists idx_ent_group_roles_role
    on ent_group_roles (role_id);

create table if not exists ent_oidc_providers
(
    id                uuid                     default gen_random_uuid()           not null
        primary key,
    tenant_id         uuid                                                         not null
        references tenants,
    name              varchar(64)                                                  not null,
    issuer            varchar(512),
    client_id         varchar(256)                                                 not null,
    client_secret_enc text                                                         not null,
    scopes            text[]                   default '{openid,email,profile}'::text[],
    enabled           boolean                  default true,
    auto_provision    boolean                  default true,
    role_mapping      jsonb                    default '{}'::jsonb,
    created_at        timestamp with time zone default now(),
    updated_at        timestamp with time zone default now(),
    protocol          varchar(16)              default 'oidc'::character varying   not null,
    provider_type     varchar(32)              default 'custom'::character varying not null,
    display_name      varchar(64),
    icon              varchar(64),
    sort_order        integer                  default 100                         not null,
    auth_url          varchar(512),
    token_url         varchar(512),
    userinfo_url      varchar(512),
    extra             jsonb                    default '{}'::jsonb                 not null,
    unique (tenant_id, name)
);

comment on table ent_oidc_providers is 'Per-tenant OIDC identity providers (client_secret_enc is encrypted at rest)';

create table if not exists ent_user_identities
(
    id          uuid                     default gen_random_uuid() not null
        primary key,
    user_id     uuid                                               not null
        references users
            on delete cascade,
    provider_id uuid                                               not null
        references ent_oidc_providers
            on delete cascade,
    subject     varchar(256)                                       not null,
    email       varchar(255),
    created_at  timestamp with time zone default now(),
    unique (provider_id, subject)
);

comment on table ent_user_identities is 'External identity bindings (provider subject -> local user)';

create index if not exists idx_ent_user_identities_user
    on ent_user_identities (user_id);

create table if not exists ent_quota_pools
(
    id            uuid                     default gen_random_uuid()            not null
        primary key,
    tenant_id     uuid                                                          not null
        references tenants,
    resource_type varchar(20)                                                   not null
        constraint chk_quota_pool_resource
            check ((resource_type)::text = ANY
                   (ARRAY [('token'::character varying)::text, ('storage_mb'::character varying)::text, ('concurrency'::character varying)::text, ('credits'::character varying)::text])),
    total_amount  bigint                   default 0                            not null
        constraint chk_quota_pool_amount
            check (total_amount >= 0),
    period        varchar(10)              default 'monthly'::character varying not null
        constraint chk_quota_pool_period
            check ((period)::text = ANY
                   (ARRAY [('daily'::character varying)::text, ('monthly'::character varying)::text])),
    created_at    timestamp with time zone default now(),
    updated_at    timestamp with time zone default now(),
    unique (tenant_id, resource_type, period)
);

comment on table ent_quota_pools is 'Per-tenant quota pools by resource type and reset period';

create table if not exists ent_quota_allocations
(
    id          uuid                     default gen_random_uuid() not null
        primary key,
    pool_id     uuid                                               not null
        references ent_quota_pools
            on delete cascade,
    target_type varchar(10)                                        not null
        constraint chk_quota_alloc_target
            check ((target_type)::text = ANY
                   (ARRAY [('group'::character varying)::text, ('user'::character varying)::text])),
    target_id   uuid                                               not null,
    amount      bigint                   default 0                 not null
        constraint chk_quota_alloc_amount
            check (amount >= 0),
    created_at  timestamp with time zone default now(),
    unique (pool_id, target_type, target_id)
);

comment on table ent_quota_allocations is 'Quota allocations from a pool to a group or user';

create table if not exists ent_tenant_policies
(
    tenant_id           uuid not null
        primary key
        references tenants,
    privacy_mode        boolean                  default false,
    data_retention_days integer                  default 0,
    training_allowed    boolean                  default true,
    redaction_rules     jsonb                    default '{}'::jsonb,
    updated_at          timestamp with time zone default now()
);

comment on table ent_tenant_policies is 'Per-tenant compliance policies (privacy, retention, redaction)';

create table if not exists ent_model_policies
(
    id               uuid                     default gen_random_uuid() not null
        primary key,
    tenant_id        uuid                                               not null
        references tenants,
    role_id          uuid
                                                                        references ent_roles
                                                                            on delete set null,
    allowed_models   text[]                   default '{}'::text[]      not null,
    per_model_limits jsonb                    default '{}'::jsonb,
    created_at       timestamp with time zone default now(),
    updated_at       timestamp with time zone default now(),
    unique nulls not distinct (tenant_id, role_id)
);

comment on table ent_model_policies is 'Model allow-list and per-model limits by tenant and role';

create table if not exists ent_catalog_items
(
    id         uuid                     default gen_random_uuid()          not null
        primary key,
    type       varchar(8)                                                  not null
        constraint chk_catalog_item_type
            check ((type)::text = ANY
                   ((ARRAY ['plugin'::character varying, 'skill'::character varying, 'agent'::character varying, 'mcp'::character varying])::text[])),
    name       varchar(128)                                                not null,
    version    varchar(32)              default '1.0.0'::character varying,
    manifest   jsonb                    default '{}'::jsonb,
    status     varchar(16)              default 'draft'::character varying not null
        constraint chk_catalog_item_status
            check ((status)::text = ANY
                   (ARRAY [('draft'::character varying)::text, ('published'::character varying)::text, ('retired'::character varying)::text])),
    created_by uuid,
    created_at timestamp with time zone default now(),
    updated_at timestamp with time zone default now()
);

comment on table ent_catalog_items is 'Marketplace catalog entries (plugins and skills)';

create table if not exists ent_catalog_installs
(
    item_id      uuid not null
        references ent_catalog_items
            on delete cascade,
    tenant_id    uuid not null
        references tenants,
    enabled      boolean                  default true,
    installed_at timestamp with time zone default now(),
    primary key (item_id, tenant_id)
);

comment on table ent_catalog_installs is 'Which catalog items are installed per tenant';

create table if not exists payments
(
    id                varchar(64)                                                   not null
        primary key,
    user_id           varchar(32)                                                   not null,
    channel           varchar(16)                                                   not null,
    credits           integer                                                       not null,
    amount_cents      bigint                   default 0                            not null,
    currency          varchar(8)               default 'CNY'::character varying     not null,
    status            varchar(16)              default 'pending'::character varying not null,
    qr_code           text,
    provider_order_id varchar(64)              default ''::character varying        not null,
    trade_no          varchar(64)              default ''::character varying        not null,
    created_at        timestamp with time zone default now()                        not null,
    paid_at           timestamp with time zone,
    expired_at        timestamp with time zone
);

create index if not exists idx_payments_user
    on payments (user_id asc, created_at desc);

create index if not exists idx_payments_provider
    on payments (provider_order_id)
    where ((provider_order_id)::text <> ''::text);

create table if not exists ent_captcha_config
(
    id         uuid                     default gen_random_uuid()              not null
        primary key,
    tenant_id  uuid                                                            not null
        unique
        references tenants,
    provider   varchar(32)              default 'turnstile'::character varying not null,
    site_key   varchar(256)             default ''::character varying          not null,
    secret_enc text                     default ''::text                       not null,
    verify_url varchar(512),
    enabled    boolean                  default false                          not null,
    created_at timestamp with time zone default now(),
    updated_at timestamp with time zone default now()
);

comment on table ent_captcha_config is 'Per-tenant human verification (CAPTCHA) configuration (secret_enc encrypted at rest)';

create table if not exists memory_summaries
(
    id               varchar(64)                                                  not null
        primary key,
    tenant_id        varchar(64)                                                  not null,
    user_id          varchar(64)                                                  not null,
    session_id       varchar(64)                                                  not null,
    content          text                                                         not null,
    topics           jsonb                    default '[]'::jsonb                 not null,
    entities         jsonb                    default '{}'::jsonb                 not null,
    turn_start       integer                  default 0                           not null,
    turn_end         integer                  default 0                           not null,
    content_hash     varchar(80)                                                  not null,
    access_count     integer                  default 0                           not null
        constraint chk_ms_access_count
            check (access_count >= 0),
    last_accessed_at timestamp with time zone,
    status           varchar(16)              default 'active'::character varying not null
        constraint chk_ms_status
            check ((status)::text = ANY
                   ((ARRAY ['active'::character varying, 'archived'::character varying, 'expired'::character varying])::text[])),
    created_at       timestamp with time zone default now()                       not null,
    constraint chk_ms_turn_range
        check (turn_start <= turn_end)
);

comment on table memory_summaries is '对话摘要表（L3层）：存储跨会话的语义摘要记忆';

comment on column memory_summaries.content is '摘要内容文本';

comment on column memory_summaries.topics is '主题列表（JSONB数组）';

comment on column memory_summaries.entities is '实体字典（JSONB对象），按类型分组';

comment on column memory_summaries.turn_start is '摘要覆盖的起始轮次';

comment on column memory_summaries.turn_end is '摘要覆盖的结束轮次';

comment on column memory_summaries.content_hash is '内容哈希，用于去重';

comment on column memory_summaries.access_count is '访问次数，用于计算 final_score';

comment on column memory_summaries.last_accessed_at is '最近访问时间，用于计算 recency';

comment on column memory_summaries.status is '状态：active（活跃）、archived（已归档）、expired（已过期）';

create index if not exists idx_ms_lookup
    on memory_summaries (tenant_id asc, user_id asc, status asc, created_at desc);

create unique index if not exists uq_ms_hash
    on memory_summaries (tenant_id, user_id, content_hash);

create index if not exists idx_ms_access
    on memory_summaries (tenant_id asc, user_id asc, last_accessed_at desc, access_count desc);

create index if not exists idx_ms_session
    on memory_summaries (session_id)
    where ((status)::text = 'active'::text);

create table if not exists unified_sessions
(
    id             text                                          not null
        primary key,
    tenant_id      text                                          not null,
    user_id        text                                          not null,
    title          text                     default ''::text     not null,
    mode           text                     default 'auto'::text not null,
    shared_context jsonb                    default '{}'::jsonb  not null,
    created_at     timestamp with time zone default now()        not null,
    updated_at     timestamp with time zone default now()        not null
);

create index if not exists idx_unified_sessions_user_updated
    on unified_sessions (tenant_id asc, user_id asc, updated_at desc);

create table if not exists unified_messages
(
    id         bigserial
        primary key,
    session_id text                                         not null
        references unified_sessions
            on delete cascade,
    role       text                                         not null,
    content    text                                         not null,
    metadata   jsonb                    default '{}'::jsonb not null,
    error      text                     default ''::text    not null,
    created_at timestamp with time zone default now()       not null
);

create index if not exists idx_unified_messages_session
    on unified_messages (session_id, created_at);

create table if not exists domains
(
    id         uuid                     default gen_random_uuid()         not null
        primary key,
    tenant_id  uuid                                                       not null
        references tenants
            on delete cascade,
    domain     varchar(255)                                               not null
        unique,
    ssl_status varchar(16)              default 'none'::character varying not null,
    verified   boolean                  default false                     not null,
    created_at timestamp with time zone default now()                     not null,
    updated_at timestamp with time zone default now()                     not null
);

create table if not exists llm_models
(
    id             uuid                     default gen_random_uuid()     not null
        primary key,
    provider       varchar(32)                                            not null,
    name           varchar(128)                                           not null,
    display_name   varchar(128)             default ''::character varying not null,
    enabled        boolean                  default true                  not null,
    context_window integer                  default 8192                  not null,
    created_at     timestamp with time zone default now()                 not null,
    updated_at     timestamp with time zone default now()                 not null,
    unique (provider, name)
);

create table if not exists cron_jobs
(
    id            uuid                     default gen_random_uuid()            not null
        primary key,
    name          varchar(128)                                                  not null,
    schedule      varchar(64)                                                   not null,
    task          varchar(255)                                                  not null,
    enabled       boolean                  default true                         not null,
    last_run_at   timestamp with time zone,
    last_status   varchar(16)              default 'pending'::character varying not null,
    created_at    timestamp with time zone default now()                        not null,
    updated_at    timestamp with time zone default now()                        not null,
    tenant_id     uuid,
    user_id       uuid,
    webhook_token varchar(64)              default ''::character varying        not null
);

create table if not exists ent_templates
(
    id          uuid                     default gen_random_uuid() not null
        primary key,
    type        varchar(16)                                        not null,
    name        varchar(128)                                       not null,
    description text                     default ''::text          not null,
    payload     jsonb                                              not null,
    published   boolean                  default true              not null,
    created_at  timestamp with time zone default now()             not null,
    updated_at  timestamp with time zone default now()             not null
);

create index if not exists idx_ent_templates_type
    on ent_templates (type);

create table if not exists user_memory_profile
(
    tenant_id          varchar(64)                                                   not null,
    user_id            varchar(64)                                                   not null,
    slot               varchar(32)                                                   not null
        constraint chk_ump_slot
            check ((slot)::text = ANY
                   ((ARRAY ['identity'::character varying, 'preference'::character varying, 'decision'::character varying, 'fact'::character varying])::text[])),
    item_key           varchar(128)                                                  not null,
    item_value         jsonb                                                         not null,
    confidence         smallint                 default 50                           not null
        constraint chk_ump_confidence
            check ((confidence >= 0) AND (confidence <= 100)),
    source             varchar(16)              default 'derived'::character varying not null
        constraint chk_ump_source
            check ((source)::text = ANY
                   ((ARRAY ['user_confirmed'::character varying, 'derived'::character varying, 'tool_written'::character varying])::text[])),
    version            integer                  default 1                            not null,
    confirmed_at       timestamp with time zone,
    last_referenced_at timestamp with time zone,
    created_at         timestamp with time zone default now()                        not null,
    updated_at         timestamp with time zone default now()                        not null,
    primary key (tenant_id, user_id, slot, item_key)
);

comment on table user_memory_profile is '用户记忆档案表（L2层）：存储跨会话稳定的结构化事实';

comment on column user_memory_profile.slot is '槽位类型：identity（身份）、preference（偏好）、decision（关键决策）、fact（长期事实）';

comment on column user_memory_profile.item_key is '槽位内键，如 "timezone" / "preferred_language"';

comment on column user_memory_profile.item_value is '值，允许 JSONB 对象';

comment on column user_memory_profile.confidence is '置信度 0-100，用于淘汰排序';

comment on column user_memory_profile.source is '来源：user_confirmed（用户确认）、derived（提炼）、tool_written（工具写入）';

comment on column user_memory_profile.version is '版本号，每次 upsert 递增';

comment on column user_memory_profile.confirmed_at is '用户最后确认时间，NULL 表示未确认';

comment on column user_memory_profile.last_referenced_at is '最近被召回引用时间，用于衰退归档（180天未引用且confidence<80）';

create index if not exists idx_ump_reference
    on user_memory_profile (tenant_id, user_id, last_referenced_at);

create index if not exists idx_ump_conflict
    on user_memory_profile (tenant_id, user_id, source)
    where ((source)::text = 'user_confirmed'::text);

create trigger trigger_ump_updated_at
    before update
    on user_memory_profile
    for each row
execute procedure update_updated_at_column();

create table if not exists system_settings
(
    id         bigserial
        primary key,
    category   varchar(32)                            not null,
    key        varchar(64)                            not null,
    value      jsonb                                  not null,
    updated_at timestamp with time zone default now() not null,
    updated_by uuid,
    encrypted  boolean                  default false not null,
    unique (category, key)
);

create index if not exists idx_system_settings_category
    on system_settings (category);


create index if not exists idx_media_assets_name_tsv
    on media_assets using gin (to_tsvector('simple'::regconfig, COALESCE(name, ''::character varying)::text));


create index if not exists idx_messages_content_tsv
    on messages using gin (to_tsvector('simple'::regconfig, content));