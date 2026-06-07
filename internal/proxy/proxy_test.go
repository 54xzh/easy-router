package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"easy-router/internal/store"
)

func TestConvertResponsesToChat(t *testing.T) {
	got, err := convertResponsesToChat(map[string]any{
		"model":             "gpt-4o",
		"instructions":      "You are concise.",
		"input":             "Hello",
		"stream":            true,
		"max_output_tokens": float64(128),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["model"] != "gpt-4o" {
		t.Fatalf("model was not preserved")
	}
	if got["max_tokens"] != float64(128) {
		t.Fatalf("max_output_tokens was not mapped")
	}
	messages, ok := got["messages"].([]map[string]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("unexpected messages: %#v", got["messages"])
	}
	if messages[0]["role"] != "system" || messages[1]["role"] != "user" {
		t.Fatalf("unexpected message roles: %#v", messages)
	}
}

func TestConvertResponsesRejectsUnknownFields(t *testing.T) {
	_, err := convertResponsesToChat(map[string]any{
		"model": "gpt-4o",
		"input": "Hello",
		"text":  map[string]any{"format": "json_schema"},
	})
	if err == nil {
		t.Fatal("expected unsupported field error")
	}
}

func TestCanceledRequestDoesNotRecordFailureOrTryNextModel(t *testing.T) {
	started := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-releaseFirst
	}))
	defer firstUpstream.Close()
	defer close(releaseFirst)
	secondCalled := make(chan struct{}, 1)
	secondUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondCalled <- struct{}{}
		writeJSON(w, http.StatusOK, map[string]any{"id": "second"})
	}))
	defer secondUpstream.Close()

	s := newProxyTestStore(t)
	first := addProxyTestModel(t, s, "p1", "first", firstUpstream.URL)
	second := addProxyTestModel(t, s, "p2", "second", secondUpstream.URL)
	addProxyTestRoute(t, s, "coder-fast", first.InternalID, second.InternalID)

	h := &Handler{store: s, client: &http.Client{Timeout: time.Second}}
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"coder-fast","messages":[]}`)).WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		h.completion(w, req, "chat")
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first upstream was not requested")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("completion did not return after cancel")
	}

	if w.Code != statusClientClosedRequest {
		t.Fatalf("unexpected status: %d", w.Code)
	}
	select {
	case <-secondCalled:
		t.Fatal("second upstream should not be requested after cancel")
	default:
	}
	firstAfter, err := s.GetModel(first.InternalID)
	if err != nil {
		t.Fatal(err)
	}
	secondAfter, err := s.GetModel(second.InternalID)
	if err != nil {
		t.Fatal(err)
	}
	if firstAfter.FailCount != 0 || secondAfter.FailCount != 0 {
		t.Fatalf("canceled request should not record failures: first=%d second=%d", firstAfter.FailCount, secondAfter.FailCount)
	}
	logs, err := s.ListLogs(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || len(logs[0].Attempts) != 1 {
		t.Fatalf("expected one logged attempt, got logs=%d attempts=%d", len(logs), len(logs[0].Attempts))
	}
	if logs[0].Attempts[0].ModelID != first.InternalID || logs[0].Attempts[0].Status != "canceled" {
		t.Fatalf("unexpected attempt: %#v", logs[0].Attempts[0])
	}
}

func TestUpstreamTimeoutFallsBackToNextModel(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		writeJSON(w, http.StatusOK, map[string]any{"id": "slow"})
	}))
	defer slow.Close()
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id":      "fast",
			"choices": []any{},
			"usage": map[string]any{
				"prompt_tokens":     1,
				"completion_tokens": 1,
				"total_tokens":      2,
			},
		})
	}))
	defer fast.Close()

	s := newProxyTestStore(t)
	if err := s.SetSetting("upstream_timeout_seconds", "0.05"); err != nil {
		t.Fatal(err)
	}
	first := addProxyTestModel(t, s, "p1", "first", slow.URL)
	second := addProxyTestModel(t, s, "p2", "second", fast.URL)
	addProxyTestRoute(t, s, "coder-fast", first.InternalID, second.InternalID)

	h := &Handler{store: s, client: &http.Client{Timeout: time.Second}}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"coder-fast","messages":[]}`))
	w := httptest.NewRecorder()

	h.completion(w, req, "chat")

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	firstAfter, err := s.GetModel(first.InternalID)
	if err != nil {
		t.Fatal(err)
	}
	if firstAfter.FailCount != 1 {
		t.Fatalf("timed out model should record one failure, got %d", firstAfter.FailCount)
	}
	logs, err := s.ListLogs(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || len(logs[0].Attempts) != 2 {
		t.Fatalf("expected two attempts, got logs=%d attempts=%d", len(logs), len(logs[0].Attempts))
	}
	if logs[0].Attempts[0].HTTPStatus != http.StatusGatewayTimeout || logs[0].Attempts[0].Status != "failed" {
		t.Fatalf("unexpected first attempt: %#v", logs[0].Attempts[0])
	}
	if logs[0].FinalModel != second.InternalID || logs[0].Attempts[1].Status != "success" {
		t.Fatalf("fallback did not finish on second model: %#v", logs[0])
	}
}

func TestAutoDisableSettingOffDoesNotRecordFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "boom"})
	}))
	defer upstream.Close()

	s := newProxyTestStore(t)
	if err := s.SetSetting("auto_disable_models", "false"); err != nil {
		t.Fatal(err)
	}
	model := addProxyTestModel(t, s, "p1", "first", upstream.URL)
	addProxyTestRoute(t, s, "coder-fast", model.InternalID)

	h := &Handler{store: s, client: &http.Client{Timeout: time.Second}}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"coder-fast","messages":[]}`))
	w := httptest.NewRecorder()

	h.completion(w, req, "chat")

	if w.Code != http.StatusBadGateway {
		t.Fatalf("unexpected status: %d", w.Code)
	}
	updated, err := s.GetModel(model.InternalID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.FailCount != 0 || updated.AutoDisabled {
		t.Fatalf("auto disable off should not record failure: fail=%d disabled=%v", updated.FailCount, updated.AutoDisabled)
	}
	if updated.CooldownCount != 0 || updated.CooldownUntil != "" {
		t.Fatalf("auto disable off should not record cooldown: count=%d until=%q", updated.CooldownCount, updated.CooldownUntil)
	}
}

func TestCooldownModelSkippedWhenAutoDisableEnabled(t *testing.T) {
	coolingCalled := make(chan struct{}, 1)
	coolingUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coolingCalled <- struct{}{}
		writeJSON(w, http.StatusOK, map[string]any{"id": "cooling", "choices": []any{}})
	}))
	defer coolingUpstream.Close()
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"id": "fast", "choices": []any{}})
	}))
	defer fast.Close()

	s := newProxyTestStore(t)
	cooling := addProxyTestModel(t, s, "p1", "first", coolingUpstream.URL)
	second := addProxyTestModel(t, s, "p2", "second", fast.URL)
	for i := 0; i < 5; i++ {
		if err := s.RecordModelFailure(cooling.InternalID, "boom"); err != nil {
			t.Fatal(err)
		}
	}
	addProxyTestRoute(t, s, "coder-fast", cooling.InternalID, second.InternalID)

	h := &Handler{store: s, client: &http.Client{Timeout: time.Second}}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"coder-fast","messages":[]}`))
	w := httptest.NewRecorder()

	h.completion(w, req, "chat")

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	select {
	case <-coolingCalled:
		t.Fatal("cooling model should not be requested")
	default:
	}
	logs, err := s.ListLogs(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || len(logs[0].Attempts) != 1 {
		t.Fatalf("expected one attempt, got logs=%d attempts=%d", len(logs), len(logs[0].Attempts))
	}
	if logs[0].Attempts[0].ModelID != second.InternalID {
		t.Fatalf("cooldown should skip to second model: %#v", logs[0].Attempts[0])
	}
}

func TestAutoDisableSettingOffUsesCoolingModel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"id": "ok", "choices": []any{}})
	}))
	defer upstream.Close()

	s := newProxyTestStore(t)
	model := addProxyTestModel(t, s, "p1", "first", upstream.URL)
	for i := 0; i < 5; i++ {
		if err := s.RecordModelFailure(model.InternalID, "boom"); err != nil {
			t.Fatal(err)
		}
	}
	cooling, err := s.GetModel(model.InternalID)
	if err != nil {
		t.Fatal(err)
	}
	if cooling.CooldownCount != 1 || !cooling.CoolingDown() {
		t.Fatalf("model should be cooling down before setting is disabled: %#v", cooling)
	}
	if err := s.SetSetting("auto_disable_models", "false"); err != nil {
		t.Fatal(err)
	}
	addProxyTestRoute(t, s, "coder-fast", model.InternalID)

	h := &Handler{store: s, client: &http.Client{Timeout: time.Second}}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"coder-fast","messages":[]}`))
	w := httptest.NewRecorder()

	h.completion(w, req, "chat")

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	updated, err := s.GetModel(model.InternalID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.AutoDisabled || updated.CooldownCount != 0 || updated.CooldownUntil != "" {
		t.Fatalf("success should clear old auto-disable state: %#v", updated)
	}
}

func TestUpstreamReturned499FallsBackToNextModel(t *testing.T) {
	canceledUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, statusClientClosedRequest, map[string]any{"error": "context canceled"})
	}))
	defer canceledUpstream.Close()
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"id": "fast", "choices": []any{}})
	}))
	defer fast.Close()

	s := newProxyTestStore(t)
	first := addProxyTestModel(t, s, "p1", "first", canceledUpstream.URL)
	second := addProxyTestModel(t, s, "p2", "second", fast.URL)
	addProxyTestRoute(t, s, "coder-fast", first.InternalID, second.InternalID)

	h := &Handler{store: s, client: &http.Client{Timeout: time.Second}}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"coder-fast","messages":[]}`))
	w := httptest.NewRecorder()

	h.completion(w, req, "chat")

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	logs, err := s.ListLogs(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || len(logs[0].Attempts) != 2 {
		t.Fatalf("expected fallback after upstream 499, got logs=%d attempts=%d", len(logs), len(logs[0].Attempts))
	}
	if logs[0].Attempts[0].HTTPStatus != statusClientClosedRequest || logs[0].Attempts[0].Status != "failed" {
		t.Fatalf("unexpected first attempt: %#v", logs[0].Attempts[0])
	}
}

func TestUpstreamTransportContextCanceledFallsBackToNextModel(t *testing.T) {
	s := newProxyTestStore(t)
	first := addProxyTestModel(t, s, "p1", "first", "https://first.test")
	second := addProxyTestModel(t, s, "p2", "second", "https://second.test")
	addProxyTestRoute(t, s, "coder-fast", first.InternalID, second.InternalID)

	h := &Handler{
		store: s,
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "first.test" {
					return nil, context.Canceled
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"id":"fast","choices":[]}`)),
					Request:    req,
				}, nil
			}),
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"coder-fast","messages":[]}`))
	w := httptest.NewRecorder()

	h.completion(w, req, "chat")

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	logs, err := s.ListLogs(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || len(logs[0].Attempts) != 2 {
		t.Fatalf("expected fallback after upstream transport cancel, got logs=%d attempts=%d", len(logs), len(logs[0].Attempts))
	}
	if !strings.Contains(logs[0].Attempts[0].Error, "上游连接取消") {
		t.Fatalf("unexpected first attempt error: %s", logs[0].Attempts[0].Error)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newProxyTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:", "a-long-test-master-key")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = s.Close()
	})
	return s
}

func addProxyTestModel(t *testing.T, s *store.Store, providerID, originalID, baseURL string) store.Model {
	t.Helper()
	if _, err := s.UpsertProvider(store.Provider{
		ID:      providerID,
		Name:    providerID,
		BaseURL: baseURL,
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	model, err := s.UpsertModel(store.Model{
		ProviderID:        providerID,
		OriginalID:        originalID,
		DisplayName:       originalID,
		SupportsChat:      true,
		SupportsResponses: true,
		SupportsStream:    true,
		Enabled:           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return model
}

func addProxyTestRoute(t *testing.T, s *store.Store, routeID string, modelIDs ...string) {
	t.Helper()
	steps := make([]store.RouteStep, 0, len(modelIDs))
	for i, modelID := range modelIDs {
		steps = append(steps, store.RouteStep{
			Position:   i + 1,
			TargetType: "model",
			TargetID:   modelID,
			Enabled:    true,
		})
	}
	if _, err := s.UpsertRoute(store.Route{
		ID:      routeID,
		Name:    routeID,
		Enabled: true,
		Steps:   steps,
	}); err != nil {
		t.Fatal(err)
	}
}
