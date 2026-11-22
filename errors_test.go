package main

import (
	"errors"
	"net/http"
	"testing"
)

func TestErrorCodesAndFields(t *testing.T) {
	cases := []struct {
		err   error
		code  int
		field ErrorField
	}{
		{newValidation("rusak", FieldName), http.StatusBadRequest, FieldName},
		{newNameTaken(), http.StatusConflict, FieldName},
		{newVercelAPI("gagal", 0), http.StatusBadGateway, FieldSource},
		{newRateLimit("lambat"), http.StatusTooManyRequests, FieldSource},
		{errors.New("lain"), http.StatusInternalServerError, FieldSource},
	}
	for _, tc := range cases {
		if got := errorCode(tc.err); got != tc.code {
			t.Errorf("%T code = %d, want %d", tc.err, got, tc.code)
		}
		if got := errorField(tc.err); got != tc.field {
			t.Errorf("%T field = %q, want %q", tc.err, got, tc.field)
		}
	}
}
