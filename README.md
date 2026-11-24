# Webiko Deployer

Deployer situs HTML statis ke Vercel tanpa akun Vercel sendiri. Satu token
Vercel disimpan di server; pengunjung membuka situs, mengisi nama, memilih
sumber (repo GitHub, ZIP/folder, atau tempel kode), lalu deploy.

Frontend-nya HTML/CSS/JS biasa. Backend-nya satu server Go (`net/http`) yang
melayani halaman, API, dan aset yang di-embed ke binary. Tidak ada Node,
React, atau proses build frontend.

## Struktur

```
main.go               Route, embed aset, bootstrap server
config.go             Identitas situs + env
pages.go              Halaman depan, admin, login, robots/sitemap/manifest
deploy_handler.go     POST /api/deploy (NDJSON) + GET /api/check-name
upload_handler.go     Token client upload Vercel Blob + callback
sources.go            Ambil file dari GitHub atau ZIP di Blob
vercel.go             Buat deployment, polling, alias, hapus project
deployments.go        Tabel deployments: slot, transisi, daftar admin, sapuan
ratelimit.go          Kuota jendela tetap per scope + IP
admin.go              Cookie sesi admin HMAC
validate.go           Nama, path, ukuran, anti zip bomb
db.go                 Koneksi Postgres (pgx)
views.go / util.go    Template funcs, ikon SVG, format tanggal
views/                index.html, admin.html, login.html
web/                  app.css, app.js, vendor/fflate.js, ikon + og.png
db/schema.sql         Skema Neon Postgres
db/migrations/        Migrasi untuk database lama
vercel.json           Framework preset Go + cron harian
```

## Setup lokal

Butuh **Go 1.26+** dan Postgres (Neon).

```bash
cp .env.example .env
# isi DATABASE_URL, VERCEL_TOKEN, BLOB_READ_WRITE_TOKEN, ADMIN_PASSWORD
psql "$DATABASE_URL" -f db/schema.sql
go mod tidy
go run .
```

Buka `http://localhost:3000`. Tanpa `DATABASE_URL`, halaman tetap tampil,
tetapi deploy, cek nama, dan admin tidak bisa dipakai.

```bash
go test ./...
go vet ./...
```

Sudah punya database dari versi Next.js? Jalankan berkas di `db/migrations/`
sesuai nomor urut. Migrasi aman diulang.

## Environment

| Variabel | Wajib | Keterangan |
| --- | --- | --- |
| `DATABASE_URL` | ya | Connection string Neon/Postgres. |
| `VERCEL_TOKEN` | ya | Token API Vercel, server saja. |
| `VERCEL_TEAM_ID` | tidak | Isi kalau project ada di dalam team. |
| `BLOB_READ_WRITE_TOKEN` | ya | Token Vercel Blob untuk upload ZIP. |
| `MAX_PROJECTS` | tidak | Default `180`. |
| `SITE_TTL_DAYS` | tidak | Umur situs dalam hari. Default `7`, maksimal `30`. |
| `CRON_SECRET` | tidak | Kalau diisi, cron memakai `Authorization: Bearer <secret>`. |
| `SITE_URL` | tidak | URL publik. Di Vercel otomatis dari `VERCEL_PROJECT_PRODUCTION_URL`. |
| `ADMIN_PASSWORD` | tidak | Kosong = menu `/admin` nonaktif. |

## Alur deploy

1. Nama divalidasi lokal (3–40 huruf kecil/angka/hyphen) lalu dicek ke database
   dan DNS `*.vercel.app`. Kalau Vercel tetap memberi host dengan suffix,
   deploy dibatalkan dan pengunjung diminta memilih nama lain.
2. Sumber ZIP/folder/ditempel dibungkus jadi satu ZIP di browser (fflate),
   lalu diunggah langsung ke Vercel Blob memakai token client. Ini yang membuat
   ZIP sampai 10 MB bisa lewat, di atas batas body request function Vercel.
3. `POST /api/deploy` memvalidasi isi ZIP (maks 100 file, 50 MB setelah
   ekstrak, wajib `index.html`), memesan slot, membuat deployment Vercel, dan
   mengalirkan progres NDJSON ke browser.
4. Kalau kapasitas penuh, project tertua dihapus lebih dulu. Project yang sudah
   hilang dari Vercel ditutup oleh sapuan harian (`/api/cron/cleanup`).

Batas: deploy 5/jam, upload 20/jam, login admin 10/15 menit, per IP.

## Deploy ke Vercel

1. Push folder ini ke repo Git, lalu import di Vercel.
2. Di project Vercel, tambahkan **Neon** (Marketplace) dan **Blob** (Storage).
   Store Blob harus **public**, karena server mengambil ZIP dari URL hasil upload.
3. Set env `VERCEL_TOKEN`, `BLOB_READ_WRITE_TOKEN`, `ADMIN_PASSWORD`, dan
   opsional `VERCEL_TEAM_ID`, `MAX_PROJECTS`, `SITE_URL`, `CRON_SECRET`.
4. Jalankan `db/schema.sql` sekali di database Neon.
5. Deploy. `vercel.json` sudah menyetel `"framework": "go"` dan cron harian.

Catatan:

- Runtime Go di Vercel masih **Beta** dan membaca versi dari `go.mod`.
- Function default 300 detik. Deploy menunggu Vercel maksimal ~55 detik.
- `go.sum` wajib ikut ter-commit. Jalankan `go mod tidy` sebelum push.
- Gambar OG ada di `web/og.png` (1200×630) dan disajikan di `/og.png`.
- Menu admin: `/admin`. Daftar publik menyamarkan nama project di server
  (dua huruf pertama + bintang).
