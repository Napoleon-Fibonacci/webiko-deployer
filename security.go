package main

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// parseClientIP accepts a bare IP, an IPv6 value in brackets, or host:port.
func parseClientIP(value string) net.IP {
	value = strings.Trim(strings.TrimSpace(value), "[]")
	if ip := net.ParseIP(value); ip != nil {
		return ip
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		return net.ParseIP(host)
	}
	return nil
}

func firstValidIP(value string) string {
	for _, part := range strings.Split(value, ",") {
		if ip := parseClientIP(part); ip != nil {
			return ip.String()
		}
	}
	return ""
}

// getClientIP prefers headers that Vercel overwrites with the real client IP.
// Client-supplied X-Forwarded-For entries are only used as a fallback, and the
// nearest proxy (the last entry) wins so a spoofed prefix cannot skip quotas.
func getClientIP(r *http.Request) string {
	if r == nil {
		return "unknown"
	}
	if ip := firstValidIP(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	if ip := firstValidIP(r.Header.Get("X-Vercel-Forwarded-For")); ip != "" {
		return ip
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			if ip := parseClientIP(parts[i]); ip != nil {
				return ip.String()
			}
		}
	}
	if host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr)); err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}
	return "unknown"
}

// sameOrigin blocks cross-site form posts to admin endpoints. Browsers send
// Origin (and Referer) on form POSTs; curl and other non-browser clients may
// omit them and are allowed through, still protected by the session cookie.
func sameOrigin(r *http.Request) bool {
	if r == nil {
		return false
	}
	source := strings.TrimSpace(r.Header.Get("Origin"))
	if source == "" {
		source = strings.TrimSpace(r.Header.Get("Referer"))
	}
	if source == "" {
		return true
	}
	parsed, err := url.Parse(source)
	if err != nil || parsed.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Host, r.Host)
}

func requireSameOrigin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			http.Error(w, "Permintaan ditolak.", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// bearerToken accepts any casing of the "Bearer" scheme and ignores stray
// spaces around the token. Pasted secrets often carry a trailing space, and
// HTTP itself strips trailing whitespace from header values.
func bearerToken(header string) string {
	const prefix = "bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

// cronAuthorized compares only the token, in constant time. An empty
// CRON_SECRET always fails closed.
func cronAuthorized(r *http.Request) bool {
	secret := strings.TrimSpace(os.Getenv("CRON_SECRET"))
	if secret == "" || r == nil {
		return false
	}
	given := bearerToken(r.Header.Get("Authorization"))
	if given == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(given), []byte(secret)) == 1
}
