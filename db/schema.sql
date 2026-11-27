-- Web Deployer Vercel — skema Neon Postgres.
-- Jalankan sekali di database Neon.

CREATE TABLE deployments (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name               text NOT NULL,
  vercel_project_id  text,
  url                text,
  source_type        text NOT NULL CHECK (source_type IN ('github','zip')),
  status             text NOT NULL CHECK (status IN ('pending','active','deleting','deleted','failed')),
  owner_id           text,
  created_at         timestamptz NOT NULL DEFAULT now(),
  deleted_at         timestamptz
);

-- Nama unik hanya untuk baris yang masih hidup (pending/active/deleting).
-- Baris deleted boleh nama kembar (nama bekas boleh dipakai lagi).
CREATE UNIQUE INDEX deployments_name_live
  ON deployments (name) WHERE status IN ('pending','active','deleting');

-- Pencarian project tertua yang aktif.
CREATE INDEX deployments_oldest
  ON deployments (created_at) WHERE status = 'active';

-- Kuota per browser: cari situs milik satu owner dan hitung yang masih hidup.
CREATE INDEX deployments_owner_live
  ON deployments (owner_id, created_at)
  WHERE status IN ('pending','active','deleting');

-- Rate limit jendela tetap per (scope, IP). Satu tabel untuk semua kebijakan;
-- scope memisahkan kuota deploy dari kuota percobaan masuk admin.
-- Lihat RATE_LIMITS di lib/ratelimit.ts.
-- Jendela yang sudah kedaluwarsa dibuang oleh sapuan harian
-- (PRUNE_RATE_LIMITS_SQL di lib/deployments.ts).
CREATE TABLE rate_limits (
  scope          text NOT NULL,
  ip             text NOT NULL,
  window_start   timestamptz NOT NULL,
  count          int NOT NULL DEFAULT 0,
  PRIMARY KEY (scope, ip, window_start)
);

-- Kunci API Vercel yang didonasikan lewat banner halaman depan. Token disimpan
-- terenkripsi (AES-GCM, kunci dari env POOL_SECRET). active = false berarti
-- menunggu aktivasi admin dari dashboard /admin.
CREATE TABLE api_keys (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  token           text NOT NULL,
  label           text,
  donor_owner_id  text,
  uses            integer NOT NULL DEFAULT 0,
  active          boolean NOT NULL DEFAULT false,
  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX api_keys_active_uses ON api_keys (uses) WHERE active;

-- Deploy mencatat kunci pool yang dipakai, agar penghapusan project di Vercel
-- memakai token yang sama dengan saat pembuatan.
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS api_key_id uuid
  REFERENCES api_keys (id) ON DELETE SET NULL;
