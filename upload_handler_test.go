package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

const testBlobToken = "vercel_blob_rw_store123_secret456"

func TestGenerateBlobClientToken(t *testing.T) {
	t.Setenv("BLOB_READ_WRITE_TOKEN", testBlobToken)

	token, err := generateBlobClientToken(blobTokenRequest{
		Pathname:    "uploads/contoh.zip",
		CallbackURL: "https://example.com/api/upload",
	})
	if err != nil {
		t.Fatalf("generateBlobClientToken: %v", err)
	}
	if !strings.HasPrefix(token, "vercel_blob_client_store123_") {
		t.Fatalf("unexpected token prefix: %q", token)
	}

	encoded := strings.TrimPrefix(token, "vercel_blob_client_store123_")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("token body is not base64: %v", err)
	}
	parts := strings.SplitN(string(decoded), ".", 2)
	if len(parts) != 2 {
		t.Fatalf("token body should be signature.payload: %q", decoded)
	}

	mac := hmac.New(sha256.New, []byte(testBlobToken))
	mac.Write([]byte(parts[1]))
	if hex.EncodeToString(mac.Sum(nil)) != parts[0] {
		t.Fatalf("token signature does not match payload")
	}

	payloadJSON, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("payload is not base64: %v", err)
	}
	var payload struct {
		Pathname            string   `json:"pathname"`
		AllowedContentTypes []string `json:"allowedContentTypes"`
		MaximumSizeInBytes  int      `json:"maximumSizeInBytes"`
		AddRandomSuffix     bool     `json:"addRandomSuffix"`
	}
	if err = json.Unmarshal(payloadJSON, &payload); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	if payload.Pathname != "uploads/contoh.zip" {
		t.Errorf("pathname = %q", payload.Pathname)
	}
	if payload.MaximumSizeInBytes != maxZipBytes {
		t.Errorf("maximumSizeInBytes = %d", payload.MaximumSizeInBytes)
	}
	if !payload.AddRandomSuffix {
		t.Errorf("addRandomSuffix should be true")
	}
	if len(payload.AllowedContentTypes) == 0 {
		t.Errorf("allowedContentTypes should not be empty")
	}
}

func TestGenerateBlobClientTokenNeedsStoreID(t *testing.T) {
	t.Setenv("BLOB_READ_WRITE_TOKEN", "rusak")
	if _, err := generateBlobClientToken(blobTokenRequest{Pathname: "uploads/a.zip"}); err == nil {
		t.Fatalf("invalid read-write token should fail")
	}
}

func TestVerifyBlobCallback(t *testing.T) {
	t.Setenv("BLOB_READ_WRITE_TOKEN", testBlobToken)
	body := []byte(`{"type":"blob.upload-completed"}`)
	mac := hmac.New(sha256.New, []byte(testBlobToken))
	mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))

	if !verifyBlobCallback(body, signature) {
		t.Fatalf("valid signature rejected")
	}
	if verifyBlobCallback(body, "") {
		t.Fatalf("empty signature accepted")
	}
	if verifyBlobCallback([]byte(`{"type":"lain"}`), signature) {
		t.Fatalf("signature for another body accepted")
	}
}
