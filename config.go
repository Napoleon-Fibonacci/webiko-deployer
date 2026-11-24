package main

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Site identity: one home for the name, description, canonical URL and locale.
type Site struct {
	Name        string
	Title       string
	Tagline     string
	Description string
	URL         string
	Locale      string
	Lang        string
	ThemeColor  string
	Keywords    []string
}

func resolveSiteURL() string {
	for _, key := range []string{"SITE_URL", "NEXT_PUBLIC_SITE_URL", "VERCEL_PROJECT_PRODUCTION_URL"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		value = strings.TrimRight(value, "/")
		if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
			value = "https://" + value
		}
		return value
	}
	return "http://localhost:3000"
}

func newSite() Site {
	return Site{
		Name:    "Webiko Deployer",
		Title:   "Webiko Deployer — Deploy situs HTML statis ke Vercel",
		Tagline: "Deploy situs HTML statis ke Vercel",
		Description: "Deploy situs HTML statis ke Vercel tanpa akun Vercel sendiri. " +
			"Sumber ZIP atau repo GitHub, nama dicek otomatis, situs aktif sekitar 7 hari lalu digilir otomatis.",
		URL:        resolveSiteURL(),
		Locale:     "id_ID",
		Lang:       "id",
		ThemeColor: "#121314",
		Keywords: []string{
			"deploy situs statis",
			"deploy HTML ke Vercel",
			"hosting statis gratis",
			"deploy ZIP ke Vercel",
			"deploy repo GitHub",
		},
	}
}

var site = newSite()

// A site lives for a fixed number of days, measured from created_at. No new
// column is needed: expiry is derived, and the daily cron deletes overdue rows.
const defaultSiteTTLDays = 7

const (
	defaultMaxSitesPerOwner = 2
	defaultMinEvictHours    = 24
)

func maxSitesPerOwner() int {
	raw := strings.TrimSpace(os.Getenv("MAX_SITES_PER_OWNER"))
	if raw == "" {
		return defaultMaxSitesPerOwner
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > 10 {
		return defaultMaxSitesPerOwner
	}
	return value
}

func minEvictHours() int {
	raw := strings.TrimSpace(os.Getenv("MIN_EVICT_HOURS"))
	if raw == "" {
		return defaultMinEvictHours
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > 168 {
		return defaultMinEvictHours
	}
	return value
}

func minEvictAge() time.Duration {
	return time.Duration(minEvictHours()) * time.Hour
}

func siteTTLDays() int {
	raw := strings.TrimSpace(os.Getenv("SITE_TTL_DAYS"))
	if raw == "" {
		return defaultSiteTTLDays
	}
	days, err := strconv.Atoi(raw)
	if err != nil || days < 1 || days > 30 {
		return defaultSiteTTLDays
	}
	return days
}

func siteTTL() time.Duration {
	return time.Duration(siteTTLDays()) * 24 * time.Hour
}

func expiryFor(createdAt time.Time) time.Time {
	if createdAt.IsZero() {
		return time.Time{}
	}
	return createdAt.Add(siteTTL())
}

func maxProjects() int {
	raw := strings.TrimSpace(os.Getenv("MAX_PROJECTS"))
	if raw == "" {
		return 180
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 180
	}
	return value
}
