package store

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInternalModelID(t *testing.T) {
	got := InternalModelID("openai", "gpt-4o")
	if got != "openai/gpt-4o" {
		t.Fatalf("unexpected internal id: %s", got)
	}
}

func TestEncryptDecryptSecret(t *testing.T) {
	master := "a-long-test-master-key"
	ciphertext, err := encryptSecret(master, "sk-test")
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := decryptSecret(master, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if plaintext != "sk-test" {
		t.Fatalf("unexpected plaintext: %s", plaintext)
	}
}

func TestCreateProxyKeyStoresReusableSecret(t *testing.T) {
	s, err := Open(":memory:", "a-long-test-master-key")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	created, err := s.CreateProxyKey("默认客户端")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.Token, "sk-") {
		t.Fatalf("proxy key should start with sk-: %s", created.Token)
	}
	if !s.ValidateProxyKey(created.Token) {
		t.Fatal("created proxy key should validate")
	}

	keys, err := s.ListProxyKeys()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 {
		t.Fatalf("unexpected key count: %d", len(keys))
	}
	token, err := s.GetProxyKeyToken(keys[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if token != created.Token {
		t.Fatalf("stored token mismatch: %s", token)
	}
}

func TestModelCooldownBeforeAutoDisable(t *testing.T) {
	s, model := newStoreTestModel(t)
	recordStoreModelFailures(t, s, model.InternalID, modelFailureThreshold)

	updated, err := s.GetModel(model.InternalID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.AutoDisabled {
		t.Fatal("model should cool down before auto disable")
	}
	if updated.CooldownCount != 1 {
		t.Fatalf("unexpected cooldown count: %d", updated.CooldownCount)
	}
	if updated.CooldownUntil == "" || !updated.CoolingDown() {
		t.Fatalf("model should be cooling down: %#v", updated)
	}
	if updated.FailCount != 0 || updated.WindowStart != "" {
		t.Fatalf("failure window should be cleared after cooldown: fail=%d window=%q", updated.FailCount, updated.WindowStart)
	}

	if err := s.RestoreModel(model.InternalID); err != nil {
		t.Fatal(err)
	}
	restored, err := s.GetModel(model.InternalID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.AutoDisabled || restored.FailCount != 0 || restored.CooldownCount != 0 || restored.CooldownUntil != "" {
		t.Fatal("model should be restored")
	}
}

func TestModelCooldownEscalatesToAutoDisable(t *testing.T) {
	s, model := newStoreTestModel(t)

	for cycle := 1; cycle <= modelCooldownLimit; cycle++ {
		recordStoreModelFailures(t, s, model.InternalID, modelFailureThreshold)
		updated, err := s.GetModel(model.InternalID)
		if err != nil {
			t.Fatal(err)
		}
		if updated.CooldownCount != cycle {
			t.Fatalf("cycle %d should set cooldown count %d, got %d", cycle, cycle, updated.CooldownCount)
		}
		if cycle < modelCooldownLimit {
			if updated.AutoDisabled {
				t.Fatalf("cycle %d should not auto disable", cycle)
			}
			if updated.CooldownUntil == "" || !updated.CoolingDown() {
				t.Fatalf("cycle %d should cool down: %#v", cycle, updated)
			}
			expireStoreModelCooldown(t, s, model.InternalID)
			continue
		}
		if !updated.AutoDisabled {
			t.Fatal("third cooldown trigger should auto disable")
		}
		if updated.CooldownUntil != "" {
			t.Fatalf("auto disabled model should not keep cooldown_until: %q", updated.CooldownUntil)
		}
	}
}

func TestModelSuccessClearsCooldownCount(t *testing.T) {
	s, model := newStoreTestModel(t)
	recordStoreModelFailures(t, s, model.InternalID, modelFailureThreshold)
	expireStoreModelCooldown(t, s, model.InternalID)

	if err := s.RecordModelSuccess(model.InternalID); err != nil {
		t.Fatal(err)
	}
	updated, err := s.GetModel(model.InternalID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.AutoDisabled || updated.FailCount != 0 || updated.CooldownCount != 0 || updated.CooldownUntil != "" {
		t.Fatalf("success should clear cooldown state: %#v", updated)
	}
}

func TestModelUpstreamErrorPersistsUntilRestore(t *testing.T) {
	s, model := newStoreTestModel(t)
	if err := s.RecordModelUpstreamError(model.InternalID, 401, "上游返回 401：invalid key"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordModelSuccess(model.InternalID); err != nil {
		t.Fatal(err)
	}
	updated, err := s.GetModel(model.InternalID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.UpstreamErrorStatus != 401 || updated.UpstreamError == "" || updated.UpstreamErrorAt == "" {
		t.Fatalf("upstream error should stay until manual clear: %#v", updated)
	}
	if err := s.RestoreModel(model.InternalID); err != nil {
		t.Fatal(err)
	}
	restored, err := s.GetModel(model.InternalID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.UpstreamErrorStatus != 0 || restored.UpstreamError != "" || restored.UpstreamErrorAt != "" {
		t.Fatalf("restore should clear upstream error: %#v", restored)
	}
}

func TestMigrationAddsModelStateColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE models (
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
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`)
	if closeErr := db.Close(); err != nil {
		t.Fatal(err)
	} else if closeErr != nil {
		t.Fatal(closeErr)
	}

	s, err := Open(path, "a-long-test-master-key")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	columns := storeTableColumns(t, s, "models")
	if !columns["cooldown_until"] || !columns["cooldown_count"] || !columns["upstream_error_status"] || !columns["upstream_error_at"] || !columns["upstream_error"] {
		t.Fatalf("model state columns should be added: %#v", columns)
	}
}

func TestMultiKeyProviderCreatesHiddenKeyAndSyncsModels(t *testing.T) {
	s, err := Open(":memory:", "a-long-test-master-key")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	provider, err := s.UpsertProvider(Provider{
		ID:              "openai",
		Name:            "OpenAI",
		BaseURL:         "https://api.openai.com/v1",
		APIKey:          "sk-one",
		Enabled:         true,
		MultiKeyEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !provider.MultiKeyEnabled || provider.MultiKeyStrategy != "round_robin" {
		t.Fatalf("unexpected provider: %#v", provider)
	}
	keys, err := s.ListProviderKeys("openai")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].Name != "Key 1" || keys[0].Prefix != "sk-one" {
		t.Fatalf("unexpected migrated key: %#v", keys)
	}

	model, err := s.UpsertModel(Model{
		ProviderID:        "openai",
		OriginalID:        "gpt-4o",
		DisplayName:       "GPT 4o",
		SupportsChat:      true,
		SupportsResponses: true,
		SupportsStream:    true,
		ContextLength:     128000,
		Enabled:           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := s.GetModel(InternalModelID(keys[0].ID, "gpt-4o"))
	if err != nil {
		t.Fatal(err)
	}
	if hidden.DisplayName != model.DisplayName || hidden.ContextLength != model.ContextLength || !hidden.Enabled {
		t.Fatalf("hidden model should copy manual fields: %#v", hidden)
	}

	model.DisplayName = "GPT 4o updated"
	model.ContextLength = 64000
	model.Enabled = false
	if _, err := s.UpsertModel(model); err != nil {
		t.Fatal(err)
	}
	hidden, err = s.GetModel(hidden.InternalID)
	if err != nil {
		t.Fatal(err)
	}
	if hidden.DisplayName != "GPT 4o updated" || hidden.ContextLength != 64000 || hidden.Enabled {
		t.Fatalf("hidden model should sync manual updates: %#v", hidden)
	}

	recordStoreModelFailures(t, s, hidden.InternalID, 1)
	root, err := s.GetModel(model.InternalID)
	if err != nil {
		t.Fatal(err)
	}
	hidden, err = s.GetModel(hidden.InternalID)
	if err != nil {
		t.Fatal(err)
	}
	if root.FailCount != 0 || hidden.FailCount != 1 {
		t.Fatalf("hidden failure state should stay independent: root=%#v hidden=%#v", root, hidden)
	}
}

func TestDisableMultiKeyKeepsHiddenKeysAndCopiesFirstEnabledKey(t *testing.T) {
	s, err := Open(":memory:", "a-long-test-master-key")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.UpsertProvider(Provider{
		ID:              "openai",
		Name:            "OpenAI",
		BaseURL:         "https://api.openai.com/v1",
		APIKey:          "sk-first",
		Enabled:         true,
		MultiKeyEnabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddProviderKey("openai", ProviderKey{Name: "Key 2", APIKey: "sk-second"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertProvider(Provider{
		ID:              "openai",
		Name:            "OpenAI",
		BaseURL:         "https://api.openai.com/v1",
		Enabled:         true,
		MultiKeyEnabled: false,
	}); err != nil {
		t.Fatal(err)
	}

	provider, err := s.GetProvider("openai", true)
	if err != nil {
		t.Fatal(err)
	}
	if provider.APIKey != "sk-first" || provider.MultiKeyEnabled {
		t.Fatalf("single-key provider should use first enabled key: %#v", provider)
	}
	keys, err := s.ListProviderKeys("openai")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("hidden keys should be preserved, got %d", len(keys))
	}
}

func TestListReadsDoNotBlockWithNestedData(t *testing.T) {
	s, err := Open(":memory:", "a-long-test-master-key")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.UpsertProvider(Provider{
		ID:      "openai",
		Name:    "OpenAI",
		BaseURL: "https://api.openai.com/v1",
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	model, err := s.UpsertModel(Model{
		ProviderID:     "openai",
		OriginalID:     "gpt-4o",
		DisplayName:    "gpt-4o",
		SupportsChat:   true,
		SupportsStream: true,
		Enabled:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	group, err := s.UpsertGroup(ModelGroup{
		Name:     "gpt组",
		Strategy: "fallback",
		Enabled:  true,
		Members: []ModelGroupMember{{
			ModelID:  model.InternalID,
			Position: 1,
			Enabled:  true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertRoute(Route{
		ID:      "coder-fast",
		Name:    "coder-fast",
		Enabled: true,
		Steps: []RouteStep{{
			Position:   1,
			TargetType: "group",
			TargetID:   group.ID,
			Enabled:    true,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddRequestLog(RequestLog{
		ID:          "req_test",
		API:         "chat",
		RouteID:     "coder-fast",
		ClientModel: "coder-fast",
		FinalModel:  model.InternalID,
		Status:      "success",
		HTTPStatus:  200,
		Attempts: []AttemptLog{{
			ModelID:    model.InternalID,
			ProviderID: "openai",
			Status:     "success",
			HTTPStatus: 200,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		if _, err := s.ListGroups(); err != nil {
			done <- err
			return
		}
		if _, err := s.ListRoutes(); err != nil {
			done <- err
			return
		}
		if _, err := s.EnabledRoutes(); err != nil {
			done <- err
			return
		}
		if _, err := s.ListLogs(10); err != nil {
			done <- err
			return
		}
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("list reads blocked")
	}
}

func TestSetRouteOverrideFalseClearsRecord(t *testing.T) {
	s, err := Open(":memory:", "a-long-test-master-key")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.UpsertRoute(Route{
		ID:      "coder-fast",
		Name:    "coder-fast",
		Enabled: true,
		Steps: []RouteStep{{
			Position:   1,
			TargetType: "model",
			TargetID:   "openai/gpt-4o",
			Enabled:    true,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	override := Override{
		RouteID:    "coder-fast",
		TargetType: "model",
		TargetID:   "openai/gpt-4o",
		Disabled:   true,
	}
	if err := s.SetRouteOverride(override); err != nil {
		t.Fatal(err)
	}
	route, err := s.GetRoute("coder-fast")
	if err != nil {
		t.Fatal(err)
	}
	if len(route.Overrides) != 1 {
		t.Fatalf("expected one override, got %d", len(route.Overrides))
	}

	override.Disabled = false
	if err := s.SetRouteOverride(override); err != nil {
		t.Fatal(err)
	}
	route, err = s.GetRoute("coder-fast")
	if err != nil {
		t.Fatal(err)
	}
	if len(route.Overrides) != 0 {
		t.Fatalf("expected override to be cleared, got %d", len(route.Overrides))
	}
}

func newStoreTestModel(t *testing.T) (*Store, Model) {
	t.Helper()
	s, err := Open(":memory:", "a-long-test-master-key")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = s.Close()
	})
	if _, err := s.UpsertProvider(Provider{
		ID:      "openai",
		Name:    "OpenAI",
		BaseURL: "https://api.openai.com/v1",
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	model, err := s.UpsertModel(Model{
		ProviderID:        "openai",
		OriginalID:        "gpt-4o",
		DisplayName:       "gpt-4o",
		SupportsChat:      true,
		SupportsStream:    true,
		SupportsResponses: true,
		Enabled:           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, model
}

func recordStoreModelFailures(t *testing.T, s *Store, modelID string, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		if err := s.RecordModelFailure(modelID, "boom"); err != nil {
			t.Fatal(err)
		}
	}
}

func expireStoreModelCooldown(t *testing.T, s *Store, modelID string) {
	t.Helper()
	expired := timeNow().Add(-time.Minute).Format(timeFormat)
	if _, err := s.db.Exec(`UPDATE models SET cooldown_until = ? WHERE internal_id = ?`, expired, modelID); err != nil {
		t.Fatal(err)
	}
}

func storeTableColumns(t *testing.T, s *Store, table string) map[string]bool {
	t.Helper()
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return columns
}
