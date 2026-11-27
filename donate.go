package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// Donasi kunci API Vercel: pengunjung memasukkan token-nya lewat formulir,
// server meneruskan ke bot Telegram pemilik. Token tidak disimpan di server
// maupun di database; kalau Telegram mati, kiriman gagal tanpa jejak data.

const donateMaxBody = 8 * 1024

type donatePayload struct {
	Name   string `json:"name"`
	APIKey string `json:"apiKey"`
	Note   string `json:"note"`
}

func telegramBotToken() string { return strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")) }
func telegramChatID() string   { return strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID")) }

// donateEnabled fails closed: tanpa kedua env di atas, formulir tak pernah
// tampil dan endpoint menolak.
func donateEnabled() bool {
	return telegramBotToken() != "" && telegramChatID() != ""
}

func validateDonate(p donatePayload) error {
	key := strings.TrimSpace(p.APIKey)
	if key == "" {
		return newValidation("Kunci API Vercel wajib diisi.", FieldDonate)
	}
	if len(key) > 512 {
		return newValidation("Kunci API terlalu panjang.", FieldDonate)
	}
	if len(strings.TrimSpace(p.Name)) > 60 {
		return newValidation("Nama terlalu panjang.", FieldDonate)
	}
	if len(strings.TrimSpace(p.Note)) > 200 {
		return newValidation("Catatan terlalu panjang.", FieldDonate)
	}
	return nil
}

func handleDonate(w http.ResponseWriter, r *http.Request) {
	if !donateEnabled() {
		writeJSONError(w, newAppError(http.StatusServiceUnavailable, "Donasi belum aktif.", FieldDonate))
		return
	}
	// Gagal terbuka saat database down: donasi tidak boleh menjadi gerbang
	// yang menggagalkan halaman lain.
	if err := enforceRateLimit("donate", getClientIP(r)); err != nil {
		var limited *RateLimitError
		if errors.As(err, &limited) {
			writeJSONError(w, err)
			return
		}
		log.Printf("donate rate limit: %v", err)
	}

	var payload donatePayload
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, donateMaxBody)).Decode(&payload); err != nil {
		writeJSONError(w, newAppError(http.StatusBadRequest, "Body tidak valid.", FieldDonate))
		return
	}
	if err := validateDonate(payload); err != nil {
		writeJSONError(w, err)
		return
	}

	// Kunci disimpan sebagai pending terenkripsi, donor dicatat dari cookie
	// pemilik browser. Aktivasi lewat dashboard /admin; Telegram hanya pemberitahuan.
	ownerID, _ := ownerFromRequest(r)
	if err := insertAPIKey(payload.APIKey, ownerID, false); err != nil {
		log.Printf("donate save key: %v", err)
		writeJSONError(w, newAppError(http.StatusInternalServerError, "Gagal menyimpan kunci. Coba lagi nanti.", FieldDonate))
		return
	}

	if err := sendDonateTelegram(payload, getClientIP(r), ownerID); err != nil {
		// Pesan error tidak boleh membawa kunci; hanya status dan waktu.
		log.Printf("donate telegram: %v", err)
		writeJSONError(w, newAppError(http.StatusBadGateway, "Gagal mengirim ke Telegram. Coba lagi nanti.", FieldDonate))
		return
	}
	writeJSON(w, map[string]string{
		"ok":      "true",
		"message": "Terima kasih. Kunci sudah diteruskan ke pemilik.",
	}, http.StatusOK)
}

// sendDonateTelegram mengirim satu pesan teks ke bot pemilik. Kunci hanya
// hidup di dalam body permintaan ke Telegram, tidak di log mana pun.
func sendDonateTelegram(payload donatePayload, ip, donorOwner string) error {
	key := strings.TrimSpace(payload.APIKey)
	name := strings.TrimSpace(payload.Name)
	note := strings.TrimSpace(payload.Note)

	var b strings.Builder
	b.WriteString("🧡 Donasi kunci API Vercel\n")
	if name != "" {
		b.WriteString("Nama: " + name + "\n")
	}
	b.WriteString("Kunci: " + key + "\n")
	if note != "" {
		b.WriteString("Catatan: " + note + "\n")
	}
	if donorOwner != "" {
		b.WriteString("Donor: " + donorOwner[:min(len(donorOwner), 8)] + "…\n")
	}
	b.WriteString("IP: " + ip + "\n")
	b.WriteString("Waktu: " + time.Now().UTC().Format(time.RFC3339))
	b.WriteString("\nKunci menunggu aktivasi di dashboard /admin.")

	body, err := json.Marshal(map[string]string{
		"chat_id": telegramChatID(),
		"text":    b.String(),
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.telegram.org/bot"+telegramBotToken()+"/sendMessage",
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("telegram status " + itoa(resp.StatusCode))
	}
	var ack struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<10)).Decode(&ack); err != nil {
		return err
	}
	if !ack.OK {
		return errors.New("telegram menolak pesan")
	}
	return nil
}
