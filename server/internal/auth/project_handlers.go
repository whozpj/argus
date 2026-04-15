package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/whozpj/argus/server/internal/store"
)

type projectHandlers struct {
	db *store.DB
}

// RegisterProjectRoutes wires project and user management routes onto mux.
// All routes require a valid JWT (enforced by RequireJWT middleware).
func RegisterProjectRoutes(mux *http.ServeMux, db *store.DB) {
	h := &projectHandlers{db: db}
	mux.Handle("GET /api/v1/me", RequireJWT(http.HandlerFunc(h.me)))
	mux.Handle("PATCH /api/v1/me", RequireJWT(http.HandlerFunc(h.updateMe)))
	mux.Handle("POST /api/v1/projects", RequireJWT(http.HandlerFunc(h.createProject)))
	mux.Handle("GET /api/v1/projects", RequireJWT(http.HandlerFunc(h.listProjects)))
	mux.Handle("POST /api/v1/projects/{id}/keys", RequireJWT(http.HandlerFunc(h.createKey)))
}

func (h *projectHandlers) me(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserIDFromContext(r.Context())

	user, err := h.db.GetUserByID(userID)
	if err != nil {
		slog.Error("get user", "err", err)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	projects, err := h.db.ListProjects(userID)
	if err != nil {
		slog.Error("list projects", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	type projectJSON struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		CreatedAt string `json:"created_at"`
	}
	type meResponse struct {
		ID          string        `json:"id"`
		Email       string        `json:"email"`
		DisplayName *string       `json:"display_name"`
		Projects    []projectJSON `json:"projects"`
	}

	var displayName *string
	if user.DisplayName != "" {
		displayName = &user.DisplayName
	}
	resp := meResponse{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: displayName,
		Projects:    make([]projectJSON, 0, len(projects)),
	}
	for _, p := range projects {
		resp.Projects = append(resp.Projects, projectJSON{
			ID:        p.ID,
			Name:      p.Name,
			CreatedAt: p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

func (h *projectHandlers) updateMe(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserIDFromContext(r.Context())

	var req struct {
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.DisplayName) > 50 {
		http.Error(w, "display_name must be 50 characters or fewer", http.StatusBadRequest)
		return
	}

	if err := h.db.UpdateDisplayName(userID, req.DisplayName); err != nil {
		slog.Error("update display name", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Return the updated user (same shape as GET /api/v1/me).
	user, err := h.db.GetUserByID(userID)
	if err != nil {
		slog.Error("get user after update", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	projects, err := h.db.ListProjects(userID)
	if err != nil {
		slog.Error("list projects after update", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	type projectJSON struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		CreatedAt string `json:"created_at"`
	}
	type meResponse struct {
		ID          string        `json:"id"`
		Email       string        `json:"email"`
		DisplayName *string       `json:"display_name"`
		Projects    []projectJSON `json:"projects"`
	}

	var displayName *string
	if user.DisplayName != "" {
		displayName = &user.DisplayName
	}
	resp := meResponse{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: displayName,
		Projects:    make([]projectJSON, 0, len(projects)),
	}
	for _, p := range projects {
		resp.Projects = append(resp.Projects, projectJSON{
			ID:        p.ID,
			Name:      p.Name,
			CreatedAt: p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

func (h *projectHandlers) createProject(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserIDFromContext(r.Context())

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	proj, err := h.db.CreateProject(userID, req.Name)
	if err != nil {
		slog.Error("create project", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
		"id":         proj.ID,
		"name":       proj.Name,
		"created_at": proj.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *projectHandlers) listProjects(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserIDFromContext(r.Context())

	projects, err := h.db.ListProjects(userID)
	if err != nil {
		slog.Error("list projects", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	type projectJSON struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		CreatedAt string `json:"created_at"`
	}
	out := make([]projectJSON, 0, len(projects))
	for _, p := range projects {
		out = append(out, projectJSON{
			ID:        p.ID,
			Name:      p.Name,
			CreatedAt: p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out) //nolint:errcheck
}

func (h *projectHandlers) createKey(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserIDFromContext(r.Context())
	projectID := r.PathValue("id")

	// Verify the authenticated user owns this project.
	projects, err := h.db.ListProjects(userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	owned := false
	for _, p := range projects {
		if p.ID == projectID {
			owned = true
			break
		}
	}
	if !owned {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	rawKey, hash, prefix, err := GenerateAPIKey()
	if err != nil {
		slog.Error("generate api key", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	k, err := h.db.CreateAPIKey(projectID, hash, prefix, req.Name)
	if err != nil {
		slog.Error("create api key", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
		"id":         k.ID,
		"key":        rawKey, // Full key returned only once — never stored in plaintext
		"prefix":     prefix,
		"name":       k.Name,
		"created_at": k.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}
