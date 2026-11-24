package main

import (
	"os"
	"strconv"
	"strings"
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
			"Sumber ZIP atau repo GitHub, nama dicek otomatis, project terlama dibersihkan otomatis.",
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
