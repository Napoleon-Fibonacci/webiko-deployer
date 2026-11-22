package main

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAdminTokenRoundTrip(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "rahasia")
	if !adminEnabled() {
		t.Fatalf("admin should be enabled with a password")
	}
	token := createAdminToken()
	if !verifyAdminToken(token) {
		t.Fatalf("fresh token rejected: %q", token)
	}
	if verifyAdminToken(token + "x") {
		t.Fatalf("tampered signature accepted")
	}
	if verifyAdminToken("") {
		t.Fatalf("empty token accepted")
	}
}

func TestAdminTokenExpires(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "rahasia")
	expired := strconv.FormatInt(time.Now().Add(-time.Minute).UnixMilli(), 10)
	if verifyAdminToken(expired + "." + adminSignature(expired)) {
		t.Fatalf("expired token accepted")
	}
	// Payload tak bisa dipalsukan tanpa tanda tangan yang cocok.
	future := strconv.FormatInt(time.Now().Add(time.Hour).UnixMilli(), 10)
	if verifyAdminToken(future + "." + adminSignature(expired)) {
		t.Fatalf("signature for another payload accepted")
	}
}

func TestAdminTokenDiesWithPasswordChange(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "lama")
	token := createAdminToken()
	t.Setenv("ADMIN_PASSWORD", "baru")
	if verifyAdminToken(token) {
		t.Fatalf("old session survived a password change")
	}
}

func TestAdminDisabledWithoutPassword(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "")
	if adminEnabled() {
		t.Fatalf("admin must be off without ADMIN_PASSWORD")
	}
	if checkPassword("") {
		t.Fatalf("empty password must never authenticate")
	}
	token := createAdminToken()
	if !strings.Contains(token, ".") || verifyAdminToken(token) {
		t.Fatalf("token must be rejected while admin is off")
	}
}

func TestCheckPassword(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "rahasia")
	if !checkPassword("rahasia") {
		t.Fatalf("correct password rejected")
	}
	if checkPassword("salah") || checkPassword("") {
		t.Fatalf("wrong password accepted")
	}
}
