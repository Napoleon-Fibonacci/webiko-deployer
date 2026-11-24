package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// A random, signed browser identity. It is not a login: it only groups sites
// so one browser cannot slowly take over every slot. Clearing cookies starts a
// new identity, which is why the per-IP deploy quota still exists.
const (
	ownerCookie = "webiko_owner"
	ownerMaxAge = 30 * 24 * time.Hour
)

var ownerIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)

// ownerKey prefers a dedicated secret, then falls back to other server-side
// secrets that are already required in production. A local run without any of
// them still works, but its cookies are not meant to be private.
func ownerKey() []byte {
	for _, name := range []string{
		"OWNER_COOKIE_SECRET",
		"CRON_SECRET",
		"VERCEL_TOKEN",
		"BLOB_READ_WRITE_TOKEN",
		"ADMIN_PASSWORD",
	} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return []byte("webiko-owner-v1:" + value)
		}
	}
	return []byte("webiko-owner-v1:local")
}

func newOwnerID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		// crypto/rand failure is unrecoverable; fall back to a time-based ID
		// so the deploy can still proceed.
		return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
	}
	return base64.RawURLEncoding.EncodeToString(buffer)
}

func ownerSignature(payload string) string {
	mac := hmac.New(sha256.New, ownerKey())
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func signOwner(id string) string {
	expires := strconv.FormatInt(time.Now().Add(ownerMaxAge).Unix(), 10)
	payload := id + "." + expires
	return payload + "." + ownerSignature(payload)
}

func parseOwner(value string) (string, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 || !ownerIDPattern.MatchString(parts[0]) {
		return "", false
	}
	expires, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return "", false
	}
	expected := ownerSignature(parts[0] + "." + parts[1])
	if !hmac.Equal([]byte(parts[2]), []byte(expected)) {
		return "", false
	}
	return parts[0], true
}

func ownerFromRequest(r *http.Request) (string, bool) {
	if r == nil {
		return "", false
	}
	cookie, err := r.Cookie(ownerCookie)
	if err != nil {
		return "", false
	}
	return parseOwner(cookie.Value)
}

// ensureOwner must run before the streaming response headers are written. It
// reuses a valid cookie and refreshes its expiry; otherwise it issues a new ID.
func ensureOwner(w http.ResponseWriter, r *http.Request) string {
	id, ok := ownerFromRequest(r)
	if !ok {
		id = newOwnerID()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     ownerCookie,
		Value:    signOwner(id),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsHTTPS(r),
		MaxAge:   int(ownerMaxAge.Seconds()),
	})
	return id
}
