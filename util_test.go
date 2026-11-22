package main

import (
	"testing"
	"time"
)

func TestMaskName(t *testing.T) {
	cases := map[string]string{
		"":              "?",
		"a":             "*",
		"ab":            "a*",
		"abc":           "ab*",
		"situs-panjang": "si***********",
	}
	for input, want := range cases {
		if got := maskName(input); got != want {
			t.Errorf("maskName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPickHost(t *testing.T) {
	hosts := []string{
		"project-abc123.vercel.app",
		"project-git-main-user.vercel.app",
		"project.vercel.app",
		"custom.example.com",
	}
	if got := pickHost(hosts, "project"); got != "project.vercel.app" {
		t.Fatalf("exact host should win, got %q", got)
	}
	if got := pickHost([]string{"project-abc123.vercel.app"}, "project"); got != "project-abc123.vercel.app" {
		t.Fatalf("shortest vercel host should win, got %q", got)
	}
	if got := pickHost([]string{"project-git-main-user.vercel.app"}, "project"); got != "" {
		t.Fatalf("git alias must never be picked, got %q", got)
	}
	if got := pickHost(nil, "project"); got != "" {
		t.Fatalf("empty list should return empty, got %q", got)
	}
}

func TestPrettyURL(t *testing.T) {
	if got := prettyURL("https://abc.vercel.app/"); got != "abc.vercel.app" {
		t.Fatalf("prettyURL = %q", got)
	}
	if got := prettyURL("http://localhost:3000/"); got != "localhost:3000" {
		t.Fatalf("prettyURL = %q", got)
	}
}

func TestFormatDate(t *testing.T) {
	value := time.Date(2026, time.November, 3, 7, 5, 0, 0, time.UTC)
	if got := formatDateShort(value); got != "3 Nov" {
		t.Fatalf("formatDateShort = %q", got)
	}
	if got := formatDateLong(value); got != "03 Nov 26 07.05" {
		t.Fatalf("formatDateLong = %q", got)
	}
	if got := formatDateShort(time.Time{}); got != "—" {
		t.Fatalf("zero time should be a dash, got %q", got)
	}
}

func TestIsZipAndHTMLName(t *testing.T) {
	for _, name := range []string{"a.zip", "A.ZIP"} {
		if !isZipName(name) {
			t.Errorf("%q should be a zip", name)
		}
	}
	for _, name := range []string{"index.html", "page.HTM"} {
		if !isHTMLName(name) {
			t.Errorf("%q should be html", name)
		}
	}
	if isZipName("a.rar") || isHTMLName("a.txt") {
		t.Fatalf("unexpected extension match")
	}
}
