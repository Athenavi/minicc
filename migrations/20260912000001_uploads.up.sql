-- 通用分片上传（断点续传）
-- 分片存于 <storageRoot>/uploads/{upload_id}/chunk_{index} 临时目录；
-- complete 时按 purpose 合并并落库（media → media_assets；kb_doc → knowledge_documents）
CREATE TABLE uploads (
    id varchar(64) PRIMARY KEY,
    user_id varchar(64) NOT NULL DEFAULT '',
    name varchar(255) NOT NULL,
    size bigint NOT NULL DEFAULT 0,
    mime_type varchar(64) NOT NULL DEFAULT '',
    purpose varchar(16) NOT NULL DEFAULT 'generic',   -- media / kb_doc / generic
    parent_id varchar(64) NOT NULL DEFAULT '',        -- media 文件夹 / kb_id
    category varchar(64) NOT NULL DEFAULT '',
    chunk_size int NOT NULL DEFAULT 2097152,          -- 2MB
    chunk_count int NOT NULL DEFAULT 0,
    chunks_received text[] NOT NULL DEFAULT '{}',
    status varchar(16) NOT NULL DEFAULT 'pending',    -- pending / uploading / completed / failed / expired
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now()
);
CREATE INDEX idx_uploads_user ON uploads (user_id, created_at DESC);
