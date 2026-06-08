package proxy

import (
	"context"
	"encoding/json"
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

func TestConvertResponsesToChatWithToolsAndImage(t *testing.T) {
	got, err := convertResponsesToChat(map[string]any{
		"model": "gpt-4o",
		"tools": []any{map[string]any{
			"type":        "function",
			"name":        "lookup",
			"description": "Lookup data",
			"parameters":  map[string]any{"type": "object"},
		}},
		"tool_choice": map[string]any{"type": "function", "name": "lookup"},
		"input": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "what is this?"},
					map[string]any{"type": "input_image", "image_url": "data:image/png;base64,abc", "detail": "low"},
				},
			},
			map[string]any{"type": "function_call", "call_id": "call_1", "name": "lookup", "arguments": `{"id":"1"}`},
			map[string]any{"type": "function_call_output", "call_id": "call_1", "output": `{"ok":true}`},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	tools, ok := got["tools"].([]map[string]any)
	if !ok || tools[0]["function"].(map[string]any)["name"] != "lookup" {
		t.Fatalf("unexpected tools: %#v", got["tools"])
	}
	choice := got["tool_choice"].(map[string]any)
	if choice["function"].(map[string]any)["name"] != "lookup" {
		t.Fatalf("unexpected tool_choice: %#v", choice)
	}
	messages, ok := got["messages"].([]map[string]any)
	if !ok || len(messages) != 3 {
		t.Fatalf("unexpected messages: %#v", got["messages"])
	}
	content, ok := messages[0]["content"].([]map[string]any)
	if !ok || content[1]["type"] != "image_url" {
		t.Fatalf("image was not converted: %#v", messages[0]["content"])
	}
	if messages[1]["role"] != "assistant" || messages[2]["role"] != "tool" {
		t.Fatalf("tool call history was not converted: %#v", messages)
	}
}

func TestConvertChatToResponses(t *testing.T) {
	got, err := convertChatToResponses(map[string]any{
		"model":          "gpt-4o",
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
		"max_tokens":     float64(128),
		"messages": []any{
			map[string]any{"role": "system", "content": "You are concise."},
			map[string]any{"role": "user", "content": "Hello"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["model"] != "gpt-4o" {
		t.Fatalf("model was not preserved")
	}
	if got["max_output_tokens"] != float64(128) {
		t.Fatalf("max_tokens was not mapped")
	}
	if _, ok := got["stream_options"]; ok {
		t.Fatalf("stream_options should not be sent to Responses upstream: %#v", got)
	}
	if got["instructions"] != "You are concise." {
		t.Fatalf("unexpected instructions: %#v", got["instructions"])
	}
	input, ok := got["input"].([]map[string]any)
	if !ok || len(input) != 1 || input[0]["role"] != "user" {
		t.Fatalf("unexpected input: %#v", got["input"])
	}
}

func TestConvertChatRejectsUnknownFields(t *testing.T) {
	_, err := convertChatToResponses(map[string]any{
		"model":           "gpt-4o",
		"messages":        []any{},
		"response_format": map[string]any{"type": "json_object"},
	})
	if err == nil {
		t.Fatal("expected unsupported field error")
	}
}

func TestConvertChatToResponsesWithToolsAndImage(t *testing.T) {
	got, err := convertChatToResponses(map[string]any{
		"model":            "gpt-4o",
		"reasoning_effort": "medium",
		"tools": []any{map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "lookup",
				"description": "Lookup data",
				"parameters":  map[string]any{"type": "object"},
			},
		}},
		"tool_choice": map[string]any{"type": "function", "function": map[string]any{"name": "lookup"}},
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "what is this?"},
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,abc", "detail": "low"}},
				},
			},
			map[string]any{"role": "assistant", "content": "previous answer", "reasoning_content": "private reasoning"},
			map[string]any{
				"role":    "assistant",
				"content": nil,
				"tool_calls": []any{map[string]any{
					"id":   "call_1",
					"type": "function",
					"function": map[string]any{
						"name":      "lookup",
						"arguments": `{"id":"1"}`,
					},
				}},
			},
			map[string]any{"role": "tool", "tool_call_id": "call_1", "content": `{"ok":true}`},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	tools, ok := got["tools"].([]map[string]any)
	if !ok || tools[0]["name"] != "lookup" {
		t.Fatalf("unexpected tools: %#v", got["tools"])
	}
	choice := got["tool_choice"].(map[string]any)
	if choice["name"] != "lookup" {
		t.Fatalf("unexpected tool_choice: %#v", choice)
	}
	if got["reasoning"].(map[string]any)["effort"] != "medium" {
		t.Fatalf("reasoning_effort was not mapped: %#v", got["reasoning"])
	}
	input, ok := got["input"].([]map[string]any)
	if !ok || len(input) != 4 {
		t.Fatalf("unexpected input: %#v", got["input"])
	}
	content, ok := input[0]["content"].([]map[string]any)
	if !ok || content[1]["type"] != "input_image" {
		t.Fatalf("image was not converted: %#v", input[0]["content"])
	}
	if input[1]["role"] != "assistant" || input[1]["content"] != "previous answer" {
		t.Fatalf("reasoning_content should not break assistant history conversion: %#v", input)
	}
	if input[2]["type"] != "function_call" || input[3]["type"] != "function_call_output" {
		t.Fatalf("tool call history was not converted: %#v", input)
	}
}

func TestChatRequestUsesResponsesOnlyModel(t *testing.T) {
	var upstreamPath string
	var upstreamPayload map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&upstreamPayload); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":         "resp_1",
			"object":     "response",
			"created_at": float64(123),
			"model":      "upstream-responses",
			"output": []any{map[string]any{
				"type": "message",
				"role": "assistant",
				"content": []any{map[string]any{
					"type": "output_text",
					"text": "pong",
				}},
			}},
			"usage": map[string]any{
				"input_tokens":  float64(2),
				"output_tokens": float64(3),
				"total_tokens":  float64(5),
			},
		})
	}))
	defer upstream.Close()

	s := newProxyTestStore(t)
	model := addProxyTestModel(t, s, "p1", "upstream-responses", upstream.URL)
	model = setProxyTestModelSupport(t, s, model, false, true)
	addProxyTestRoute(t, s, "coder-fast", model.InternalID)

	h := &Handler{store: s, client: &http.Client{Timeout: time.Second}}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model":"coder-fast",
		"messages":[
			{"role":"system","content":"be brief"},
			{"role":"user","content":"ping"}
		],
		"max_tokens":16
	}`))
	w := httptest.NewRecorder()

	h.completion(w, req, "chat")

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	if upstreamPath != "/responses" {
		t.Fatalf("expected /responses upstream, got %q", upstreamPath)
	}
	if upstreamPayload["model"] != "upstream-responses" || upstreamPayload["instructions"] != "be brief" {
		t.Fatalf("unexpected upstream payload: %#v", upstreamPayload)
	}
	input, ok := upstreamPayload["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("unexpected upstream input: %#v", upstreamPayload["input"])
	}
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	choices, _ := response["choices"].([]any)
	if len(choices) != 1 {
		t.Fatalf("unexpected chat response: %#v", response)
	}
	message, _ := choices[0].(map[string]any)["message"].(map[string]any)
	if message["content"] != "pong" {
		t.Fatalf("unexpected converted content: %#v", response)
	}
}

func TestChatRequestWithReasoningContentUsesResponsesOnlyModel(t *testing.T) {
	var upstreamPayload map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&upstreamPayload); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":         "resp_1",
			"created_at": float64(123),
			"model":      "upstream-responses",
			"output": []any{map[string]any{
				"type": "message",
				"role": "assistant",
				"content": []any{map[string]any{
					"type": "output_text",
					"text": "ok",
				}},
			}},
		})
	}))
	defer upstream.Close()

	s := newProxyTestStore(t)
	model := addProxyTestModel(t, s, "p1", "upstream-responses", upstream.URL)
	model = setProxyTestModelSupport(t, s, model, false, true)
	addProxyTestRoute(t, s, "coder-fast", model.InternalID)

	h := &Handler{store: s, client: &http.Client{Timeout: time.Second}}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model":"coder-fast",
		"messages":[
			{"role":"user","content":"first"},
			{"role":"assistant","content":"answer","reasoning_content":"thinking"},
			{"role":"user","content":"continue"}
		]
	}`))
	w := httptest.NewRecorder()

	h.completion(w, req, "chat")

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	input, ok := upstreamPayload["input"].([]any)
	if !ok || len(input) != 3 {
		t.Fatalf("unexpected upstream input: %#v", upstreamPayload["input"])
	}
	assistant := input[1].(map[string]any)
	if assistant["role"] != "assistant" || assistant["content"] != "answer" {
		t.Fatalf("assistant message was not preserved: %#v", assistant)
	}
}

func TestRouteFallsBackWhenCandidateCannotBeConverted(t *testing.T) {
	firstCalled := make(chan struct{}, 1)
	firstUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstCalled <- struct{}{}
		writeJSON(w, http.StatusOK, map[string]any{"id": "first"})
	}))
	defer firstUpstream.Close()
	var secondPayload map[string]any
	secondUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&secondPayload); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":      "chatcmpl_2",
			"object":  "chat.completion",
			"created": float64(123),
			"model":   "upstream-chat",
			"choices": []any{map[string]any{
				"message": map[string]any{
					"role":    "assistant",
					"content": "fallback-ok",
				},
				"finish_reason": "stop",
			}},
		})
	}))
	defer secondUpstream.Close()

	s := newProxyTestStore(t)
	first := addProxyTestModel(t, s, "p1", "upstream-responses", firstUpstream.URL)
	first = setProxyTestModelSupport(t, s, first, false, true)
	second := addProxyTestModel(t, s, "p2", "upstream-chat", secondUpstream.URL)
	second = setProxyTestModelSupport(t, s, second, true, false)
	addProxyTestRoute(t, s, "coder-fast", first.InternalID, second.InternalID)

	h := &Handler{store: s, client: &http.Client{Timeout: time.Second}}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model":"coder-fast",
		"messages":[{"role":"user","content":"ping"}],
		"response_format":{"type":"json_object"}
	}`))
	w := httptest.NewRecorder()

	h.completion(w, req, "chat")

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	select {
	case <-firstCalled:
		t.Fatal("first upstream should be skipped before request because conversion is unsupported")
	default:
	}
	if secondPayload["response_format"] == nil {
		t.Fatalf("second chat model should receive original chat payload: %#v", secondPayload)
	}
	logs, err := s.ListLogs(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].FinalModel != second.InternalID || len(logs[0].Attempts) != 1 {
		t.Fatalf("fallback should finish on second model: %#v", logs)
	}
}

func TestMultiKeyProviderRoundRobinUsesHiddenKeysAndLogsMainModel(t *testing.T) {
	authHeaders := []string{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		writeJSON(w, http.StatusOK, map[string]any{"id": "ok", "choices": []any{}})
	}))
	defer upstream.Close()

	s := newProxyTestStore(t)
	if _, err := s.UpsertProvider(store.Provider{
		ID:              "openai",
		Name:            "OpenAI",
		BaseURL:         upstream.URL,
		APIKey:          "sk-one",
		Enabled:         true,
		MultiKeyEnabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddProviderKey("openai", store.ProviderKey{Name: "Key 2", APIKey: "sk-two"}); err != nil {
		t.Fatal(err)
	}
	model, err := s.UpsertModel(store.Model{
		ProviderID:        "openai",
		OriginalID:        "gpt-4o",
		DisplayName:       "gpt-4o",
		SupportsChat:      true,
		SupportsResponses: true,
		SupportsStream:    true,
		Enabled:           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	addProxyTestRoute(t, s, "coder-fast", model.InternalID)

	h := &Handler{store: s, client: &http.Client{Timeout: time.Second}}
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"coder-fast","messages":[]}`))
		w := httptest.NewRecorder()
		h.completion(w, req, "chat")
		if w.Code != http.StatusOK {
			t.Fatalf("request %d unexpected status: %d body=%s", i+1, w.Code, w.Body.String())
		}
	}
	if len(authHeaders) != 2 || authHeaders[0] != "Bearer sk-one" || authHeaders[1] != "Bearer sk-two" {
		t.Fatalf("round robin should use both keys in order: %#v", authHeaders)
	}
	logs, err := s.ListLogs(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 || logs[0].FinalModel != model.InternalID || logs[0].Attempts[0].ModelID != model.InternalID {
		t.Fatalf("logs should show the main model: %#v", logs)
	}
	seenKeys := map[string]bool{}
	for _, log := range logs {
		if len(log.Attempts) != 1 {
			t.Fatalf("each request should have one attempt: %#v", logs)
		}
		seenKeys[log.Attempts[0].KeyName] = true
	}
	if !seenKeys["Key 1"] || !seenKeys["Key 2"] {
		t.Fatalf("logs should keep key names: %#v", logs)
	}
}

func TestModelGroupExpandsMultiKeyModelBeforeNextGroupMember(t *testing.T) {
	normalCalled := make(chan struct{}, 1)
	multiKeyUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer sk-one" {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "first key failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": "ok", "choices": []any{}})
	}))
	defer multiKeyUpstream.Close()
	normalUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		normalCalled <- struct{}{}
		writeJSON(w, http.StatusOK, map[string]any{"id": "normal", "choices": []any{}})
	}))
	defer normalUpstream.Close()

	s := newProxyTestStore(t)
	if _, err := s.UpsertProvider(store.Provider{
		ID:               "openai",
		Name:             "OpenAI",
		BaseURL:          multiKeyUpstream.URL,
		APIKey:           "sk-one",
		Enabled:          true,
		MultiKeyEnabled:  true,
		MultiKeyStrategy: "fallback",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddProviderKey("openai", store.ProviderKey{Name: "Key 2", APIKey: "sk-two"}); err != nil {
		t.Fatal(err)
	}
	first, err := s.UpsertModel(store.Model{
		ProviderID:        "openai",
		OriginalID:        "gpt-4o",
		DisplayName:       "gpt-4o",
		SupportsChat:      true,
		SupportsResponses: true,
		SupportsStream:    true,
		Enabled:           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	second := addProxyTestModel(t, s, "p2", "backup", normalUpstream.URL)
	group, err := s.UpsertGroup(store.ModelGroup{
		Name:     "group",
		Strategy: "fallback",
		Enabled:  true,
		Members: []store.ModelGroupMember{
			{ModelID: first.InternalID, Position: 1, Enabled: true},
			{ModelID: second.InternalID, Position: 2, Enabled: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertRoute(store.Route{
		ID:      "coder-fast",
		Name:    "coder-fast",
		Enabled: true,
		Steps: []store.RouteStep{{
			Position:   1,
			TargetType: "group",
			TargetID:   group.ID,
			Enabled:    true,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	h := &Handler{store: s, client: &http.Client{Timeout: time.Second}}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"coder-fast","messages":[]}`))
	w := httptest.NewRecorder()
	h.completion(w, req, "chat")

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	select {
	case <-normalCalled:
		t.Fatal("backup model should not be used before the second key")
	default:
	}
	logs, err := s.ListLogs(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || len(logs[0].Attempts) != 2 {
		t.Fatalf("expected two key attempts, got %#v", logs)
	}
	if logs[0].Attempts[0].ModelID != first.InternalID || logs[0].Attempts[1].KeyName != "Key 2" {
		t.Fatalf("group should expand the multi-key model before backup: %#v", logs[0].Attempts)
	}
}

func TestResponsesRequestConvertsChatModelResponse(t *testing.T) {
	var upstreamPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		writeJSON(w, http.StatusOK, map[string]any{
			"id":      "chatcmpl_1",
			"object":  "chat.completion",
			"created": float64(123),
			"model":   "upstream-chat",
			"choices": []any{map[string]any{
				"index": float64(0),
				"message": map[string]any{
					"role":    "assistant",
					"content": "pong",
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{
				"prompt_tokens":     float64(2),
				"completion_tokens": float64(3),
				"total_tokens":      float64(5),
			},
		})
	}))
	defer upstream.Close()

	s := newProxyTestStore(t)
	model := addProxyTestModel(t, s, "p1", "upstream-chat", upstream.URL)
	model = setProxyTestModelSupport(t, s, model, true, false)
	addProxyTestRoute(t, s, "coder-fast", model.InternalID)

	h := &Handler{store: s, client: &http.Client{Timeout: time.Second}}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"coder-fast","input":"ping"}`))
	w := httptest.NewRecorder()

	h.completion(w, req, "responses")

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	if upstreamPath != "/chat/completions" {
		t.Fatalf("expected /chat/completions upstream, got %q", upstreamPath)
	}
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["object"] != "response" || response["output_text"] != "pong" {
		t.Fatalf("unexpected converted response: %#v", response)
	}
}

func TestConvertChatToolCallResponseToResponses(t *testing.T) {
	got, err := convertChatResponseToResponses([]byte(`{
		"id":"chatcmpl_1",
		"created":123,
		"model":"upstream-chat",
		"choices":[{
			"message":{
				"role":"assistant",
				"content":null,
				"tool_calls":[{
					"id":"call_1",
					"type":"function",
					"function":{"name":"lookup","arguments":"{\"id\":\"1\"}"}
				}]
			},
			"finish_reason":"tool_calls"
		}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := json.Unmarshal(got, &response); err != nil {
		t.Fatal(err)
	}
	output, _ := response["output"].([]any)
	if len(output) != 1 {
		t.Fatalf("unexpected output: %#v", response)
	}
	call := output[0].(map[string]any)
	if call["type"] != "function_call" || call["name"] != "lookup" || call["call_id"] != "call_1" {
		t.Fatalf("unexpected function call output: %#v", call)
	}
}

func TestConvertChatReasoningResponseToResponses(t *testing.T) {
	got, err := convertChatResponseToResponses([]byte(`{
		"id":"chatcmpl_1",
		"created":123,
		"model":"upstream-chat",
		"choices":[{
			"message":{
				"role":"assistant",
				"reasoning_content":"thinking",
				"content":"answer"
			},
			"finish_reason":"stop"
		}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := json.Unmarshal(got, &response); err != nil {
		t.Fatal(err)
	}
	output := response["output"].([]any)
	if output[0].(map[string]any)["type"] != "reasoning" {
		t.Fatalf("reasoning output was not preserved: %#v", output)
	}
	if response["output_text"] != "answer" {
		t.Fatalf("answer text was not preserved: %#v", response)
	}
}

func TestConvertResponsesToolCallResponseToChat(t *testing.T) {
	got, err := convertResponsesResponseToChat([]byte(`{
		"id":"resp_1",
		"created_at":123,
		"model":"upstream-responses",
		"output":[{
			"type":"function_call",
			"id":"fc_1",
			"call_id":"call_1",
			"name":"lookup",
			"arguments":"{\"id\":\"1\"}",
			"status":"completed"
		}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := json.Unmarshal(got, &response); err != nil {
		t.Fatal(err)
	}
	choice := response["choices"].([]any)[0].(map[string]any)
	if choice["finish_reason"] != "tool_calls" {
		t.Fatalf("unexpected finish_reason: %#v", choice)
	}
	message := choice["message"].(map[string]any)
	toolCalls := message["tool_calls"].([]any)
	call := toolCalls[0].(map[string]any)
	if call["id"] != "call_1" || call["function"].(map[string]any)["name"] != "lookup" {
		t.Fatalf("unexpected tool_calls: %#v", message)
	}
}

func TestConvertResponsesReasoningResponseToChat(t *testing.T) {
	got, err := convertResponsesResponseToChat([]byte(`{
		"id":"resp_1",
		"created_at":123,
		"model":"upstream-responses",
		"output":[
			{"type":"reasoning","summary":[{"type":"summary_text","text":"thinking"}]},
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := json.Unmarshal(got, &response); err != nil {
		t.Fatal(err)
	}
	message := response["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	if message["reasoning_content"] != "thinking" || message["content"] != "answer" {
		t.Fatalf("reasoning/content was not converted: %#v", message)
	}
}

func TestStreamResponsesToChatConvertsTextDelta(t *testing.T) {
	body := strings.NewReader(`event: response.output_text.delta
data: {"type":"response.output_text.delta","response_id":"resp_1","delta":"pong"}

event: response.completed
data: {"type":"response.completed","response":{"id":"resp_1","created_at":123,"model":"upstream-responses"}}

`)
	w := httptest.NewRecorder()

	if err := streamResponsesToChat(w, body, "upstream-responses", false); err != nil {
		t.Fatal(err)
	}
	out := w.Body.String()
	if !strings.Contains(out, `"object":"chat.completion.chunk"`) || !strings.Contains(out, `"content":"pong"`) {
		t.Fatalf("unexpected chat stream: %s", out)
	}
	if !strings.Contains(out, "data: [DONE]") {
		t.Fatalf("chat stream did not finish: %s", out)
	}
}

func TestStreamResponsesToChatIncludesUsageWhenRequested(t *testing.T) {
	body := strings.NewReader(`event: response.output_text.delta
data: {"type":"response.output_text.delta","response_id":"resp_1","delta":"pong"}

event: response.completed
data: {"type":"response.completed","response":{"id":"resp_1","created_at":123,"model":"upstream-responses","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}

`)
	w := httptest.NewRecorder()

	if err := streamResponsesToChat(w, body, "upstream-responses", true); err != nil {
		t.Fatal(err)
	}
	out := w.Body.String()
	if !strings.Contains(out, `"choices":[]`) || !strings.Contains(out, `"prompt_tokens":2`) || !strings.Contains(out, `"completion_tokens":3`) {
		t.Fatalf("usage chunk was not converted: %s", out)
	}
	if !strings.Contains(out, "data: [DONE]") {
		t.Fatalf("chat stream did not finish: %s", out)
	}
}

func TestStreamChatToResponsesConvertsTextDelta(t *testing.T) {
	body := strings.NewReader(`data: {"id":"chatcmpl_1","created":123,"model":"upstream-chat","choices":[{"delta":{"content":"pong"},"finish_reason":null}]}

data: [DONE]

`)
	w := httptest.NewRecorder()

	if err := streamChatToResponses(w, body, "upstream-chat"); err != nil {
		t.Fatal(err)
	}
	out := w.Body.String()
	if !strings.Contains(out, "response.output_text.delta") || !strings.Contains(out, `"delta":"pong"`) {
		t.Fatalf("unexpected responses stream: %s", out)
	}
	if !strings.Contains(out, "response.completed") {
		t.Fatalf("responses stream did not finish: %s", out)
	}
}

func TestStreamResponsesToChatConvertsReasoningDelta(t *testing.T) {
	body := strings.NewReader(`event: response.reasoning_summary_text.delta
data: {"type":"response.reasoning_summary_text.delta","response_id":"resp_1","delta":"thinking"}

event: response.output_text.delta
data: {"type":"response.output_text.delta","response_id":"resp_1","delta":"answer"}

event: response.completed
data: {"type":"response.completed","response":{"id":"resp_1","created_at":123,"model":"upstream-responses"}}

`)
	w := httptest.NewRecorder()

	if err := streamResponsesToChat(w, body, "upstream-responses", false); err != nil {
		t.Fatal(err)
	}
	out := w.Body.String()
	if !strings.Contains(out, `"reasoning_content":"thinking"`) || !strings.Contains(out, `"content":"answer"`) {
		t.Fatalf("reasoning stream was not converted: %s", out)
	}
}

func TestStreamChatToResponsesConvertsReasoningDelta(t *testing.T) {
	body := strings.NewReader(`data: {"id":"chatcmpl_1","created":123,"model":"upstream-chat","choices":[{"delta":{"reasoning_content":"thinking"},"finish_reason":null}]}

data: {"id":"chatcmpl_1","created":123,"model":"upstream-chat","choices":[{"delta":{"content":"answer"},"finish_reason":null}]}

data: [DONE]

`)
	w := httptest.NewRecorder()

	if err := streamChatToResponses(w, body, "upstream-chat"); err != nil {
		t.Fatal(err)
	}
	out := w.Body.String()
	if !strings.Contains(out, "response.reasoning_summary_text.delta") || !strings.Contains(out, `"delta":"thinking"`) {
		t.Fatalf("reasoning stream was not converted: %s", out)
	}
	if !strings.Contains(out, "response.output_text.delta") || !strings.Contains(out, `"delta":"answer"`) {
		t.Fatalf("answer stream was not converted: %s", out)
	}
}

func TestStreamResponsesToChatConvertsToolCallDelta(t *testing.T) {
	body := strings.NewReader(`event: response.output_item.added
data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":"","status":"in_progress"}}

event: response.function_call_arguments.delta
data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":0,"delta":"{\"id\""}

event: response.function_call_arguments.delta
data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":0,"delta":":\"1\"}"}

event: response.completed
data: {"type":"response.completed","response":{"id":"resp_1","created_at":123,"model":"upstream-responses"}}

`)
	w := httptest.NewRecorder()

	if err := streamResponsesToChat(w, body, "upstream-responses", false); err != nil {
		t.Fatal(err)
	}
	out := w.Body.String()
	if !strings.Contains(out, `"tool_calls"`) || !strings.Contains(out, `"name":"lookup"`) {
		t.Fatalf("unexpected chat tool stream: %s", out)
	}
	if !strings.Contains(out, `"finish_reason":"tool_calls"`) {
		t.Fatalf("chat tool stream did not finish as tool_calls: %s", out)
	}
}

func TestStreamChatToResponsesConvertsToolCallDelta(t *testing.T) {
	body := strings.NewReader(`data: {"id":"chatcmpl_1","created":123,"model":"upstream-chat","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":""}}]},"finish_reason":null}]}

data: {"id":"chatcmpl_1","created":123,"model":"upstream-chat","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"id\":\"1\"}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl_1","created":123,"model":"upstream-chat","choices":[{"delta":{},"finish_reason":"tool_calls"}]}

`)
	w := httptest.NewRecorder()

	if err := streamChatToResponses(w, body, "upstream-chat"); err != nil {
		t.Fatal(err)
	}
	out := w.Body.String()
	if !strings.Contains(out, "response.output_item.added") || !strings.Contains(out, `"name":"lookup"`) {
		t.Fatalf("unexpected responses tool stream: %s", out)
	}
	if !strings.Contains(out, "response.function_call_arguments.delta") || !strings.Contains(out, `"delta"`) || !strings.Contains(out, `id`) {
		t.Fatalf("tool arguments were not streamed: %s", out)
	}
	if !strings.Contains(out, "response.function_call_arguments.done") {
		t.Fatalf("tool arguments did not finish: %s", out)
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

func TestUpstreamBadRequestFallsBackToNextModel(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad request"})
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"id": "good", "choices": []any{}})
	}))
	defer good.Close()

	s := newProxyTestStore(t)
	first := addProxyTestModel(t, s, "p1", "first", bad.URL)
	second := addProxyTestModel(t, s, "p2", "second", good.URL)
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
		t.Fatalf("expected fallback after 400, got logs=%d attempts=%d", len(logs), len(logs[0].Attempts))
	}
	if logs[0].Attempts[0].HTTPStatus != http.StatusBadRequest || logs[0].Attempts[0].Status != "failed" {
		t.Fatalf("unexpected first attempt: %#v", logs[0].Attempts[0])
	}
	firstAfter, err := s.GetModel(first.InternalID)
	if err != nil {
		t.Fatal(err)
	}
	if firstAfter.UpstreamErrorStatus != 0 {
		t.Fatalf("400 should not create a model issue marker: %#v", firstAfter)
	}
}

func TestUpstreamUnauthorizedMarksModelIssueAndFallsBack(t *testing.T) {
	unauthorized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid key"})
	}))
	defer unauthorized.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"id": "good", "choices": []any{}})
	}))
	defer good.Close()

	s := newProxyTestStore(t)
	first := addProxyTestModel(t, s, "p1", "first", unauthorized.URL)
	second := addProxyTestModel(t, s, "p2", "second", good.URL)
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
	if firstAfter.UpstreamErrorStatus != http.StatusUnauthorized || firstAfter.UpstreamErrorAt == "" || !strings.Contains(firstAfter.UpstreamError, "上游返回 401") {
		t.Fatalf("401 should create a model issue marker: %#v", firstAfter)
	}
	if err := s.RestoreModel(first.InternalID); err != nil {
		t.Fatal(err)
	}
	restored, err := s.GetModel(first.InternalID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.UpstreamErrorStatus != 0 || restored.UpstreamError != "" || restored.UpstreamErrorAt != "" {
		t.Fatalf("restore should clear model issue marker: %#v", restored)
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

func setProxyTestModelSupport(t *testing.T, s *store.Store, model store.Model, supportsChat bool, supportsResponses bool) store.Model {
	t.Helper()
	model.SupportsChat = supportsChat
	model.SupportsResponses = supportsResponses
	updated, err := s.UpsertModel(model)
	if err != nil {
		t.Fatal(err)
	}
	return updated
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
