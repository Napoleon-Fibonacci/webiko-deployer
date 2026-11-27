package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"log"
	"os"
	"strings"
	"time"
)

// Kunci API Vercel yang didonasikan. Token disimpan terenkripsi AES-GCM;
// kunci enkripsi dari env (POOL_SECRET dulu, lalu rahasia server yang sudah
// wajib ada). Kunci aktif dipakai bergiliran, yang paling jarang dipakai
// duluan. VERCEL_TOKEN tetap jadi cadangan saat pool kosong.

var errNoActiveKey = errors.New("tidak ada kunci API aktif di pool")

type APIKeyRow struct {
	ID           string
	Label        string
	DonorOwnerID string
	Uses         int
	Active       bool
	CreatedAt    time.Time
}

func poolKey() []byte {
	for _, name := range []string{
		"POOL_SECRET",
		"OWNER_COOKIE_SECRET",
		"CRON_SECRET",
		"VERCEL_TOKEN",
		"ADMIN_PASSWORD",
	} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			sum := sha256.Sum256([]byte(value))
			return sum[:]
		}
	}
	sum := sha256.Sum256([]byte("webiko-pool-v1:local"))
	return sum[:]
}

func encryptToken(plain string) (string, error) {
	block, err := aes.NewCipher(poolKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func decryptToken(encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(poolKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("token terenkripsi rusak")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// maskKey membuat pratinjau untuk dashboard admin dan pesan Telegram; kunci
// utuh tidak pernah ditampilkan di luar pesan donasi awal.
func maskKey(token string) string {
	token = strings.TrimSpace(token)
	if len(token) <= 8 {
		return token
	}
	return token[:4] + "…" + token[len(token)-4:]
}

func insertAPIKey(token, donorOwner string, active bool) error {
	encrypted, err := encryptToken(strings.TrimSpace(token))
	if err != nil {
		return err
	}
	return execQuery(
		`INSERT INTO api_keys (token, label, donor_owner_id, active)
		 VALUES ($1, $2, NULLIF($3, ''), $4)`,
		encrypted, maskKey(token), donorOwner, active,
	)
}

func setAPIKeyActive(id string, active bool) error {
	return execQuery(`UPDATE api_keys SET active = $1 WHERE id = $2`, active, id)
}

func deleteAPIKey(id string) error {
	return execQuery(`DELETE FROM api_keys WHERE id = $1`, id)
}

func listAPIKeys() ([]APIKeyRow, error) {
	db, err := getDB()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx,
		`SELECT id, label, donor_owner_id, uses, active, created_at
		 FROM api_keys ORDER BY active DESC, uses ASC, created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKeyRow
	for rows.Next() {
		var row APIKeyRow
		var donor sql.NullString
		if err = rows.Scan(&row.ID, &row.Label, &donor, &row.Uses, &row.Active, &row.CreatedAt); err != nil {
			return nil, err
		}
		row.DonorOwnerID = donor.String
		out = append(out, row)
	}
	return out, rows.Err()
}

// pickAPIKey mengambil kunci aktif dengan pemakaian terkecil lalu menambah
// hitungannya. Tanpa kunci aktif, errNoActiveKey menyuruh pemanggil kembali
// ke VERCEL_TOKEN.
func pickAPIKey() (string, string, error) {
	db, err := getDB()
	if err != nil {
		return "", "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var id, encrypted string
	err = db.QueryRowContext(ctx,
		`SELECT id, token FROM api_keys WHERE active
		 ORDER BY uses ASC, created_at ASC LIMIT 1 FOR UPDATE SKIP LOCKED`,
	).Scan(&id, &encrypted)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", errNoActiveKey
	}
	if err != nil {
		return "", "", err
	}
	token, err := decryptToken(encrypted)
	if err != nil {
		return "", "", err
	}
	if _, err = db.ExecContext(ctx, `UPDATE api_keys SET uses = uses + 1 WHERE id = $1`, id); err != nil {
		return "", "", err
	}
	return id, token, nil
}

// loadToken membuka kunci terenkripsi dari database. Gagal membuka berarti
// kunci tidak bisa dipakai; pemanggil yang memutuskan cadangan.
func loadToken(keyID string) (string, error) {
	db, err := getDB()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var encrypted string
	if err = db.QueryRowContext(ctx, `SELECT token FROM api_keys WHERE id = $1`, keyID).Scan(&encrypted); err != nil {
		return "", err
	}
	return decryptToken(encrypted)
}

// tokenForDeployment mengembalikan token untuk satu baris deploy: kunci pool
// bila tercatat, VERCEL_TOKEN bila tidak (cadangan atau deploy lama).
func tokenForDeployment(keyID sql.NullString) string {
	if keyID.Valid && keyID.String != "" {
		if token, err := loadToken(keyID.String); err == nil && token != "" {
			return token
		} else if err != nil {
			log.Printf("load token %s: %v", keyID.String, err)
		}
	}
	return strings.TrimSpace(os.Getenv("VERCEL_TOKEN"))
}
