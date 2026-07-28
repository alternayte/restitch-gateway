package main

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/restitch/restitch-gateway/internal/session"
)

// maxPreferencesBody caps a preferences PUT at 16 KB.
const maxPreferencesBody = 16 * 1024

// PreferencesAPI holds HTTP handlers for /api/v1/preferences.
type PreferencesAPI struct {
	store *session.Store
}

// NewPreferencesAPI creates a PreferencesAPI backed by store.
func NewPreferencesAPI(store *session.Store) *PreferencesAPI {
	return &PreferencesAPI{store: store}
}

// preferencesResponse is the wire shape returned by both handlers. It carries
// Initialized, which the request shape deliberately does not have — the
// decoder rejects unknown fields, so a client echoing a response back as a
// request would 400 if the shapes were shared.
type preferencesResponse struct {
	PinnedCompositions []string `json:"pinned_compositions"`
	SidebarCollapsed   bool     `json:"sidebar_collapsed"`
	DefaultTimeRange   string   `json:"default_time_range"`
	Initialized        bool     `json:"initialized"`
}

func toResponse(p session.Preferences, initialized bool) preferencesResponse {
	if p.PinnedCompositions == nil {
		p.PinnedCompositions = []string{}
	}
	return preferencesResponse{
		PinnedCompositions: p.PinnedCompositions,
		SidebarCollapsed:   p.SidebarCollapsed,
		DefaultTimeRange:   p.DefaultTimeRange,
		Initialized:        initialized,
	}
}

// handleGetPreferences handles GET /api/v1/preferences.
func (a *PreferencesAPI) handleGetPreferences(w http.ResponseWriter, r *http.Request) {
	id, ok := session.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusInternalServerError, "no browser session")
		return
	}

	prefs, initialized, err := a.store.GetPreferences(r.Context(), id)
	if err != nil {
		slog.Error("get preferences", "error", err, "session_id", id)
		writeError(w, http.StatusInternalServerError, "could not read preferences")
		return
	}

	writeJSON(w, http.StatusOK, toResponse(prefs, initialized))
}

// handlePutPreferences handles PUT /api/v1/preferences.
func (a *PreferencesAPI) handlePutPreferences(w http.ResponseWriter, r *http.Request) {
	id, ok := session.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusInternalServerError, "no browser session")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPreferencesBody)

	var prefs session.Preferences
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&prefs); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body exceeds 16KB")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if err := prefs.Validate(); err != nil {
		var ve *session.ValidationError
		if errors.As(err, &ve) {
			writeJSON(w, http.StatusBadRequest, ve)
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.store.PutPreferences(r.Context(), id, prefs); err != nil {
		slog.Error("put preferences", "error", err, "session_id", id)
		writeError(w, http.StatusInternalServerError, "could not save preferences")
		return
	}

	writeJSON(w, http.StatusOK, toResponse(prefs, true))
}
