package proxy

import "testing"

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
