package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db        *sql.DB
	secretKey string
}

type Provider struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	BaseURL      string            `json:"base_url"`
	APIKey       string            `json:"api_key,omitempty"`
	ExtraHeaders map[string]string `json:"extra_headers"`
	Enabled      bool              `json:"enabled"`
	CreatedAt    string            `json:"created_at"`
	UpdatedAt    string            `json:"updated_at"`
}

type Model struct {
	InternalID          string `json:"internal_id"`
	ProviderID          string `json:"provider_id"`
	OriginalID          string `json:"original_id"`
	DisplayName         string `json:"display_name"`
	SupportsChat        bool   `json:"supports_chat"`
	SupportsResponses   bool   `json:"supports_responses"`
	SupportsStream      bool   `json:"supports_stream"`
	ContextLength       int64  `json:"context_length"`
	Enabled             bool   `json:"enabled"`
	AutoDisabled        bool   `json:"auto_disabled"`
	AutoDisabledReason  string `json:"auto_disabled_reason"`
	FailCount           int    `json:"fail_count"`
	WindowStart         string `json:"window_start"`
	LastFailureAt       string `json:"last_failure_at"`
	CooldownUntil       string `json:"cooldown_until"`
	CooldownCount       int    `json:"cooldown_count"`
	UpstreamErrorStatus int    `json:"upstream_error_status"`
	UpstreamErrorAt     string `json:"upstream_error_at"`
	UpstreamError       string `json:"upstream_error"`
	ProviderEnabled     bool   `json:"provider_enabled"`
}

type ModelGroup struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Strategy  string             `json:"strategy"`
	Enabled   bool               `json:"enabled"`
	Cursor    int                `json:"cursor"`
	Members   []ModelGroupMember `json:"members"`
	CreatedAt string             `json:"created_at"`
	UpdatedAt string             `json:"updated_at"`
}

type ModelGroupMember struct {
	ModelID  string `json:"model_id"`
	Position int    `json:"position"`
	Enabled  bool   `json:"enabled"`
	Model    *Model `json:"model,omitempty"`
}

type Route struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Enabled   bool        `json:"enabled"`
	Steps     []RouteStep `json:"steps"`
	Overrides []Override  `json:"overrides,omitempty"`
	CreatedAt string      `json:"created_at"`
	UpdatedAt string      `json:"updated_at"`
}

type RouteStep struct {
	ID         int64  `json:"id"`
	Position   int    `json:"position"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Enabled    bool   `json:"enabled"`
	Label      string `json:"label,omitempty"`
}

type Override struct {
	RouteID    string `json:"route_id"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Disabled   bool   `json:"disabled"`
}

type ProxyKey struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"`
	Enabled    bool   `json:"enabled"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at"`
}

type ProxyKeyWithSecret struct {
	ProxyKey
	Token string `json:"token"`
}

type RequestLog struct {
	ID               string       `json:"id"`
	CreatedAt        string       `json:"created_at"`
	API              string       `json:"api"`
	RouteID          string       `json:"route_id"`
	ClientModel      string       `json:"client_model"`
	FinalModel       string       `json:"final_model"`
	Status           string       `json:"status"`
	HTTPStatus       int          `json:"http_status"`
	DurationMS       int64        `json:"duration_ms"`
	PromptTokens     int64        `json:"prompt_tokens"`
	CompletionTokens int64        `json:"completion_tokens"`
	TotalTokens      int64        `json:"total_tokens"`
	Error            string       `json:"error"`
	Attempts         []AttemptLog `json:"attempts,omitempty"`
}

type AttemptLog struct {
	ID         int64  `json:"id"`
	RequestID  string `json:"request_id"`
	Position   int    `json:"position"`
	ModelID    string `json:"model_id"`
	ProviderID string `json:"provider_id"`
	Status     string `json:"status"`
	HTTPStatus int    `json:"http_status"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error"`
}

func Open(path, secretKey string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, secretKey: secretKey}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA foreign_keys=ON`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS admin_users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS admin_sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY(user_id) REFERENCES admin_users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS providers (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			base_url TEXT NOT NULL,
			api_key_cipher TEXT NOT NULL,
			extra_headers_json TEXT NOT NULL DEFAULT '{}',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS models (
			internal_id TEXT PRIMARY KEY,
			provider_id TEXT NOT NULL,
			original_id TEXT NOT NULL,
			display_name TEXT NOT NULL,
			supports_chat INTEGER NOT NULL DEFAULT 1,
			supports_responses INTEGER NOT NULL DEFAULT 0,
			supports_stream INTEGER NOT NULL DEFAULT 1,
			context_length INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			auto_disabled INTEGER NOT NULL DEFAULT 0,
			auto_disabled_reason TEXT NOT NULL DEFAULT '',
			fail_count INTEGER NOT NULL DEFAULT 0,
			window_start TEXT NOT NULL DEFAULT '',
			last_failure_at TEXT NOT NULL DEFAULT '',
			cooldown_until TEXT NOT NULL DEFAULT '',
			cooldown_count INTEGER NOT NULL DEFAULT 0,
			upstream_error_status INTEGER NOT NULL DEFAULT 0,
			upstream_error_at TEXT NOT NULL DEFAULT '',
			upstream_error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(provider_id, original_id),
			FOREIGN KEY(provider_id) REFERENCES providers(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS model_groups (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			strategy TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			cursor INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS model_group_members (
			group_id TEXT NOT NULL,
			model_id TEXT NOT NULL,
			position INTEGER NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			PRIMARY KEY(group_id, model_id),
			FOREIGN KEY(group_id) REFERENCES model_groups(id) ON DELETE CASCADE,
			FOREIGN KEY(model_id) REFERENCES models(internal_id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS routes (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS route_steps (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			route_id TEXT NOT NULL,
			position INTEGER NOT NULL,
			target_type TEXT NOT NULL,
			target_id TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			FOREIGN KEY(route_id) REFERENCES routes(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS route_overrides (
			route_id TEXT NOT NULL,
			target_type TEXT NOT NULL,
			target_id TEXT NOT NULL,
			disabled INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY(route_id, target_type, target_id),
			FOREIGN KEY(route_id) REFERENCES routes(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS proxy_keys (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			key_hash TEXT NOT NULL UNIQUE,
			key_cipher TEXT NOT NULL DEFAULT '',
			prefix TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			last_used_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS request_logs (
			id TEXT PRIMARY KEY,
			created_at TEXT NOT NULL,
			api TEXT NOT NULL,
			route_id TEXT NOT NULL,
			client_model TEXT NOT NULL,
			final_model TEXT NOT NULL,
			status TEXT NOT NULL,
			http_status INTEGER NOT NULL,
			duration_ms INTEGER NOT NULL,
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			error TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS attempt_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			request_id TEXT NOT NULL,
			position INTEGER NOT NULL,
			model_id TEXT NOT NULL,
			provider_id TEXT NOT NULL,
			status TEXT NOT NULL,
			http_status INTEGER NOT NULL,
			duration_ms INTEGER NOT NULL,
			error TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(request_id) REFERENCES request_logs(id) ON DELETE CASCADE
		)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if err := s.ensureProxyKeyCipherColumn(ctx); err != nil {
		return err
	}
	if err := s.ensureModelCooldownColumns(ctx); err != nil {
		return err
	}
	defaults := map[string]string{
		"models_expose_raw":        "false",
		"log_retention_days":       "30",
		"auto_disable_models":      "true",
		"upstream_timeout_seconds": "20",
	}
	for key, value := range defaults {
		if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO settings(key, value) VALUES(?, ?)`, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ensureProxyKeyCipherColumn(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(proxy_keys)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == "key_cipher" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `ALTER TABLE proxy_keys ADD COLUMN key_cipher TEXT NOT NULL DEFAULT ''`)
	return err
}

func (s *Store) ensureModelCooldownColumns(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(models)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if !columns["cooldown_until"] {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE models ADD COLUMN cooldown_until TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if !columns["cooldown_count"] {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE models ADD COLUMN cooldown_count INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if !columns["upstream_error_status"] {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE models ADD COLUMN upstream_error_status INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if !columns["upstream_error_at"] {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE models ADD COLUMN upstream_error_at TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if !columns["upstream_error"] {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE models ADD COLUMN upstream_error TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) EnsureAdminUser() (string, error) {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM admin_users`).Scan(&count); err != nil {
		return "", err
	}
	if count > 0 {
		return "", nil
	}
	password, err := randomToken(18)
	if err != nil {
		return "", err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return "", err
	}
	now := nowString()
	_, err = s.db.Exec(`INSERT INTO admin_users(id, username, password_hash, created_at, updated_at) VALUES(?, 'admin', ?, ?, ?)`, newID("adm"), hash, now, now)
	return password, err
}

func (s *Store) LoginAdmin(username, password string) (string, error) {
	var id, passwordHash string
	if err := s.db.QueryRow(`SELECT id, password_hash FROM admin_users WHERE username = ?`, username).Scan(&id, &passwordHash); err != nil {
		return "", errors.New("用户名或密码错误")
	}
	if !verifyPassword(passwordHash, password) {
		return "", errors.New("用户名或密码错误")
	}
	token, err := randomToken(36)
	if err != nil {
		return "", err
	}
	now := nowString()
	expires := time.Now().Add(7 * 24 * time.Hour).UTC().Format(time.RFC3339)
	_, err = s.db.Exec(`INSERT INTO admin_sessions(id, user_id, token_hash, expires_at, created_at) VALUES(?, ?, ?, ?, ?)`, newID("ses"), id, sha256Hex(token), expires, now)
	return token, err
}

func (s *Store) ValidateAdminSession(token string) bool {
	if token == "" {
		return false
	}
	var expires string
	err := s.db.QueryRow(`SELECT expires_at FROM admin_sessions WHERE token_hash = ?`, sha256Hex(token)).Scan(&expires)
	if err != nil {
		return false
	}
	t, err := time.Parse(time.RFC3339, expires)
	return err == nil && time.Now().Before(t)
}

func (s *Store) DeleteAdminSession(token string) {
	_, _ = s.db.Exec(`DELETE FROM admin_sessions WHERE token_hash = ?`, sha256Hex(token))
}

func (s *Store) ChangeAdminPassword(oldPassword, newPassword string) error {
	var id, passwordHash string
	if err := s.db.QueryRow(`SELECT id, password_hash FROM admin_users WHERE username = 'admin'`).Scan(&id, &passwordHash); err != nil {
		return err
	}
	if !verifyPassword(passwordHash, oldPassword) {
		return errors.New("旧密码不正确")
	}
	hash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE admin_users SET password_hash = ?, updated_at = ? WHERE id = ?`, hash, nowString(), id)
	return err
}

func (s *Store) GetSetting(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	return value, err
}

func (s *Store) Settings() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		out[key] = value
	}
	return out, rows.Err()
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *Store) AutoDisableModelsEnabled() (bool, error) {
	value, err := s.GetSetting("auto_disable_models")
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return value != "false", nil
}

func (s *Store) UpstreamTimeout() (time.Duration, error) {
	value, err := s.GetSetting("upstream_timeout_seconds")
	if errors.Is(err, sql.ErrNoRows) || strings.TrimSpace(value) == "" {
		return 20 * time.Second, nil
	}
	if err != nil {
		return 0, err
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, fmt.Errorf("upstream_timeout_seconds 必须是秒数")
	}
	if seconds <= 0 {
		return 0, nil
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

func (s *Store) CreateProxyKey(name string) (ProxyKeyWithSecret, error) {
	token, err := proxyKeyToken()
	if err != nil {
		return ProxyKeyWithSecret{}, err
	}
	cipher, err := encryptSecret(s.secretKey, token)
	if err != nil {
		return ProxyKeyWithSecret{}, err
	}
	id := newID("pkey")
	key := ProxyKey{
		ID:        id,
		Name:      name,
		Prefix:    tokenPrefix(token),
		Enabled:   true,
		CreatedAt: nowString(),
	}
	_, err = s.db.Exec(`INSERT INTO proxy_keys(id, name, key_hash, key_cipher, prefix, enabled, created_at) VALUES(?, ?, ?, ?, ?, 1, ?)`, key.ID, key.Name, sha256Hex(token), cipher, key.Prefix, key.CreatedAt)
	if err != nil {
		return ProxyKeyWithSecret{}, err
	}
	return ProxyKeyWithSecret{ProxyKey: key, Token: token}, nil
}

func (s *Store) ValidateProxyKey(token string) bool {
	if token == "" {
		return false
	}
	now := nowString()
	res, err := s.db.Exec(`UPDATE proxy_keys SET last_used_at = ? WHERE key_hash = ? AND enabled = 1`, now, sha256Hex(token))
	if err != nil {
		return false
	}
	affected, _ := res.RowsAffected()
	return affected == 1
}

func (s *Store) ListProxyKeys() ([]ProxyKey, error) {
	rows, err := s.db.Query(`SELECT id, name, prefix, enabled, created_at, last_used_at FROM proxy_keys ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []ProxyKey{}
	for rows.Next() {
		var key ProxyKey
		if err := rows.Scan(&key.ID, &key.Name, &key.Prefix, &key.Enabled, &key.CreatedAt, &key.LastUsedAt); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *Store) GetProxyKeyToken(id string) (string, error) {
	var cipher string
	if err := s.db.QueryRow(`SELECT key_cipher FROM proxy_keys WHERE id = ?`, id).Scan(&cipher); err != nil {
		return "", err
	}
	if cipher == "" {
		return "", errors.New("这个旧密钥没有保存完整内容，请重新创建")
	}
	return decryptSecret(s.secretKey, cipher)
}

func (s *Store) SetProxyKeyEnabled(id string, enabled bool) error {
	_, err := s.db.Exec(`UPDATE proxy_keys SET enabled = ? WHERE id = ?`, boolInt(enabled), id)
	return err
}

func (s *Store) DeleteProxyKey(id string) error {
	_, err := s.db.Exec(`DELETE FROM proxy_keys WHERE id = ?`, id)
	return err
}

func newID(prefix string) string {
	token, err := randomToken(12)
	if err != nil {
		panic(err)
	}
	return prefix + "_" + strings.ToLower(token)
}

func proxyKeyToken() (string, error) {
	token, err := randomToken(32)
	if err != nil {
		return "", err
	}
	return "sk-" + token, nil
}

func nowString() string {
	return timeNow().Format(timeFormat)
}

const timeFormat = time.RFC3339

const (
	modelFailureWindow    = 10 * time.Minute
	modelFailureThreshold = 5
	modelCooldownDuration = time.Hour
	modelCooldownLimit    = 3
)

func timeNow() time.Time {
	return time.Now().UTC()
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(timeFormat, value)
}

func (m Model) CoolingDown() bool {
	if m.CooldownUntil == "" {
		return false
	}
	until, err := parseTime(m.CooldownUntil)
	return err == nil && timeNow().Before(until)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func tokenPrefix(token string) string {
	if len(token) <= 8 {
		return token
	}
	return token[:8]
}

func internalModelID(providerID, originalID string) string {
	return providerID + "/" + originalID
}

func InternalModelID(providerID, originalID string) string {
	return internalModelID(providerID, originalID)
}

func marshalHeaders(headers map[string]string) (string, error) {
	if headers == nil {
		headers = map[string]string{}
	}
	payload, err := json.Marshal(headers)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func unmarshalHeaders(payload string) map[string]string {
	out := map[string]string{}
	_ = json.Unmarshal([]byte(payload), &out)
	return out
}
