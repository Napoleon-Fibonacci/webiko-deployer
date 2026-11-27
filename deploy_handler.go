package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type deploySourcePayload struct {
	Type    string `json:"type"`
	URL     string `json:"url"`
	BlobURL string `json:"blobUrl"`
}

type deployRequest struct {
	Name       string              `json:"name"`
	Source     deploySourcePayload `json:"source"`
	AccessCode string              `json:"accessCode"`
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

	// The gate runs before any cookie, quota, or Vercel call.
	if err := checkDeployAccess(payload.AccessCode, getClientIP(r)); err != nil {
		writeJSONError(w, err)
		return
	}

	// Set-Cookie has to be queued before the streaming headers go out.
	ownerID := ensureOwner(w, r)

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
	var victim *ReservedVictim
	finish := func(err error) {
		// The oldest project is only marked "deleting" during reservation. If
		// the new deployment never becomes active, put it back so a failed
		// deploy does not cost an existing site.
		if victim != nil {
			if revertErr := revertVictim(victim.ID); revertErr != nil {
				log.Printf("revert victim %s: %v", victim.ID, revertErr)
			}
			victim = nil
		}
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
	// The ZIP has been read into memory (or the read failed). Either way the
	// public blob is no longer needed; keep it out of storage.
	if payload.Source.Type == "zip" && payload.Source.BlobURL != "" {
		if deleteErr := deleteBlob(payload.Source.BlobURL); deleteErr != nil {
			log.Printf("delete uploaded blob: %v", deleteErr)
		}
	}
	if err != nil {
		finish(err)
		return
	}

	stage("slot")
	// Kunci pool dipilih sebelum slot dipesan, lalu dicatat di baris deploy.
	// Pool kosong atau database down = fallback ke VERCEL_TOKEN seperti biasa.
	keyID, vercelToken, err := pickAPIKey()
	if err != nil {
		log.Printf("pick api key: %v", err)
	}
	if vercelToken == "" {
		vercelToken = strings.TrimSpace(os.Getenv("VERCEL_TOKEN"))
		keyID = ""
	}
	reservation, err := reserveSlot(name, payload.Source.Type, ownerID, keyID)
	if err != nil {
		finish(err)
		return
	}
	rowID = reservation.RowID
	victim = reservation.Victim
	stage("create")

	// Build the replacement before removing anything. The victim's Vercel
	// project is deleted only after the new deployment is active, so a Vercel
	// failure cannot leave the visitor with neither site.
	result, err := createDeployment(name, files, vercelToken)
	if err != nil {
		finish(err)
		return
	}
	// Vercel may give a suffixed host when the exact name is taken. Reject it
	// instead of handing out a confusing URL: delete the temporary project and
	// ask the visitor to pick another name.
	if result.Renamed {
		if deleteErr := deleteVercelProject(result.ProjectID, vercelToken); deleteErr != nil {
			log.Printf("cleanup rejected project %s: %v", result.ProjectID, deleteErr)
		}
		finish(newNameTaken())
		return
	}
	if err = markActive(rowID, result.ProjectID, result.URL); err != nil {
		// The project exists but could not be recorded, so remove it to avoid
		// an orphan and restore the old project.
		if deleteErr := deleteVercelProject(result.ProjectID, vercelToken); deleteErr != nil {
			log.Printf("cleanup unrecorded project %s: %v", result.ProjectID, deleteErr)
		}
		finish(err)
		return
	}

	if victim != nil {
		old := victim
		victim = nil
		if old.VercelProjectID.Valid && old.VercelProjectID.String != "" {
			if deleteErr := deleteVercelProject(old.VercelProjectID.String, tokenForDeployment(old.APIKeyID)); deleteErr != nil {
				// The new site is already live; keep the old one running too
				// instead of marking a live project as deleted. A later deploy
				// or the daily sweep can retry.
				log.Printf("delete victim %s: %v", old.ID, deleteErr)
				if revertErr := revertVictim(old.ID); revertErr != nil {
					log.Printf("revert victim %s: %v", old.ID, revertErr)
				}
			} else if markErr := markDeleted(old.ID); markErr != nil {
				// Project is gone; the daily sweep closes the row by checking
				// whether the Vercel project still exists.
				log.Printf("mark victim %s deleted: %v", old.ID, markErr)
			}
		} else if markErr := markDeleted(old.ID); markErr != nil {
			log.Printf("mark victim %s deleted: %v", old.ID, markErr)
		}
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
