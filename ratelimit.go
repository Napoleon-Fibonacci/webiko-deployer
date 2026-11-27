package main

import (
	"context"
	"net/http"
	"time"
)

type rateLimitPolicy struct {
	Max     int
	Window  time.Duration
	Message string
}

var rateLimits = map[string]rateLimitPolicy{
	"deploy": {
		Max:     5,
		Window:  time.Hour,
		Message: "Terlalu banyak deploy. Coba lagi dalam satu jam.",
	},
	"upload": {
		Max:     20,
		Window:  time.Hour,
		Message: "Terlalu banyak unggahan. Coba lagi dalam satu jam.",
	},
	"adminLogin": {
		Max:     10,
		Window:  15 * time.Minute,
		Message: "Terlalu banyak percobaan masuk. Coba lagi nanti.",
	},
	"access": {
		Max:     10,
		Window:  time.Hour,
		Message: "Terlalu banyak percobaan kode akses. Coba lagi dalam satu jam.",
	},
	"donate": {
		Max:     5,
		Window:  time.Hour,
		Message: "Terlalu banyak kiriman donasi. Coba lagi dalam satu jam.",
	},
}

func windowStartFor(scope string, now time.Time) time.Time {
	policy := rateLimits[scope]
	return time.UnixMilli((now.UnixMilli() / policy.Window.Milliseconds()) * policy.Window.Milliseconds()).UTC()
}

// enforceRateLimit records one attempt and throws once the window is over.
func enforceRateLimit(scope, identifier string) error {
	policy, ok := rateLimits[scope]
	if !ok {
		return newAppError(http.StatusInternalServerError, "Scope rate limit tidak dikenal.", FieldSource)
	}
	db, err := getDB()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var count int
	err = db.QueryRowContext(ctx,
		`INSERT INTO rate_limits (scope, ip, window_start, count)
		 VALUES ($1, $2, $3, 1)
		 ON CONFLICT (scope, ip, window_start)
		 DO UPDATE SET count = rate_limits.count + 1
		 RETURNING count`,
		scope, identifier, windowStartFor(scope, time.Now()),
	).Scan(&count)
	if err != nil {
		return err
	}
	if count > policy.Max {
		return newRateLimit(policy.Message)
	}
	return nil
}
