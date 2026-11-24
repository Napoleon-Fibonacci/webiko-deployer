package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// DeployFile is one file that Vercel will receive.
type DeployFile struct {
	Path string
	Data []byte
}

var httpClient = &http.Client{Timeout: 70 * time.Second}

func fromZipBuffer(buffer []byte, label string) ([]DeployFile, error) {
	if len(buffer) > maxZipBytes {
		return nil, newValidation(label+" lebih dari "+itoa(maxZipBytes/1024/1024)+" MB.", FieldSource)
	}
	reader, err := zip.NewReader(bytes.NewReader(buffer), int64(len(buffer)))
	if err != nil {
		return nil, newValidation(label+" tidak bisa dibaca. Pastikan file berformat ZIP.", FieldSource)
	}
	files := make([]DeployFile, 0, len(reader.File))
	if len(reader.File) > maxZipEntries {
		return nil, newValidation(
			"ZIP berisi lebih dari "+itoa(maxZipEntries)+" entri. Maksimal "+itoa(maxFiles)+" file.",
			FieldSource,
		)
	}
	total := 0
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() || strings.HasSuffix(entry.Name, "/") {
			continue
		}
		if entry.UncompressedSize64 == 0 {
			continue
		}
		if entry.UncompressedSize64 > uint64(maxExtractedBytes) {
			return nil, newValidation(
				"Total isi file melebihi "+itoa(maxExtractedBytes/1024/1024)+" MB (anti zip bomb).",
				FieldSource,
			)
		}
		if total >= maxExtractedBytes {
			return nil, newValidation(
				"Total isi file melebihi "+itoa(maxExtractedBytes/1024/1024)+" MB (anti zip bomb).",
				FieldSource,
			)
		}
		remaining := int64(maxExtractedBytes - total)
		opened, openErr := entry.Open()
		if openErr != nil {
			return nil, newValidation(label+" tidak bisa dibaca. Pastikan file berformat ZIP.", FieldSource)
		}
		data, readErr := io.ReadAll(io.LimitReader(opened, remaining+1))
		opened.Close()
		if readErr != nil {
			return nil, newValidation(label+" tidak bisa dibaca. Pastikan file berformat ZIP.", FieldSource)
		}
		if int64(len(data)) > remaining {
			return nil, newValidation(
				"Total isi file melebihi "+itoa(maxExtractedBytes/1024/1024)+" MB (anti zip bomb).",
				FieldSource,
			)
		}
		total += len(data)
		files = append(files, DeployFile{Path: entry.Name, Data: data})
	}
	return validateFiles(stripRoot(files), label)
}

func fetchLimited(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("respons terlalu besar")
	}
	return data, nil
}

func fromBlobURL(blobURL string) ([]DeployFile, error) {
	parsed, err := url.Parse(blobURL)
	if err != nil || parsed.Hostname() == "" {
		return nil, newValidation("URL Blob tidak valid.", FieldSource)
	}
	if !strings.HasSuffix(parsed.Hostname(), blobHostSuffix) {
		return nil, newValidation("Sumber ZIP bukan dari penyimpanan yang diizinkan.", FieldSource)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	buffer, err := fetchLimited(ctx, blobURL, maxZipBytes)
	if err != nil {
		return nil, newValidation("Gagal mengambil ZIP dari penyimpanan sementara.", FieldSource)
	}
	return fromZipBuffer(buffer, "ZIP")
}

var repoPattern = regexp.MustCompile(`^/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?/?$`)

func fromGitHub(rawURL string) ([]DeployFile, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Hostname() == "" {
		return nil, newValidation("URL GitHub tidak valid.", FieldSource)
	}
	if parsed.Host != "github.com" {
		return nil, newValidation("Hanya repo github.com yang didukung.", FieldSource)
	}
	match := repoPattern.FindStringSubmatch(parsed.Path)
	if match == nil {
		return nil, newValidation("URL repo tidak valid. Pakai bentuk github.com/user/repo.", FieldSource)
	}
	owner, repo := match[1], match[2]

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()

	apiURL := "https://api.github.com/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "webiko-deployer")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, newValidation("Repo tidak ditemukan atau privat.", FieldSource)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, newValidation("Repo tidak ditemukan atau privat.", FieldSource)
	}
	var info struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, newValidation("Repo tidak ditemukan atau privat.", FieldSource)
	}
	branch := info.DefaultBranch
	if branch == "" {
		branch = "main"
	}

	archiveURL := "https://codeload.github.com/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) +
		"/zip/refs/heads/" + url.PathEscape(branch)
	archive, err := fetchLimited(ctx, archiveURL, maxZipBytes)
	if err != nil {
		return nil, newValidation("Gagal mengunduh repo dari GitHub.", FieldSource)
	}
	return fromZipBuffer(archive, "Repo")
}

func fetchDeployFiles(sourceType, sourceURL, blobURL string) ([]DeployFile, error) {
	switch sourceType {
	case "zip":
		if blobURL == "" {
			return nil, newValidation("ZIP belum diunggah.", FieldSource)
		}
		return fromBlobURL(blobURL)
	case "github":
		return fromGitHub(sourceURL)
	default:
		return nil, newValidation("Sumber tidak dikenal.", FieldSource)
	}
}
