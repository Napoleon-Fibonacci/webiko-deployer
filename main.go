package main

import (
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

//go:embed web views
var content embed.FS

var templates = template.Must(
	template.New("").Funcs(templateFuncs()).ParseFS(content, "views/*.html"),
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", handleHome)
	mux.HandleFunc("GET /admin", handleAdmin)
	mux.HandleFunc("GET /admin/{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /admin/login", handleAdminLogin)
	mux.HandleFunc("GET /donasi", handleDonatePage)
	mux.HandleFunc("GET /admin/login/{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/login", http.StatusMovedPermanently)
	})

	mux.HandleFunc("POST /api/deploy", handleDeploy)
	mux.HandleFunc("POST /api/upload", handleUpload)
	mux.HandleFunc("GET /api/check-name", handleCheckName)
	mux.HandleFunc("POST /api/admin/login", requireSameOrigin(handleAdminLoginSubmit))
	mux.HandleFunc("POST /api/admin/logout", requireSameOrigin(handleAdminLogout))
	mux.HandleFunc("POST /api/admin/delete", requireSameOrigin(handleAdminDelete))
	mux.HandleFunc("POST /api/admin/sweep", requireSameOrigin(handleAdminSweep))
	mux.HandleFunc("POST /api/admin/key/toggle", requireSameOrigin(handleAdminKeyToggle))
	mux.HandleFunc("POST /api/admin/key/delete", requireSameOrigin(handleAdminKeyDelete))
	mux.HandleFunc("POST /api/admin/key/add", requireSameOrigin(handleAdminKeyAdd))
	mux.HandleFunc("POST /api/donate", requireSameOrigin(handleDonate))
	mux.HandleFunc("GET /api/cron/cleanup", handleCron)
	mux.HandleFunc("POST /api/cron/cleanup", handleCron)

	mux.HandleFunc("GET /robots.txt", handleRobots)
	mux.HandleFunc("GET /sitemap.xml", handleSitemap)
	mux.HandleFunc("GET /manifest.webmanifest", handleManifest)
	mux.HandleFunc("GET /icon.svg", serveEmbedded("web/icon.svg", "image/svg+xml"))
	mux.HandleFunc("GET /icon-32.png", serveEmbedded("web/icon-32.png", "image/png"))
	mux.HandleFunc("GET /icon-192.png", serveEmbedded("web/icon-192.png", "image/png"))
	mux.HandleFunc("GET /icon-512.png", serveEmbedded("web/icon-512.png", "image/png"))
	mux.HandleFunc("GET /og.png", serveEmbedded("web/og.png", "image/png"))
	mux.HandleFunc("GET /apple-icon.png", serveEmbedded("web/apple-icon.png", "image/png"))
	mux.HandleFunc("GET /favicon.ico", serveEmbedded("web/favicon.ico", "image/x-icon"))

	webFS, err := fs.Sub(content, "web")
	if err != nil {
		log.Fatal(err)
	}
	static := http.StripPrefix("/static/", withCache(http.FileServer(http.FS(webFS)), "public, max-age=300"))
	mux.Handle("GET /static/", static)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           withSecurityHeaders(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      300 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	log.Printf("Webiko Deployer listening on :%s", port)
	log.Fatal(server.ListenAndServe())
}

func serveEmbedded(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := content.ReadFile(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(data)
	}
}

func withCache(next http.Handler, value string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", value)
		next.ServeHTTP(w, r)
	})
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'; base-uri 'self'; form-action 'self'; object-src 'none'")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("X-Permitted-Cross-Domain-Policies", "none")
		next.ServeHTTP(w, r)
	})
}

func render(w http.ResponseWriter, name string, data any, status int) {
	var builder strings.Builder
	if err := templates.ExecuteTemplate(&builder, name, data); err != nil {
		log.Printf("template %s: %v", name, err)
		http.Error(w, "Terjadi kesalahan tak terduga.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(builder.String()))
}

func writeJSON(w http.ResponseWriter, value any, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
