package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Vercel Blob management API. The public upload URLs live on
// *.vercel-storage.com, but delete/list calls go through vercel.com.
const (
	blobAPIBase    = "https://vercel.com/api/blob"
	blobAPIVersion = "13"
	uploadPrefix   = "uploads/"
	blobBatchSize  = 50
)

var blobHTTPClient = &http.Client{Timeout: 30 * time.Second}

type blobItem struct {
	URL        string    `json:"url"`
	Pathname   string    `json:"pathname"`
	UploadedAt time.Time `json:"uploadedAt"`
}

func blobToken() string {
	return strings.TrimSpace(os.Getenv("BLOB_READ_WRITE_TOKEN"))
}

// allowedBlobURL narrows deletes to ZIPs this app uploaded. The token is
// store-wide, so never let a caller pass an arbitrary blob URL through.
func allowedBlobURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return false
	}
	if !strings.HasSuffix(parsed.Hostname(), blobHostSuffix) {
		return false
	}
	if strings.Contains(parsed.Path, "..") {
		return false
	}
	return strings.HasPrefix(strings.TrimPrefix(parsed.Path, "/"), uploadPrefix)
}

func blobRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	token := blobToken()
	if token == "" {
		return nil, fmt.Errorf("BLOB_READ_WRITE_TOKEN belum diset")
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, blobAPIBase+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("x-api-version", blobAPIVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return blobHTTPClient.Do(req)
}

func drainBlobResponse(resp *http.Response) error {
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusNotFound {
		return nil // already gone
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("Blob API status %d", resp.StatusCode)
	}
	return nil
}

// deleteBlobs removes uploaded ZIPs once they are read, or after they are old
// enough to be abandoned. It is safe to call with an empty list.
func deleteBlobs(urls []string) error {
	allowed := make([]string, 0, len(urls))
	for _, rawURL := range urls {
		if allowedBlobURL(rawURL) {
			allowed = append(allowed, rawURL)
		}
	}
	if len(allowed) == 0 {
		return nil
	}
	payload, err := json.Marshal(map[string]any{"urls": allowed})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := blobRequest(ctx, http.MethodPost, "/delete", payload)
	if err != nil {
		return err
	}
	return drainBlobResponse(resp)
}

func deleteBlob(rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return nil
	}
	if !allowedBlobURL(rawURL) {
		return fmt.Errorf("URL blob tidak diizinkan")
	}
	return deleteBlobs([]string{rawURL})
}

func listUploadBlobs(ctx context.Context, cursor string) ([]blobItem, string, error) {
	query := url.Values{}
	query.Set("prefix", uploadPrefix)
	query.Set("limit", "1000")
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	resp, err := blobRequest(ctx, http.MethodGet, "/?"+query.Encode(), nil)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return nil, "", fmt.Errorf("Blob API status %d", resp.StatusCode)
	}
	var payload struct {
		Blobs   []blobItem `json:"blobs"`
		Cursor  string     `json:"cursor"`
		HasMore bool       `json:"hasMore"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil {
		return nil, "", err
	}
	if !payload.HasMore {
		payload.Cursor = ""
	}
	return payload.Blobs, payload.Cursor, nil
}

// cleanupOrphanBlobs deletes ZIPs left in Blob storage: abandoned uploads and
// blobs whose immediate delete failed. Deploys remove their ZIP right away, so
// anything older than the cutoff is no longer needed.
func cleanupOrphanBlobs(olderThan time.Duration) (int, error) {
	if blobToken() == "" {
		return 0, nil
	}
	cutoff := time.Now().Add(-olderThan)
	// Keep the sweep's total runtime bounded: one large page is plenty for
	// abandoned uploads, and the next run continues where this one stopped.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	deleted := 0
	cursor := ""
	for page := 0; page < 20; page++ {
		items, next, err := listUploadBlobs(ctx, cursor)
		if err != nil {
			return deleted, err
		}
		var stale []string
		for _, item := range items {
			if item.UploadedAt.Before(cutoff) && allowedBlobURL(item.URL) {
				stale = append(stale, item.URL)
			}
		}
		for start := 0; start < len(stale); start += blobBatchSize {
			end := start + blobBatchSize
			if end > len(stale) {
				end = len(stale)
			}
			if err = deleteBlobs(stale[start:end]); err != nil {
				return deleted, err
			}
			deleted += end - start
		}
		if next == "" {
			break
		}
		cursor = next
	}
	return deleted, nil
}
