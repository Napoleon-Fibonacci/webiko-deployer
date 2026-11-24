package main

import (
	"crypto/subtle"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type pageData struct {
	Site    Site
	Used    int
	HasUsed bool
	Max     int
	Rows    []ActiveRow
	JSONLD  template.JS
}

type statusStat struct {
	Key   string
	Label string
	Count int
}

type flashMessage struct {
	Tone string
	Text string
}

type adminData struct {
	Site       Site
	Rows       []AdminRow
	Stats      []statusStat
	Total      int
	Matched    int
	Status     string
	Query      string
	Flash      *flashMessage
	Limit      int
	AdminError bool
}

type loginData struct {
	Site        Site
	Disabled    bool
	Error       string
	SessionTime int
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	used := 0
	hasUsed := false
	if value, err := countLive(); err == nil {
		used = value
		hasUsed = true
	}
	rows, err := recentActive(20)
	if err != nil {
		rows = nil
	}

	jsonLD := map[string]any{
		"@context": "https://schema.org",
		"@graph": []any{
			map[string]any{
				"@type":       "WebSite",
				"@id":         site.URL + "/#website",
				"url":         site.URL + "/",
				"name":        site.Name,
				"description": site.Description,
				"inLanguage":  "id-ID",
			},
			map[string]any{
				"@type":               "SoftwareApplication",
				"@id":                 site.URL + "/#app",
				"name":                site.Name,
				"url":                 site.URL + "/",
				"description":         site.Description,
				"applicationCategory": "DeveloperApplication",
				"operatingSystem":     "Web",
				"isAccessibleForFree": true,
				"offers": map[string]any{
					"@type":         "Offer",
					"price":         "0",
					"priceCurrency": "IDR",
				},
			},
		},
	}

	w.Header().Set("Cache-Control", "public, s-maxage=30, stale-while-revalidate=60")
	render(w, "index.html", pageData{
		Site:    site,
		Used:    used,
		HasUsed: hasUsed,
		Max:     maxProjects(),
		Rows:    rows,
		JSONLD:  jsonForScript(jsonLD),
	}, http.StatusOK)
}

func handleAdmin(w http.ResponseWriter, r *http.Request) {
	if !adminEnabled() {
		http.Redirect(w, r, "/admin/login?error=off", http.StatusSeeOther)
		return
	}
	if !isAdmin(r) {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}

	query := r.URL.Query()
	status := query.Get("status")
	if !isDeployStatus(status) {
		status = ""
	}
	search := strings.TrimSpace(query.Get("q"))

	list, err := listForAdmin(AdminFilter{Status: status, Query: search}, 100)
	data := adminData{
		Site:       site,
		Status:     status,
		Query:      search,
		Limit:      100,
		Flash:      flashFromQuery(query),
		AdminError: err != nil,
	}
	if err == nil {
		data.Rows = list.Rows
		data.Total = list.Total
		data.Matched = list.Matched
		data.Stats = []statusStat{{Key: "", Label: "Semua", Count: list.Total}}
		for _, key := range deployStatuses {
			data.Stats = append(data.Stats, statusStat{Key: key, Label: statusLabel[key], Count: list.Counts[key]})
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	render(w, "admin.html", data, http.StatusOK)
}

func handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if !adminEnabled() {
		w.Header().Set("Cache-Control", "no-store")
		render(w, "login.html", loginData{Site: site, Disabled: true}, http.StatusOK)
		return
	}
	if isAdmin(r) {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	render(w, "login.html", loginData{
		Site:        site,
		Error:       r.URL.Query().Get("error"),
		SessionTime: int(adminMaxAge.Hours()),
	}, http.StatusOK)
}

func handleAdminLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if !adminEnabled() {
		http.Redirect(w, r, "/admin/login?error=off", http.StatusSeeOther)
		return
	}
	if err := enforceRateLimit("adminLogin", getClientIP(r)); err != nil {
		code := "db"
		if errCode := errorCode(err); errCode == http.StatusTooManyRequests {
			code = "limit"
		}
		http.Redirect(w, r, "/admin/login?error="+code, http.StatusSeeOther)
		return
	}
	_ = r.ParseForm()
	if !checkPassword(r.PostFormValue("password")) {
		http.Redirect(w, r, "/admin/login?error=1", http.StatusSeeOther)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookie,
		Value:    createAdminToken(),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsHTTPS(r),
		MaxAge:   int(adminMaxAge.Seconds()),
	})
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsHTTPS(r),
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func handleAdminDelete(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	_ = r.ParseForm()
	id := strings.TrimSpace(r.PostFormValue("id"))
	target := "/admin?missing=1"
	if id != "" {
		result, err := deleteOne(id)
		switch {
		case err == nil && result.Outcome == "done":
			target = "/admin?removed=" + url.QueryEscape(result.Name)
		case err == nil:
			target = "/admin?retry=" + url.QueryEscape(result.Name)
		case errorCode(err) == http.StatusNotFound:
			target = "/admin?missing=1"
		default:
			target = "/admin?fail=1"
		}
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func handleAdminSweep(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	result, err := sweepStale(10)
	if err != nil {
		http.Redirect(w, r, "/admin?swept=0", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin?swept=1&failed="+strconv.FormatInt(result.Failed, 10)+
		"&deleted="+itoa(result.Deleted)+"&gone="+itoa(result.Gone), http.StatusSeeOther)
}

func handleCron(w http.ResponseWriter, r *http.Request) {
	authorized := false
	secret := strings.TrimSpace(os.Getenv("CRON_SECRET"))
	if secret != "" {
		expected := "Bearer " + secret
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(expected)) == 1 {
			authorized = true
		}
	}
	if !authorized && isAdmin(r) && sameOrigin(r) {
		authorized = true
	}
	if !authorized {
		writeJSON(w, map[string]string{"error": "Tidak diizinkan."}, http.StatusUnauthorized)
		return
	}
	result, err := sweepStale(10)
	if err != nil {
		writeJSON(w, map[string]string{"error": errorMessage(err)}, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"ok":        true,
		"failed":    result.Failed,
		"deleted":   result.Deleted,
		"gone":      result.Gone,
		"remaining": result.Remaining,
	}, http.StatusOK)
}

func handleRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("User-Agent: *\nAllow: /\nDisallow: /api/\nDisallow: /admin\nDisallow: /admin/\n\nSitemap: " + site.URL + "/sitemap.xml\nHost: " + site.URL + "\n"))
}

func handleSitemap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>` + site.URL + `/</loc>
    <lastmod>` + time.Now().Format("2006-01-02") + `</lastmod>
    <changefreq>weekly</changefreq>
    <priority>1.0</priority>
  </url>
</urlset>
`))
}

func handleManifest(w http.ResponseWriter, r *http.Request) {
	payload := map[string]any{
		"name":             site.Title,
		"short_name":       site.Name,
		"description":      site.Description,
		"start_url":        "/",
		"scope":            "/",
		"display":          "standalone",
		"background_color": site.ThemeColor,
		"theme_color":      site.ThemeColor,
		"lang":             site.Lang,
		"icons": []map[string]string{
			{"src": "/icon.svg", "sizes": "any", "type": "image/svg+xml", "purpose": "any"},
		},
	}
	writeJSON(w, payload, http.StatusOK)
}

func isDeployStatus(value string) bool {
	for _, status := range deployStatuses {
		if value == status {
			return true
		}
	}
	return false
}

func flashFromQuery(query url.Values) *flashMessage {
	if query.Get("swept") == "1" {
		gone := 0
		if value := query.Get("gone"); value != "" && value != "0" {
			gone, _ = strconv.Atoi(value)
		}
		text := "Sapuan selesai: " + defaultZero(query.Get("failed")) + " pending basi ditandai gagal, " +
			defaultZero(query.Get("deleted")) + " project dihapus di Vercel."
		if gone > 0 {
			text += " " + itoa(gone) + " baris ditutup karena project-nya sudah tak ada di Vercel."
		}
		return &flashMessage{Tone: "ok", Text: text}
	}
	if query.Get("swept") == "0" {
		return &flashMessage{Tone: "err", Text: "Sapuan gagal dijalankan. Coba lagi."}
	}
	if removed := query.Get("removed"); removed != "" {
		return &flashMessage{Tone: "ok", Text: removed + " dihapus: project di Vercel dan barisnya beres."}
	}
	if retry := query.Get("retry"); retry != "" {
		return &flashMessage{Tone: "err", Text: retry + " gagal dihapus di Vercel. Barisnya tetap terkunci sampai sapuan berikutnya berhasil."}
	}
	if query.Get("missing") == "1" {
		return &flashMessage{Tone: "err", Text: "Baris itu sudah tidak ada."}
	}
	if query.Get("fail") == "1" {
		return &flashMessage{Tone: "err", Text: "Hapus gagal dijalankan. Coba lagi."}
	}
	return nil
}

func defaultZero(value string) string {
	if value == "" || value == "0" {
		return "0"
	}
	return value
}
