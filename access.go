package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
)

// deployAccessCode returns the optional shared deploy code. Empty means the
// deployer stays open to everyone, as before.
func deployAccessCode() string {
	return strings.TrimSpace(os.Getenv("DEPLOY_ACCESS_CODE"))
}

func accessCodeRequired() bool {
	return deployAccessCode() != ""
}

// accessCodeMatches hashes both sides so the comparison is constant time even
// when the submitted code has a different length.
func accessCodeMatches(given, want string) bool {
	if want == "" {
		return true
	}
	givenHash := sha256.Sum256([]byte(strings.TrimSpace(given)))
	wantHash := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(givenHash[:], wantHash[:]) == 1
}

// checkDeployAccess fails closed when a code is configured. Wrong attempts are
// rate-limited per IP so the gate cannot be brute-forced cheaply.
func checkDeployAccess(given, ip string) error {
	want := deployAccessCode()
	if want == "" || accessCodeMatches(given, want) {
		return nil
	}
	// Keep the 403 even if the quota table is unavailable: the gate must fail
	// closed. The limiter resumes working as soon as the database is back.
	if err := enforceRateLimit("access", ip); err != nil {
		var limited *RateLimitError
		if errors.As(err, &limited) {
			return err
		}
		log.Printf("access code rate limit: %v", err)
	}
	return newAppError(http.StatusForbidden, "Kode akses salah atau kosong.", FieldAccess)
}
