package main

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/restitch/restitch-gateway/internal/registry"
)

// RegistryAPI holds HTTP handlers for the Studio-native config registry
// endpoints under /api/v1/*.
type RegistryAPI struct {
	store *registry.Store
}

// NewRegistryAPI creates a new RegistryAPI backed by store.
func NewRegistryAPI(store *registry.Store) *RegistryAPI {
	return &RegistryAPI{store: store}
}

// writeJSON encodes v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// writeError writes a JSON error response of the form {"error": message}.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// isNotFound reports whether err is one of the store's string-based
// "not found" errors (there is no sentinel error type).
func isNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not found")
}

// requireRegistryKey returns middleware that rejects requests without a
// matching X-Admin-Key. With an empty expected key no request can match, so
// the registry API stays locked until the operator configures a key. This is
// the same default-required stance as the gateway admin API.
func requireRegistryKey(expected string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if expected == "" || !keyMatches(r.Header.Get("X-Admin-Key"), expected) {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// keyMatches reports whether got equals want in constant time when the
// lengths match. A length mismatch rejects immediately.
func keyMatches(got, want string) bool {
	if len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// handleCreateConfig handles POST /api/v1/configs.
func (a *RegistryAPI) handleCreateConfig(w http.ResponseWriter, r *http.Request) {
	var input registry.CreateConfigInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	result := registry.Validate([]byte(input.YAMLContent))
	if !result.Valid {
		writeJSON(w, http.StatusUnprocessableEntity, result)
		return
	}

	cfg, err := a.store.CreateConfig(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, cfg)
}

// handleListConfigs handles GET /api/v1/configs.
func (a *RegistryAPI) handleListConfigs(w http.ResponseWriter, r *http.Request) {
	params := registry.ListConfigsParams{
		Cursor: r.URL.Query().Get("cursor"),
	}
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			params.Limit = limit
		}
	}

	configs, pageInfo, err := a.store.ListConfigs(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":     configs,
		"page_info": pageInfo,
	})
}

// handleGetConfig handles GET /api/v1/configs/{id}.
func (a *RegistryAPI) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	cfg, err := a.store.GetConfig(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cfg == nil {
		writeError(w, http.StatusNotFound, "config not found: "+id)
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

// handleUpdateConfigContent handles PUT /api/v1/configs/{id}.
func (a *RegistryAPI) handleUpdateConfigContent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var input registry.UpdateConfigInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	result := registry.Validate([]byte(input.YAMLContent))
	if !result.Valid {
		writeJSON(w, http.StatusUnprocessableEntity, result)
		return
	}

	cfg, err := a.store.UpdateConfigContent(r.Context(), id, input)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

// handleUpdateConfigMetadata handles PATCH /api/v1/configs/{id}.
func (a *RegistryAPI) handleUpdateConfigMetadata(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var input registry.UpdateConfigMetadataInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	cfg, err := a.store.UpdateConfigMetadata(r.Context(), id, input)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

// handleDeleteConfig handles DELETE /api/v1/configs/{id}.
func (a *RegistryAPI) handleDeleteConfig(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := a.store.DeleteConfig(r.Context(), id); err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleListVersions handles GET /api/v1/configs/{id}/versions.
func (a *RegistryAPI) handleListVersions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	limit := 0
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	versions, err := a.store.ListVersions(r.Context(), id, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": versions,
	})
}

// handleActivateVersion handles POST /api/v1/configs/{id}/versions/{version}/activate.
func (a *RegistryAPI) handleActivateVersion(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	versionStr := r.PathValue("version")

	version, err := strconv.Atoi(versionStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid version: "+versionStr)
		return
	}

	if err := a.store.SetActiveVersion(r.Context(), id, version); err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	cfg, err := a.store.GetConfig(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cfg == nil {
		writeError(w, http.StatusNotFound, "config not found: "+id)
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

// validateRequest is the request payload for POST /api/v1/configs/validate.
type validateRequest struct {
	YAMLContent string `json:"yaml_content"`
}

// handleValidateConfig handles POST /api/v1/configs/validate. It always
// returns 200; validity is communicated via the "valid" field of the
// response body.
func (a *RegistryAPI) handleValidateConfig(w http.ResponseWriter, r *http.Request) {
	var req validateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	result := registry.Validate([]byte(req.YAMLContent))
	writeJSON(w, http.StatusOK, result)
}

// handleGetBundle handles GET /api/v1/registry/bundle. Supports conditional
// requests via If-None-Match, returning 304 when the client's ETag matches
// the current bundle ETag.
func (a *RegistryAPI) handleGetBundle(w http.ResponseWriter, r *http.Request) {
	bundle, err := a.store.GetBundledConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if match := r.Header.Get("If-None-Match"); match != "" && match == bundle.ETag {
		w.Header().Set("ETag", bundle.ETag)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("ETag", bundle.ETag)
	writeJSON(w, http.StatusOK, bundle)
}
