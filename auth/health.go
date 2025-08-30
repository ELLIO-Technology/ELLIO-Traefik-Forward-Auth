package auth

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/edl"
)

type HealthHandler struct {
	updater *edl.Updater
}

func NewHealthHandler(updater *edl.Updater) *HealthHandler {
	return &HealthHandler{
		updater: updater,
	}
}

func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	lastUpdate, lastError, updateCount, entryCount := h.updater.GetStatus()

	status := map[string]interface{}{
		"status":                   "healthy",
		"version":                  getVersion(),
		"git_commit":               getGitCommit(),
		"build_date":               getBuildDate(),
		"last_update":              lastUpdate.Format(time.RFC3339),
		"update_count":             updateCount,
		"entry_count":              entryCount,
		"uptime_since_last_update": time.Since(lastUpdate).Seconds(),
	}

	if lastError != nil {
		status["last_error"] = lastError.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		// Log error but response is already being written
		_ = err
	}
}

func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	lastUpdate, _, _, entryCount := h.updater.GetStatus()

	if lastUpdate.IsZero() || entryCount == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, err := w.Write([]byte("Not ready - EDL not yet loaded")); err != nil {
			// Log error but response is already being written
			_ = err
		}
		return
	}

	if time.Since(lastUpdate) > 2*time.Hour {
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, err := w.Write([]byte("Not ready - EDL data is stale")); err != nil {
			// Log error but response is already being written
			_ = err
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("Ready")); err != nil {
		// Log error but response is already being written
		_ = err
	}
}
