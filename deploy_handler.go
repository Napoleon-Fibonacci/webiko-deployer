package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type deploySourcePayload struct {
	Type    string `json:"type"`
	URL     string `json:"url"`
	BlobURL string `json:"blobUrl"`
}

type deployRequest struct {
	Name   string              `json:"name"`
	Source deploySourcePayload `json:"source"`
}

type deployEvent struct {
	Type    string     `json:"type"`
	Stage   string     `json:"stage,omitempty"`
	URL     string     `json:"url,omitempty"`
	Renamed bool       `json:"renamed,omitempty"`
	Code    int        `json:"code,omitempty"`
	Message string     `json:"message,omitempty"`
	Field   ErrorField `json:"field,omitempty"`
}

func errorEvent(err error) deployEvent {
	return deployEvent{
		Type:    "error",
		Code:    errorCode(err),
		Message: errorMessage(err),
		Field:   errorField(err),
	}
}

func handleDeploy(w http.ResponseWriter, r *http.Request) {
	var payload deployRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		writeJSONError(w, newAppError(http.StatusBadRequest, "Body tidak valid.", FieldSource))
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	send := func(event deployEvent) {
		encoded, _ := json.Marshal(event)
		_, _ = w.Write(append(encoded, '\n'))
		if flusher != nil {
			flusher.Flush()
		}
	}
	stage := func(value string) {
		send(deployEvent{Type: "stage", Stage: value})
	}

	var rowID string
	finish := func(err error) {
		if rowID != "" {
			_ = markFailed(rowID)
		}
		send(errorEvent(err))
	}

	if payload.Source.Type == "" {
		finish(newValidation("Body tidak valid.", FieldSource))
		return
	}
	if payload.Source.Type != "zip" && payload.Source.Type != "github" {
		finish(newValidation("Sumber tidak dikenal.", FieldSource))
		return
	}

	// Local validation first: invalid input must not spend quota or touch DB.
	name, err := validateName(payload.Name)
	if err != nil {
		finish(err)
		return
	}
	// Re-check on the server: the browser may skip /api/check-name, and a
	// taken name must fail before a slot or an old project is touched.
	if vercelNameTaken(name) {
		finish(newNameTaken())
		return
	}
	if err = enforceRateLimit("deploy", getClientIP(r)); err != nil {
		finish(err)
		return
	}

	stage("files")
	files, err := fetchDeployFiles(payload.Source.Type, payload.Source.URL, payload.Source.BlobURL)
	if err != nil {
		finish(err)
		return
	}

	stage("slot")
	reservation, err := reserveSlot(name, payload.Source.Type)
	if err != nil {
		finish(err)
		return
	}
	rowID = reservation.RowID
	stage("create")

	// Deleting the oldest project happens outside the advisory lock.
	if reservation.Victim != nil {
		victim := reservation.Victim
		var deleteErr error
		if victim.VercelProjectID.Valid && victim.VercelProjectID.String != "" {
			// Only ever delete via vercel_project_id from our own table.
			deleteErr = deleteVercelProject(victim.VercelProjectID.String)
		}
		if deleteErr != nil {
			_ = revertVictim(victim.ID)
			finish(newVercelAPI("Gagal menghapus project tertua di Vercel. Deploy baru dibatalkan; coba lagi.", http.StatusBadGateway))
			return
		}
		if err = markDeleted(victim.ID); err != nil {
			_ = revertVictim(victim.ID)
			finish(err)
			return
		}
	}

	result, err := createDeployment(name, files)
	if err != nil {
		finish(err)
		return
	}
	// Vercel may give a suffixed host when the exact name is taken. Reject it
	// instead of handing out a confusing URL: delete the temporary project and
	// ask the visitor to pick another name.
	if result.Renamed {
		if deleteErr := deleteVercelProject(result.ProjectID); deleteErr != nil {
			log.Printf("cleanup rejected project %s: %v", result.ProjectID, deleteErr)
		}
		finish(newNameTaken())
		return
	}
	if err = markActive(rowID, result.ProjectID, result.URL); err != nil {
		finish(err)
		return
	}
	send(deployEvent{Type: "done", URL: result.URL, Renamed: result.Renamed})
}

func handleCheckName(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if _, err := validateName(name); err != nil {
		writeJSON(w, map[string]any{"available": false, "reason": errorMessage(err)}, http.StatusOK)
		return
	}
	if taken, err := findLiveByName(name); err == nil && taken {
		writeJSON(w, map[string]any{"available": false, "reason": nameTakenMessage}, http.StatusOK)
		return
	}
	if vercelNameTaken(name) {
		writeJSON(w, map[string]any{
			"available": false,
			"reason":    "Subdomain ini sudah dipakai di Vercel. Coba nama lain.",
		}, http.StatusOK)
		return
	}
	writeJSON(w, map[string]any{"available": true}, http.StatusOK)
}

// Vercel installs a wildcard on *.vercel.app, so DNS always resolves. An
// unused name answers 404 with x-vercel-error: DEPLOYMENT_NOT_FOUND.
func vercelNameTaken(name string) bool {
	client := &http.Client{
		Timeout: 8 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequest(http.MethodHead, "https://"+name+".vercel.app/", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.Header.Get("X-Vercel-Error") != "DEPLOYMENT_NOT_FOUND"
}
