-- Add permanent storage_id to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS storage_id VARCHAR(64) UNIQUE;

-- Create guest storage mapping table (for unauthenticated users)
CREATE TABLE IF NOT EXISTS guest_storage (
    client_id VARCHAR(64) PRIMARY KEY,
    storage_id VARCHAR(64) UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_guest_storage_id ON guest_storage(storage_id);
