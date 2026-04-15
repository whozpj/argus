package store

import (
	"database/sql"
	"fmt"
	"time"
)

// User represents an authenticated Argus user.
type User struct {
	ID          string
	Email       string
	GithubID    string
	GoogleID    string
	DisplayName string // empty string means unset
	CreatedAt   time.Time
}

// Project is an isolation boundary — events and baselines are scoped per project.
type Project struct {
	ID        string
	UserID    string
	Name      string
	CreatedAt time.Time
}

// APIKey is a hashed ingest credential scoped to a project.
type APIKey struct {
	ID        string
	ProjectID string
	KeyHash   string
	KeyPrefix string
	Name      string
	CreatedAt time.Time
}

// UpsertUser creates or updates a user by their OAuth provider ID.
// Returns the user's ID.
func (d *DB) UpsertUser(email, githubID, googleID string) (string, error) {
	var id string
	err := d.sql.QueryRow(`
		INSERT INTO users (id, email, github_id, google_id)
		VALUES (gen_random_uuid()::text, $1, NULLIF($2,''), NULLIF($3,''))
		ON CONFLICT (email) DO UPDATE SET
			github_id = COALESCE(NULLIF($2,''), users.github_id),
			google_id = COALESCE(NULLIF($3,''), users.google_id)
		RETURNING id`, email, githubID, googleID).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert user: %w", err)
	}
	return id, nil
}

// GetUserByID returns a user by their ID.
func (d *DB) GetUserByID(id string) (User, error) {
	var u User
	var githubID, googleID, displayName sql.NullString
	err := d.sql.QueryRow(`
		SELECT id, email, COALESCE(github_id,''), COALESCE(google_id,''),
		       COALESCE(display_name,''), created_at
		FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.Email, &githubID, &googleID, &displayName, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return User{}, fmt.Errorf("user not found")
	}
	if err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	u.GithubID = githubID.String
	u.GoogleID = googleID.String
	u.DisplayName = displayName.String
	return u, nil
}

// UpdateDisplayName sets the display name for a user.
// An empty string clears the display name.
func (d *DB) UpdateDisplayName(userID, displayName string) error {
	var val any
	if displayName != "" {
		val = displayName
	}
	_, err := d.sql.Exec(
		`UPDATE users SET display_name = $1 WHERE id = $2`, val, userID)
	if err != nil {
		return fmt.Errorf("update display name: %w", err)
	}
	return nil
}

// CreateProject creates a new project for a user and returns it.
func (d *DB) CreateProject(userID, name string) (Project, error) {
	var p Project
	err := d.sql.QueryRow(`
		INSERT INTO projects (id, user_id, name)
		VALUES (gen_random_uuid()::text, $1, $2)
		RETURNING id, user_id, name, created_at`, userID, name).
		Scan(&p.ID, &p.UserID, &p.Name, &p.CreatedAt)
	if err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	return p, nil
}

// ListProjects returns all projects for a user, ordered by creation time.
func (d *DB) ListProjects(userID string) ([]Project, error) {
	rows, err := d.sql.Query(`
		SELECT id, user_id, name, created_at
		FROM projects WHERE user_id = $1 ORDER BY created_at ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CreateAPIKey stores a new (pre-hashed) API key for a project.
func (d *DB) CreateAPIKey(projectID, keyHash, keyPrefix, name string) (APIKey, error) {
	var k APIKey
	err := d.sql.QueryRow(`
		INSERT INTO api_keys (id, project_id, key_hash, key_prefix, name)
		VALUES (gen_random_uuid()::text, $1, $2, $3, $4)
		RETURNING id, project_id, key_hash, key_prefix, name, created_at`,
		projectID, keyHash, keyPrefix, name).
		Scan(&k.ID, &k.ProjectID, &k.KeyHash, &k.KeyPrefix, &k.Name, &k.CreatedAt)
	if err != nil {
		return APIKey{}, fmt.Errorf("create api key: %w", err)
	}
	return k, nil
}

// ListAPIKeys returns all API keys for a project (key_hash is not returned to callers).
func (d *DB) ListAPIKeys(projectID string) ([]APIKey, error) {
	rows, err := d.sql.Query(`
		SELECT id, project_id, key_prefix, name, created_at
		FROM api_keys WHERE project_id = $1 ORDER BY created_at ASC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var out []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.KeyPrefix, &k.Name, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// ResolveAPIKey looks up a project ID by the raw API key hash.
// Returns ("", false, nil) if the key doesn't exist.
func (d *DB) ResolveAPIKey(keyHash string) (projectID string, ok bool, err error) {
	err = d.sql.QueryRow(`SELECT project_id FROM api_keys WHERE key_hash = $1`, keyHash).Scan(&projectID)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("resolve api key: %w", err)
	}
	return projectID, true, nil
}

// CreateOAuthSession stores a short-lived CLI login code.
func (d *DB) CreateOAuthSession(code, userID string, expiresAt time.Time) error {
	_, err := d.sql.Exec(`
		INSERT INTO oauth_sessions (code, user_id, expires_at) VALUES ($1, $2, $3)`,
		code, userID, expiresAt)
	if err != nil {
		return fmt.Errorf("create oauth session: %w", err)
	}
	return nil
}

// OwnsProject returns true when userID is the owner of projectID.
// Used by the dashboard to validate project selection from JWT-authenticated requests.
func (d *DB) OwnsProject(userID, projectID string) (bool, error) {
	var count int
	err := d.sql.QueryRow(
		`SELECT COUNT(*) FROM projects WHERE id = $1 AND user_id = $2`,
		projectID, userID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("owns project: %w", err)
	}
	return count > 0, nil
}

// ConsumeOAuthSession exchanges a code for a userID and deletes the session.
// Returns ("", false, nil) if code not found or expired.
func (d *DB) ConsumeOAuthSession(code string) (userID string, ok bool, err error) {
	err = d.sql.QueryRow(`
		DELETE FROM oauth_sessions
		WHERE code = $1 AND expires_at > NOW()
		RETURNING user_id`, code).Scan(&userID)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("consume oauth session: %w", err)
	}
	return userID, true, nil
}
