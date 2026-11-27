package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const vercelAPI = "https://api.vercel.com"

type CreateDeploymentResult struct {
	ProjectID    string
	DeploymentID string
	URL          string
	Renamed      bool
}

func vercelHeadersFor(token string) (http.Header, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, newVercelAPI("VERCEL_TOKEN belum diset di server.", http.StatusBadGateway)
	}
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)
	headers.Set("Content-Type", "application/json")
	return headers, nil
}

func withTeam(rawURL string) string {
	teamID := strings.TrimSpace(os.Getenv("VERCEL_TEAM_ID"))
	if teamID == "" {
		return rawURL
	}
	separator := "?"
	if strings.Contains(rawURL, "?") {
		separator = "&"
	}
	return rawURL + separator + "teamId=" + url.QueryEscape(teamID)
}

func vercelDo(ctx context.Context, method, rawURL string, body io.Reader, token string) (*http.Response, error) {
	headers, err := vercelHeadersFor(token)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	req.Header = headers
	return httpClient.Do(req)
}

func parseVercelError(resp *http.Response, fallback string) error {
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&payload)
	if payload.Error.Message != "" {
		return newVercelAPI(fallback+" ("+payload.Error.Message+")", http.StatusBadGateway)
	}
	return newVercelAPI(fallback, http.StatusBadGateway)
}

func createDeployment(name string, files []DeployFile, token string) (CreateDeploymentResult, error) {
	type vercelFile struct {
		File     string `json:"file"`
		Data     string `json:"data"`
		Encoding string `json:"encoding"`
	}
	payloadFiles := make([]vercelFile, 0, len(files))
	for _, file := range files {
		payloadFiles = append(payloadFiles, vercelFile{
			File: normalizePath(file.Path),
			Data: base64.StdEncoding.EncodeToString(file.Data),
			// Without encoding=base64 Vercel reads data as utf-8 text and
			// stores the base64 string itself.
			Encoding: "base64",
		})
	}
	body, err := json.Marshal(map[string]any{
		"name":            name,
		"target":          "production",
		"projectSettings": map[string]any{"framework": nil},
		"files":           payloadFiles,
	})
	if err != nil {
		return CreateDeploymentResult{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
	defer cancel()
	resp, err := vercelDo(ctx, http.MethodPost,
		withTeam(vercelAPI+"/v13/deployments?skipAutoDetectionConfirmation=1"),
		bytes.NewReader(body), token)
	if err != nil {
		return CreateDeploymentResult{}, newVercelAPI("Deploy gagal di sisi Vercel.", http.StatusBadGateway)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CreateDeploymentResult{}, parseVercelError(resp, "Deploy gagal di sisi Vercel.")
	}
	var created struct {
		ID string `json:"id"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&created); err != nil || created.ID == "" {
		return CreateDeploymentResult{}, newVercelAPI("Deploy gagal di sisi Vercel.", http.StatusBadGateway)
	}

	final, err := pollDeployment(created.ID, token)
	if err != nil {
		return CreateDeploymentResult{}, err
	}
	if final.ReadyState == "ERROR" || final.ReadyState == "CANCELED" {
		return CreateDeploymentResult{}, newVercelAPI("Deploy gagal di sisi Vercel. Coba lagi sebentar lagi.", http.StatusBadGateway)
	}
	if final.ProjectID == "" {
		return CreateDeploymentResult{}, newVercelAPI("Deploy gagal di sisi Vercel (URL tidak ditemukan).", http.StatusBadGateway)
	}
	publicURL, err := resolvePublicURL(final, name, token)
	if err != nil {
		return CreateDeploymentResult{}, err
	}
	if publicURL == "" {
		return CreateDeploymentResult{}, newVercelAPI("Deploy gagal di sisi Vercel (URL tidak ditemukan).", http.StatusBadGateway)
	}
	return CreateDeploymentResult{
		ProjectID:    final.ProjectID,
		DeploymentID: final.ID,
		URL:          publicURL,
		Renamed:      publicURL != "https://"+name+".vercel.app",
	}, nil
}

type deploymentState struct {
	ID         string   `json:"id"`
	ProjectID  string   `json:"projectId"`
	ReadyState string   `json:"readyState"`
	Alias      []string `json:"alias"`
	URL        string   `json:"url"`
}

func pollDeployment(deploymentID, token string) (deploymentState, error) {
	deadline := time.Now().Add(55 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 16*time.Second)
		resp, err := vercelDo(ctx, http.MethodGet,
			withTeam(vercelAPI+"/v13/deployments/"+url.PathEscape(deploymentID)), nil, token)
		if err != nil {
			cancel()
			return deploymentState{}, newVercelAPI("Gagal membaca status deployment.", http.StatusBadGateway)
		}
		var state deploymentState
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			err = parseVercelError(resp, "Gagal membaca status deployment.")
			resp.Body.Close()
			cancel()
			return deploymentState{}, err
		}
		err = json.NewDecoder(resp.Body).Decode(&state)
		resp.Body.Close()
		cancel()
		if err != nil {
			return deploymentState{}, newVercelAPI("Gagal membaca status deployment.", http.StatusBadGateway)
		}
		if state.ReadyState == "READY" || state.ReadyState == "ERROR" || state.ReadyState == "CANCELED" {
			return state, nil
		}
		time.Sleep(2 * time.Second)
	}
	return deploymentState{}, newVercelAPI("Deploy belum selesai dalam batas waktu. Coba lagi nanti.", http.StatusGatewayTimeout)
}

// pickHost prefers the exact production host, then the shortest *.vercel.app
// host. Branch aliases (-git-) and deployment hash URLs are never chosen.
func pickHost(hosts []string, name string) string {
	clean := make([]string, 0, len(hosts))
	for _, host := range hosts {
		if strings.HasSuffix(host, ".vercel.app") && !strings.Contains(host, "-git-") {
			clean = append(clean, host)
		}
	}
	exact := name + ".vercel.app"
	for _, host := range clean {
		if host == exact {
			return host
		}
	}
	sort.SliceStable(clean, func(i, j int) bool { return len(clean[i]) < len(clean[j]) })
	if len(clean) > 0 {
		return clean[0]
	}
	return ""
}

func getVercelJSON(ctx context.Context, path string, target any, token string) bool {
	resp, err := vercelDo(ctx, http.MethodGet, withTeam(vercelAPI+path), nil, token)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}
	return json.NewDecoder(resp.Body).Decode(target) == nil
}

// productionHosts gathers aliases from three Vercel endpoints; any of them
// can be empty for a moment right after READY.
func productionHosts(deployment deploymentState, token string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var fresh struct {
		Alias []string `json:"alias"`
	}
	var project struct {
		Targets struct {
			Production struct {
				Alias []string `json:"alias"`
			} `json:"production"`
		} `json:"targets"`
	}
	var domains struct {
		Domains []struct {
			Name      string `json:"name"`
			GitBranch string `json:"gitBranch"`
		} `json:"domains"`
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		getVercelJSON(ctx, "/v13/deployments/"+url.PathEscape(deployment.ID), &fresh, token)
	}()
	if deployment.ProjectID != "" {
		wg.Add(2)
		go func() {
			defer wg.Done()
			getVercelJSON(ctx, "/v9/projects/"+url.PathEscape(deployment.ProjectID), &project, token)
		}()
		go func() {
			defer wg.Done()
			getVercelJSON(ctx, "/v9/projects/"+url.PathEscape(deployment.ProjectID)+"/domains?production=true", &domains, token)
		}()
	}
	wg.Wait()

	hosts := append([]string{}, deployment.Alias...)
	hosts = append(hosts, fresh.Alias...)
	hosts = append(hosts, project.Targets.Production.Alias...)
	for _, domain := range domains.Domains {
		if domain.Name != "" && domain.GitBranch == "" {
			hosts = append(hosts, domain.Name)
		}
	}
	return hosts
}

func resolvePublicURL(deployment deploymentState, name, token string) (string, error) {
	for attempt := 0; attempt < 2; attempt++ {
		if host := pickHost(productionHosts(deployment, token), name); host != "" {
			return "https://" + host, nil
		}
		if attempt == 0 {
			time.Sleep(1500 * time.Millisecond)
		}
	}
	if deployment.URL != "" {
		return "https://" + deployment.URL, nil
	}
	return "", nil
}

func deleteVercelProject(projectID, token string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	resp, err := vercelDo(ctx, http.MethodDelete,
		withTeam(vercelAPI+"/v9/projects/"+url.PathEscape(projectID)), nil, token)
	if err != nil {
		return newVercelAPI("Gagal menghapus project lama di Vercel.", http.StatusBadGateway)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || (resp.StatusCode >= 200 && resp.StatusCode < 300) {
		return nil
	}
	return parseVercelError(resp, "Gagal menghapus project lama di Vercel.")
}

// projectExists returns false only on a confident 404. Network failures, 403,
// 429 and 5xx stay unknown so a possibly-live site is never freed by mistake.
func projectExists(projectID, token string) (exists *bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 16*time.Second)
	defer cancel()
	resp, err := vercelDo(ctx, http.MethodGet,
		withTeam(vercelAPI+"/v9/projects/"+url.PathEscape(projectID)), nil, token)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		value := false
		return &value, nil
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		value := true
		return &value, nil
	}
	return nil, errors.New("status Vercel tidak pasti")
}
