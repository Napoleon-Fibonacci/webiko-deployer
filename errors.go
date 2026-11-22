package main

import (
	"encoding/json"
	"errors"
	"net/http"
)

// ErrorField is the form field a failure belongs to.
type ErrorField string

const (
	FieldName   ErrorField = "name"
	FieldSource ErrorField = "source"
)

// AppError carries an HTTP status and the form field to highlight, so no
// caller has to guess from the message text.
type AppError struct {
	Code    int
	Message string
	Field   ErrorField
}

func (e *AppError) Error() string { return e.Message }

func newAppError(code int, message string, field ErrorField) *AppError {
	return &AppError{Code: code, Message: message, Field: field}
}

// ValidationError is always a bad request and points at the source by default.
type ValidationError struct{ *AppError }

func newValidation(message string, field ErrorField) *ValidationError {
	return &ValidationError{newAppError(http.StatusBadRequest, message, field)}
}

func (e *ValidationError) Unwrap() error { return e.AppError }

// NameTakenError means a live row already owns the name.
type NameTakenError struct{ *AppError }

func newNameTaken() *NameTakenError {
	return &NameTakenError{newAppError(http.StatusConflict, nameTakenMessage, FieldName)}
}

func (e *NameTakenError) Unwrap() error { return e.AppError }

// VercelAPIError is a failure on Vercel's side; default 502.
type VercelAPIError struct{ *AppError }

func newVercelAPI(message string, status int) *VercelAPIError {
	if status == 0 {
		status = http.StatusBadGateway
	}
	return &VercelAPIError{newAppError(status, message, FieldSource)}
}

func (e *VercelAPIError) Unwrap() error { return e.AppError }

// RateLimitError is 429 with the policy message.
type RateLimitError struct{ *AppError }

func newRateLimit(message string) *RateLimitError {
	return &RateLimitError{newAppError(http.StatusTooManyRequests, message, FieldSource)}
}

func (e *RateLimitError) Unwrap() error { return e.AppError }

const (
	nameTakenMessage = "Nama ini sudah dipakai. Coba nama lain."
	genericMessage   = "Terjadi kesalahan tak terduga."
)

func errorCode(err error) int {
	var app *AppError
	if errors.As(err, &app) {
		return app.Code
	}
	return http.StatusInternalServerError
}

func errorField(err error) ErrorField {
	var app *AppError
	if errors.As(err, &app) {
		return app.Field
	}
	return FieldSource
}

func errorMessage(err error) string {
	if err == nil {
		return genericMessage
	}
	if message := err.Error(); message != "" {
		return message
	}
	return genericMessage
}

func writeJSONError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(errorCode(err))
	_ = json.NewEncoder(w).Encode(map[string]string{"error": errorMessage(err)})
}
