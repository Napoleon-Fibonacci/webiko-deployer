package main

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestOwnerCookieRoundTrip(t *testing.T) {
	t.Setenv("OWNER_COOKIE_SECRET", "test-owner-secret")
	id := newOwnerID()

	parsed, ok := parseOwner(signOwner(id))
	if !ok || parsed != id {
		t.Fatalf("parseOwner(signOwner(%q)) = %q, %v", id, parsed, ok)
	}
}

func TestOwnerCookieRejectsTampering(t *testing.T) {
	t.Setenv("OWNER_COOKIE_SECRET", "test-owner-secret")
	id := newOwnerID()
	token := signOwner(id)

	parts := strings.Split(token, ".")
	tampered := strings.Join([]string{newOwnerID(), parts[1], parts[2]}, ".")
	if _, ok := parseOwner(tampered); ok {
		t.Fatal("expected a changed owner id to fail")
	}

	tampered = token[:len(token)-1] + "x"
	if _, ok := parseOwner(tampered); ok {
		t.Fatal("expected a changed signature to fail")
	}
}

func TestOwnerCookieRejectsExpiredAndMalformed(t *testing.T) {
	t.Setenv("OWNER_COOKIE_SECRET", "test-owner-secret")
	id := newOwnerID()
	expires := strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10)
	payload := id + "." + expires
	expired := payload + "." + ownerSignature(payload)
	if _, ok := parseOwner(expired); ok {
		t.Fatal("expected an expired cookie to fail")
	}

	for _, value := range []string{"", "a.b", "a.b.c.d", "short.123.sig", "not-a-cookie"} {
		if _, ok := parseOwner(value); ok {
			t.Fatalf("parseOwner(%q) unexpectedly passed", value)
		}
	}
}

func TestEnsureOwnerSetsCookieAndReusesIt(t *testing.T) {
	t.Setenv("OWNER_COOKIE_SECRET", "test-owner-secret")

	firstReq := httptest.NewRequest("POST", "/api/deploy", nil)
	firstRec := httptest.NewRecorder()
	firstID := ensureOwner(firstRec, firstReq)
	if firstID == "" {
		t.Fatal("expected a new owner id")
	}

	cookies := firstRec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != ownerCookie {
		t.Fatalf("expected one %s cookie, got %v", ownerCookie, cookies)
	}
	if !cookies[0].HttpOnly {
		t.Fatal("owner cookie must be HttpOnly")
	}

	secondReq := httptest.NewRequest("POST", "/api/deploy", nil)
	secondReq.AddCookie(cookies[0])
	secondRec := httptest.NewRecorder()
	secondID := ensureOwner(secondRec, secondReq)
	if secondID != firstID {
		t.Fatalf("ensureOwner changed the id: %q -> %q", firstID, secondID)
	}
}

func TestOwnerLimitConfig(t *testing.T) {
	tests := []struct {
		value string
		want  int
	}{
		{value: "", want: defaultMaxSitesPerOwner},
		{value: "1", want: 1},
		{value: "3", want: 3},
		{value: "0", want: defaultMaxSitesPerOwner},
		{value: "11", want: defaultMaxSitesPerOwner},
		{value: "abc", want: defaultMaxSitesPerOwner},
	}
	for _, test := range tests {
		t.Setenv("MAX_SITES_PER_OWNER", test.value)
		if got := maxSitesPerOwner(); got != test.want {
			t.Fatalf("maxSitesPerOwner(%q) = %d, want %d", test.value, got, test.want)
		}
	}
}

func TestMinEvictHoursConfig(t *testing.T) {
	tests := []struct {
		value string
		want  int
	}{
		{value: "", want: defaultMinEvictHours},
		{value: "1", want: 1},
		{value: "48", want: 48},
		{value: "0", want: defaultMinEvictHours},
		{value: "169", want: defaultMinEvictHours},
		{value: "abc", want: defaultMinEvictHours},
	}
	for _, test := range tests {
		t.Setenv("MIN_EVICT_HOURS", test.value)
		if got := minEvictHours(); got != test.want {
			t.Fatalf("minEvictHours(%q) = %d, want %d", test.value, got, test.want)
		}
	}
}
