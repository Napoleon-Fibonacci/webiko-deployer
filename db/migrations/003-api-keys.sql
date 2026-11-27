-- Kunci API Vercel hasil donasi: pool terenkripsi + jejak kunci per deploy.
-- Aman diulang; dipakai untuk database lama yang sudah punya 001 dan 002.

CREATE TABLE IF NOT EXISTS api_keys (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  token           text NOT NULL,
  label           text,
  donor_owner_id  text,
  uses            integer NOT NULL DEFAULT 0,
  active          boolean NOT NULL DEFAULT false,
  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS api_keys_active_uses ON api_keys (uses) WHERE active;

ALTER TABLE deployments ADD COLUMN IF NOT EXISTS api_key_id uuid
  REFERENCES api_keys (id) ON DELETE SET NULL;
