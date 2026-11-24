package main

import (
	"testing"
	"time"
)

func TestAllowedBlobURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "uploaded zip",
			url:  "https://abc.public.blob.vercel-storage.com/uploads/my-site-123.zip",
			want: true,
		},
		{
			name: "other host",
			url:  "https://evil.example/uploads/my-site.zip",
			want: false,
		},
		{
			name: "lookalike host",
			url:  "https://blob.vercel-storage.com.evil.example/uploads/x.zip",
			want: false,
		},
		{
			name: "wrong folder",
			url:  "https://abc.public.blob.vercel-storage.com/private/x.zip",
			want: false,
		},
		{
			name: "path traversal",
			url:  "https://abc.public.blob.vercel-storage.com/uploads/../private/x.zip",
			want: false,
		},
		{
			name: "http not https",
			url:  "http://abc.public.blob.vercel-storage.com/uploads/x.zip",
			want: false,
		},
		{
			name: "empty",
			url:  "",
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := allowedBlobURL(test.url); got != test.want {
				t.Fatalf("allowedBlobURL(%q) = %v, want %v", test.url, got, test.want)
			}
		})
	}
}

func TestSiteTTL(t *testing.T) {
	t.Setenv("SITE_TTL_DAYS", "3")
	if days := siteTTLDays(); days != 3 {
		t.Fatalf("siteTTLDays = %d, want 3", days)
	}
	created := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if got := expiryFor(created); !got.Equal(created.Add(72 * time.Hour)) {
		t.Fatalf("expiryFor = %v, want +72h", got)
	}
}

func TestSiteTTLFallsBackOnBadInput(t *testing.T) {
	for _, value := range []string{"", "0", "-2", "abc", "31"} {
		t.Setenv("SITE_TTL_DAYS", value)
		if days := siteTTLDays(); days != defaultSiteTTLDays {
			t.Fatalf("siteTTLDays(%q) = %d, want %d", value, days, defaultSiteTTLDays)
		}
	}
}

func TestExpiryForZeroTime(t *testing.T) {
	if got := expiryFor(time.Time{}); !got.IsZero() {
		t.Fatalf("expiryFor(zero) = %v, want zero", got)
	}
}
