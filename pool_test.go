package main

import (
	"strings"
	"testing"
)

func TestEncryptDecryptToken(t *testing.T) {
	secret := "kunci-rahasia-untuk-uji"
	t.Setenv("POOL_SECRET", secret)

	encrypted, err := encryptToken("vercel-token-asli")
	if err != nil {
		t.Fatalf("encryptToken: %v", err)
	}
	if strings.Contains(encrypted, "vercel-token-asli") {
		t.Fatal("token terenkripsi masih berisi teks asli")
	}

	decrypted, err := decryptToken(encrypted)
	if err != nil {
		t.Fatalf("decryptToken: %v", err)
	}
	if decrypted != "vercel-token-asli" {
		t.Fatalf("decryptToken = %q, want %q", decrypted, "vercel-token-asli")
	}
}

func TestDecryptTokenTampered(t *testing.T) {
	t.Setenv("POOL_SECRET", "rahasia")
	encrypted, err := encryptToken("token")
	if err != nil {
		t.Fatalf("encryptToken: %v", err)
	}
	if _, err := decryptToken(encrypted[:len(encrypted)-2]); err == nil {
		t.Fatal("decryptToken menerima ciphertext yang dimanipulasi")
	}
}

func TestMaskKey(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"abcdefghij", "abcd…ghij"},
		{"short", "short"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := maskKey(tc.in); got != tc.want {
			t.Errorf("maskKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPoolKeyFallback(t *testing.T) {
	t.Setenv("POOL_SECRET", "")
	t.Setenv("OWNER_COOKIE_SECRET", "")
	t.Setenv("CRON_SECRET", "")
	t.Setenv("VERCEL_TOKEN", "")
	t.Setenv("ADMIN_PASSWORD", "")
	if len(poolKey()) != 32 {
		t.Fatalf("poolKey() panjang = %d, want 32", len(poolKey()))
	}
}
