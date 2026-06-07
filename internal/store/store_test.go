package store

import (
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

func TestAutoDisableAfterFiveFailures(t *testing.T) {
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
	for i := 0; i < 5; i++ {
		if err := s.RecordModelFailure(model.InternalID, "boom"); err != nil {
			t.Fatal(err)
		}
	}
	updated, err := s.GetModel(model.InternalID)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.AutoDisabled {
		t.Fatal("model should be auto disabled")
	}
	if err := s.RestoreModel(model.InternalID); err != nil {
		t.Fatal(err)
	}
	restored, err := s.GetModel(model.InternalID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.AutoDisabled || restored.FailCount != 0 {
		t.Fatal("model should be restored")
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
