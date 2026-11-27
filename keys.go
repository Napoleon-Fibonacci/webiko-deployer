package main

import (
	"net/http"
	"strings"
)

// Dashboard admin untuk pool kunci API Vercel: donasi masuk sebagai pending
// dari banner halaman depan, pemilik mengaktifkannya dari sini. Semua aksi
// butuh sesi admin dan origin yang sama, seperti menu admin lainnya.

func adminGate(w http.ResponseWriter, r *http.Request) bool {
	if isAdmin(r) {
		return true
	}
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
	return false
}

func handleAdminKeyToggle(w http.ResponseWriter, r *http.Request) {
	if !adminGate(w, r) {
		return
	}
	_ = r.ParseForm()
	id := strings.TrimSpace(r.PostFormValue("id"))
	nowActive := r.PostFormValue("active") != "true"
	target := "/admin?keys=1"
	if id == "" || setAPIKeyActive(id, nowActive) != nil {
		target = "/admin?keys=0"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func handleAdminKeyDelete(w http.ResponseWriter, r *http.Request) {
	if !adminGate(w, r) {
		return
	}
	_ = r.ParseForm()
	id := strings.TrimSpace(r.PostFormValue("id"))
	target := "/admin?keys=1"
	if id == "" || deleteAPIKey(id) != nil {
		target = "/admin?keys=0"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func handleAdminKeyAdd(w http.ResponseWriter, r *http.Request) {
	if !adminGate(w, r) {
		return
	}
	_ = r.ParseForm()
	key := strings.TrimSpace(r.PostFormValue("apiKey"))
	target := "/admin?keys=1"
	if len(key) < 8 || len(key) > 512 || insertAPIKey(key, "", true) != nil {
		target = "/admin?keys=0"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}
