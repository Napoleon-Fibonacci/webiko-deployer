package main

import (
	"database/sql"
	"strings"
	"testing"
	"time"
)

func renderTemplate(t *testing.T, name string, data any) string {
	t.Helper()
	var builder strings.Builder
	if err := templates.ExecuteTemplate(&builder, name, data); err != nil {
		t.Fatalf("execute %s: %v", name, err)
	}
	return builder.String()
}

func TestIndexTemplateRenders(t *testing.T) {
	html := renderTemplate(t, "index.html", pageData{
		Site:      site,
		Used:      3,
		HasUsed:   true,
		Max:       180,
		Remaining: 177,
		Rows: []ActiveRow{
			{ID: "1", Name: "situs-panjang", CreatedAt: time.Date(2026, 11, 3, 7, 5, 0, 0, time.UTC)},
		},
		JSONLD: jsonForScript(map[string]any{"@context": "https://schema.org"}),
	})
	for _, want := range []string{
		"Webiko Deployer",
		"Deploy situs ke Vercel",
		"177</strong> slot tersisa",
		"si***********<span>.vercel.app</span>",
		"3 Nov",
		"/static/app.js",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html missing %q", want)
		}
	}
}

func TestIndexTemplateEmpty(t *testing.T) {
	html := renderTemplate(t, "index.html", pageData{Site: site, Max: 180})
	if !strings.Contains(html, "Belum ada yang hidup.") {
		t.Errorf("empty ledger message missing")
	}
	if !strings.Contains(html, "maks 180 project") {
		t.Errorf("capacity fallback missing")
	}
	if strings.Contains(html, `id="access-code"`) {
		t.Errorf("access code field must stay hidden while deploy is open")
	}
}

func TestIndexTemplateAccessCodeField(t *testing.T) {
	html := renderTemplate(t, "index.html", pageData{Site: site, Max: 180, AccessCodeRequired: true})
	for _, want := range []string{`id="access-code"`, `id="access-error"`, "Deploy sedang dibatasi"} {
		if !strings.Contains(html, want) {
			t.Errorf("gated index.html missing %q", want)
		}
	}
}

func TestAdminTemplateRenders(t *testing.T) {
	html := renderTemplate(t, "admin.html", adminData{
		Site:    site,
		Status:  "",
		Query:   "",
		Total:   1,
		Matched: 1,
		Stats: []statusStat{
			{Key: "", Label: "Semua", Count: 1},
			{Key: "active", Label: "Aktif", Count: 1},
		},
		Rows: []AdminRow{
			{
				ID:         "row-1",
				Name:       "situs-ku",
				Status:     "active",
				SourceType: "zip",
				URL:        sql.NullString{String: "https://situs-ku.vercel.app/", Valid: true},
				CreatedAt:  time.Date(2026, 11, 3, 7, 5, 0, 0, time.UTC),
			},
		},
	})
	for _, want := range []string{
		"situs-ku",
		"situs-ku.vercel.app",
		"delete-trigger",
		"03 Nov 26 07.05",
		"Bersihkan",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("admin.html missing %q", want)
		}
	}
}

func TestLoginTemplateRenders(t *testing.T) {
	html := renderTemplate(t, "login.html", loginData{Site: site, SessionTime: 12})
	if !strings.Contains(html, "Masuk admin") || !strings.Contains(html, "Sesi bertahan 12 jam") {
		t.Errorf("login form missing content")
	}

	disabled := renderTemplate(t, "login.html", loginData{Site: site, Disabled: true})
	if !strings.Contains(disabled, "Admin nonaktif") {
		t.Errorf("disabled state missing")
	}

	wrong := renderTemplate(t, "login.html", loginData{Site: site, Error: "1", SessionTime: 12})
	if !strings.Contains(wrong, "Password salah.") {
		t.Errorf("wrong-password message missing")
	}
}
