package main

import (
	"net/http/httptest"
	"testing"
)

func TestGetClientIPPrefersRealIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	r.Header.Set("X-Real-IP", "203.0.113.9")
	r.RemoteAddr = "10.0.0.1:1234"

	if got := getClientIP(r); got != "203.0.113.9" {
		t.Fatalf("getClientIP = %q, want trusted X-Real-IP", got)
	}
}

func TestGetClientIPPrefersVercelHeader(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	r.Header.Set("X-Vercel-Forwarded-For", "198.51.100.7")

	if got := getClientIP(r); got != "198.51.100.7" {
		t.Fatalf("getClientIP = %q, want X-Vercel-Forwarded-For", got)
	}
}

func TestGetClientIPUsesNearestForwardedHop(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 203.0.113.7")

	if got := getClientIP(r); got != "203.0.113.7" {
		t.Fatalf("getClientIP = %q, want the last forwarded hop", got)
	}
}

func TestGetClientIPFallsBackToRemoteAddr(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.0.2.5:4321"

	if got := getClientIP(r); got != "192.0.2.5" {
		t.Fatalf("getClientIP = %q, want RemoteAddr host", got)
	}
}

func TestSameOrigin(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		origin  string
		referer string
		want    bool
	}{
		{name: "matching origin", host: "example.com", origin: "https://example.com", want: true},
		{name: "different origin", host: "example.com", origin: "https://evil.example", want: false},
		{name: "null origin", host: "example.com", origin: "null", want: false},
		{name: "referer fallback", host: "example.com", referer: "https://example.com/admin", want: true},
		{name: "bad referer", host: "example.com", referer: "https://evil.example/x", want: false},
		{name: "no browser headers", host: "example.com", want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/admin/delete", nil)
			r.Host = test.host
			if test.origin != "" {
				r.Header.Set("Origin", test.origin)
			}
			if test.referer != "" {
				r.Header.Set("Referer", test.referer)
			}
			if got := sameOrigin(r); got != test.want {
				t.Fatalf("sameOrigin = %v, want %v", got, test.want)
			}
		})
	}
}
