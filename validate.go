package main

import (
	"encoding/json"
	"path"
	"regexp"
	"strings"
)

const (
	maxNameLength        = 40
	minNameLength        = 3
	maxFiles             = 100
	maxZipBytes          = 10 * 1024 * 1024 // 10 MB per ZIP
	maxExtractedBytes    = 50 * 1024 * 1024 // 50 MB, anti zip bomb
	zipOnlyMessage       = "Sumber ZIP harus berformat .zip."
	nameRuleText         = "Huruf kecil, angka, tanda hubung; 3–40 karakter."
	zipAccept            = ".zip,application/zip,application/x-zip-compressed"
	htmlAccept           = ".html,.htm,text/html"
	blobHostSuffix       = ".vercel-storage.com"
	vercelBlobAPIBase    = "https://vercel.com/api/blob"
	vercelBlobAPIVersion = "11"
)

var (
	namePattern = regexp.MustCompile(`^[a-z0-9-]+$`)
	htmlPattern = regexp.MustCompile(`(?i)\.html?$`)
)

var zipContentTypes = []string{"application/zip", "application/x-zip-compressed"}

func isZipName(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".zip")
}

func isHTMLName(name string) bool {
	return htmlPattern.MatchString(name)
}

// isValidName is the non-throwing gate used by the client-side flow too.
func isValidName(value string) bool {
	return len(value) >= minNameLength &&
		len(value) <= maxNameLength &&
		namePattern.MatchString(value) &&
		!strings.HasPrefix(value, "-") &&
		!strings.HasSuffix(value, "-")
}

func validateName(name string) (string, error) {
	if name == "" {
		return "", newValidation("Nama project wajib diisi.", FieldName)
	}
	if !isValidName(name) {
		return "", newValidation(
			"Nama harus 3-40 karakter: huruf kecil, angka, tanda hubung, tanpa hubung di awal atau akhir.",
			FieldName,
		)
	}
	return name, nil
}

func normalizePath(value string) string {
	return strings.ReplaceAll(value, "\\", "/")
}

// assertSafePath rejects traversal, absolute paths and Windows drive letters.
func assertSafePath(value string) error {
	p := normalizePath(value)
	if strings.HasPrefix(p, "/") ||
		strings.Contains(p, "..") ||
		regexp.MustCompile(`^[a-zA-Z]:`).MatchString(p) ||
		regexp.MustCompile(`(^|/)\.\.?(/|$)`).MatchString(p) {
		return newValidation("Path tidak aman di file: "+value, FieldSource)
	}
	return nil
}

var junkNames = map[string]bool{
	".DS_Store":   true,
	"Thumbs.db":   true,
	"desktop.ini": true,
}

func isJunkPath(value string) bool {
	segments := strings.Split(normalizePath(value), "/")
	for _, segment := range segments {
		if segment == "__MACOSX" || segment == ".git" {
			return true
		}
	}
	return junkNames[segments[len(segments)-1]]
}

// validateFiles cleans the list, enforces limits and guarantees index.html.
func validateFiles(files []DeployFile, sourceLabel string) ([]DeployFile, error) {
	cleaned := make([]DeployFile, 0, len(files))
	for _, file := range files {
		p := normalizePath(file.Path)
		if err := assertSafePath(p); err != nil {
			return nil, err
		}
		if isJunkPath(p) || len(file.Data) == 0 {
			continue
		}
		cleaned = append(cleaned, DeployFile{Path: p, Data: file.Data})
	}
	if len(cleaned) == 0 {
		return nil, newValidation(sourceLabel+" tidak berisi file.", FieldSource)
	}
	cleaned, err := ensureIndexHTML(cleaned)
	if err != nil {
		return nil, err
	}
	if len(cleaned) > maxFiles {
		return nil, newValidation(
			sourceLabel+" berisi lebih dari "+itoa(maxFiles)+" file. Maksimal "+itoa(maxFiles)+" file.",
			FieldSource,
		)
	}
	var total int
	for _, file := range cleaned {
		total += len(file.Data)
		if total > maxExtractedBytes {
			return nil, newValidation(
				"Total isi file melebihi "+itoa(maxExtractedBytes/1024/1024)+" MB (anti zip bomb).",
				FieldSource,
			)
		}
	}
	for _, file := range cleaned {
		if file.Path != "package.json" {
			continue
		}
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}
		if json.Unmarshal(file.Data, &pkg) != nil {
			continue
		}
		if _, ok := pkg.Scripts["build"]; ok {
			return nil, newValidation("package.json punya skrip build. Deployer hanya menerima situs statis tanpa build.", FieldSource)
		}
		if _, ok := pkg.Scripts["prebuild"]; ok {
			return nil, newValidation("package.json punya skrip build. Deployer hanya menerima situs statis tanpa build.", FieldSource)
		}
	}
	return cleaned, nil
}

// ensureIndexHTML copies a lone root HTML file to index.html, or rejects the
// source when the root cannot serve a page.
func ensureIndexHTML(files []DeployFile) ([]DeployFile, error) {
	hasIndex := false
	onlyAPI := true
	for _, file := range files {
		if file.Path == "index.html" {
			hasIndex = true
		}
		if !strings.HasPrefix(file.Path, "api/") {
			onlyAPI = false
		}
	}
	if hasIndex {
		return files, nil
	}
	if onlyAPI {
		return files, nil
	}
	var rootHTML []DeployFile
	for _, file := range files {
		if !strings.Contains(file.Path, "/") && isHTMLName(file.Path) {
			rootHTML = append(rootHTML, file)
		}
	}
	if len(rootHTML) == 1 {
		return append(files, DeployFile{Path: "index.html", Data: rootHTML[0].Data}), nil
	}
	return nil, newValidation(
		"index.html tidak ditemukan di root. Wajib ada index.html, kecuali situs hanya berisi folder api/.",
		FieldSource,
	)
}

// stripRoot drops the single top-level folder GitHub archives always add.
func stripRoot(files []DeployFile) []DeployFile {
	if len(files) == 0 {
		return files
	}
	prefixes := map[string]bool{}
	for _, file := range files {
		prefixes[strings.Split(normalizePath(file.Path), "/")[0]] = true
	}
	if len(prefixes) != 1 {
		return files
	}
	var only string
	for prefix := range prefixes {
		only = prefix
	}
	if only == "" {
		return files
	}
	for _, file := range files {
		if normalizePath(file.Path) == only {
			return files
		}
	}
	out := make([]DeployFile, 0, len(files))
	for _, file := range files {
		p := normalizePath(file.Path)
		rest := strings.TrimPrefix(p, only+"/")
		if rest == p {
			rest = path.Base(p)
		}
		out = append(out, DeployFile{Path: rest, Data: file.Data})
	}
	return out
}
