package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateDonate(t *testing.T) {
	cases := []struct {
		name    string
		payload donatePayload
		wantErr bool
	}{
		{"kunci kosong", donatePayload{}, true},
		{"kunci terisi", donatePayload{APIKey: "abc123"}, false},
		{"nama panjang", donatePayload{APIKey: "abc123", Name: strings.Repeat("a", 61)}, true},
		{"catatan panjang", donatePayload{APIKey: "abc123", Note: strings.Repeat("a", 201)}, true},
		{"kunci panjang", donatePayload{APIKey: strings.Repeat("x", 513)}, true},
	}
	for _, tc := range cases {
		err := validateDonate(tc.payload)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err=%v wantErr=%v", tc.name, err, tc.wantErr)
		}
	}
}

func TestDonateEnabled(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	t.Setenv("TELEGRAM_CHAT_ID", "")
	if donateEnabled() {
		t.Fatal("donateEnabled() = true, want false saat env kosong")
	}
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	if donateEnabled() {
		t.Fatal("donateEnabled() = true tanpa chat id, want false")
	}
	t.Setenv("TELEGRAM_CHAT_ID", "123")
	if !donateEnabled() {
		t.Fatal("donateEnabled() = false dengan dua env terisi, want true")
	}
}

func TestHandleDonateDisabled(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	t.Setenv("TELEGRAM_CHAT_ID", "")
	req := httptest.NewRequest("POST", "/api/donate", strings.NewReader(`{"apiKey":"x"}`))
	rec := httptest.NewRecorder()
	handleDonate(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestHandleDonateInvalidBody(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_CHAT_ID", "123")
	req := httptest.NewRequest("POST", "/api/donate", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	handleDonate(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleDonatePage(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_CHAT_ID", "123")
	req := httptest.NewRequest("GET", "/donasi", nil)
	rec := httptest.NewRecorder()
	handleDonatePage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `id="donate-form"`) {
		t.Fatal("formulir donasi tidak tampil di /donasi")
	}
}

func TestHandleDonatePageDisabled(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	t.Setenv("TELEGRAM_CHAT_ID", "")
	req := httptest.NewRequest("GET", "/donasi", nil)
	rec := httptest.NewRecorder()
	handleDonatePage(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
}
