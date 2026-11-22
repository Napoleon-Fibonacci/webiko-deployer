package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type blobEnvelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type blobTokenRequest struct {
	Pathname      string          `json:"pathname"`
	CallbackURL   string          `json:"callbackUrl"`
	ClientPayload json.RawMessage `json:"clientPayload"`
	Multipart     bool            `json:"multipart"`
}

// handleUpload speaks the Vercel Blob client-upload protocol: the browser asks
// for a scoped token here, uploads the ZIP straight to Blob, and Blob calls
// back with blob.upload-completed.
func handleUpload(w http.ResponseWriter, r *http.Request) {
	if err := enforceRateLimit("upload", getClientIP(r)); err != nil {
		writeJSONError(w, err)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeJSONError(w, newAppError(http.StatusBadRequest, "Body tidak valid.", FieldSource))
		return
	}
	var envelope blobEnvelope
	if err = json.Unmarshal(body, &envelope); err != nil {
		writeJSONError(w, newAppError(http.StatusBadRequest, "Body tidak valid.", FieldSource))
		return
	}

	switch envelope.Type {
	case "blob.generate-client-token":
		var request blobTokenRequest
		if err = json.Unmarshal(envelope.Payload, &request); err != nil {
			writeJSONError(w, newAppError(http.StatusBadRequest, "Body tidak valid.", FieldSource))
			return
		}
		if !strings.HasPrefix(request.Pathname, "uploads/") || !isZipName(request.Pathname) {
			writeJSONError(w, newAppError(http.StatusBadRequest, "Nama berkas unggahan tidak valid.", FieldSource))
			return
		}
		token, tokenErr := generateBlobClientToken(request)
		if tokenErr != nil {
			writeJSONError(w, newAppError(http.StatusBadRequest, errorMessage(tokenErr), FieldSource))
			return
		}
		writeJSON(w, map[string]string{
			"type":        "blob.generate-client-token",
			"clientToken": token,
		}, http.StatusOK)

	case "blob.upload-completed":
		signature := r.Header.Get("X-Vercel-Signature")
		if !verifyBlobCallback(body, signature) {
			writeJSONError(w, newAppError(http.StatusBadRequest, "Tanda tangan unggahan tidak valid.", FieldSource))
			return
		}
		// The deploy route verifies the blob host and contents again.
		writeJSON(w, map[string]string{
			"type":     "blob.upload-completed",
			"response": "ok",
		}, http.StatusOK)

	default:
		writeJSONError(w, newAppError(http.StatusBadRequest, "Jenis unggahan tidak dikenal.", FieldSource))
	}
}

func generateBlobClientToken(request blobTokenRequest) (string, error) {
	readWriteToken := strings.TrimSpace(os.Getenv("BLOB_READ_WRITE_TOKEN"))
	if readWriteToken == "" {
		return "", newAppError(http.StatusBadRequest, "BLOB_READ_WRITE_TOKEN belum diset di server.", FieldSource)
	}
	parts := strings.Split(readWriteToken, "_")
	if len(parts) < 4 || parts[3] == "" {
		return "", newAppError(http.StatusBadRequest, "BLOB_READ_WRITE_TOKEN tidak valid.", FieldSource)
	}
	storeID := parts[3]

	tokenPayload := string(request.ClientPayload)
	if tokenPayload == "null" {
		tokenPayload = ""
	}
	payload := map[string]any{
		"pathname":            request.Pathname,
		"onUploadCompleted":   map[string]any{"callbackUrl": request.CallbackURL, "tokenPayload": tokenPayload},
		"allowedContentTypes": append(append([]string{}, zipContentTypes...), "application/octet-stream"),
		"maximumSizeInBytes":  maxZipBytes,
		"addRandomSuffix":     true,
		"tokenPayload":        tokenPayload,
		"validUntil":          time.Now().Add(time.Hour).UnixMilli(),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	payloadBase64 := base64.StdEncoding.EncodeToString(payloadJSON)
	mac := hmac.New(sha256.New, []byte(readWriteToken))
	mac.Write([]byte(payloadBase64))
	signature := hex.EncodeToString(mac.Sum(nil))
	secured := base64.StdEncoding.EncodeToString([]byte(signature + "." + payloadBase64))
	return "vercel_blob_client_" + storeID + "_" + secured, nil
}

func verifyBlobCallback(body []byte, signature string) bool {
	token := strings.TrimSpace(os.Getenv("BLOB_READ_WRITE_TOKEN"))
	if token == "" || signature == "" {
		return false
	}
	given, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), given)
}
