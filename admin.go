package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Single-password admin session: httpOnly cookie "exp.signature" signed with
// HMAC-SHA256 using ADMIN_PASSWORD as the key. Changing the password kills
// every old session.
const (
	adminCookie = "webiko_admin"
	adminMaxAge = 12 * time.Hour
)

func adminPassword() string {
	return os.Getenv("ADMIN_PASSWORD")
}

func adminEnabled() bool {
	return adminPassword() != ""
}

func checkPassword(input string) bool {
	expected := adminPassword()
	if expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(input), []byte(expected)) == 1
}

func adminSignature(payload string) string {
	mac := hmac.New(sha256.New, []byte(adminPassword()))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func createAdminToken() string {
	expires := strconv.FormatInt(time.Now().Add(adminMaxAge).UnixMilli(), 10)
	return expires + "." + adminSignature(expires)
}

func verifyAdminToken(token string) bool {
	if token == "" || !adminEnabled() {
		return false
	}
	payload, given, ok := strings.Cut(token, ".")
	if !ok || payload == "" || given == "" {
		return false
	}
	expected := adminSignature(payload)
	if subtle.ConstantTimeCompare([]byte(given), []byte(expected)) != 1 {
		return false
	}
	expires, err := strconv.ParseInt(payload, 10, 64)
	return err == nil && expires > time.Now().UnixMilli()
}

func isAdmin(r *http.Request) bool {
	cookie, err := r.Cookie(adminCookie)
	if err != nil {
		return false
	}
	return verifyAdminToken(cookie.Value)
}

func requestIsHTTPS(r *http.Request) bool {
	if r != nil && (r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")) {
		return true
	}
	return os.Getenv("VERCEL") == "1" || strings.HasPrefix(site.URL, "https://")
}
