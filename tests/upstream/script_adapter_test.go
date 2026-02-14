package upstream_test

import (
	. "ccgateway/internal/upstream"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ccgateway/internal/orchestrator"
)

func TestScriptAdapterComplete(t *testing.T) {
	script := writeScript(t, `#!/bin/sh
set -eu
payload="$(cat)"
if ! printf "%s" "$payload" | grep -q '"description":"No description provided"'; then
  printf '%s\n' '{"text":"missing tool description"}'
  exit 0
fi
printf '%s\n' '{"model":"script-model","text":"ok","usage":{"prompt_tokens":3,"completion_tokens":2}}'
`)

	adapter, err := NewScriptAdapter(ScriptAdapterConfig{
		Name:    "script-a1",
		Command: script,
		Model:   "script-model",
	})
	if err != nil {
		t.Fatalf("new script adapter failed: %v", err)
	}

	resp, err := adapter.Complete(context.Background(), orchestrator.Request{
		Model:     "fallback-model",
		MaxTokens: 128,
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hello"},
		},
		Tools: []orchestrator.Tool{
			{Name: "read_file", InputSchema: map[string]any{"type": "object"}},
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if resp.Model != "script-model" {
		t.Fatalf("unexpected model: %q", resp.Model)
	}
	if len(resp.Blocks) != 1 || resp.Blocks[0].Text != "ok" {
		t.Fatalf("unexpected blocks: %+v", resp.Blocks)
	}
	if resp.Usage.InputTokens != 3 || resp.Usage.OutputTokens != 2 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
}

func TestScriptAdapterStreamNDJSON(t *testing.T) {
	script := writeScript(t, `#!/bin/sh
set -eu
cat >/dev/null
printf '%s\n' '{"type":"event","event":{"type":"message_start"}}'
printf '%s\n' '{"type":"event","event":{"type":"content_block_start","index":0,"block":{"type":"text"}}}'
printf '%s\n' '{"type":"event","event":{"type":"content_block_delta","index":0,"delta_text":"hel"}}'
printf '%s\n' '{"type":"event","event":{"type":"content_block_delta","index":0,"delta_text":"lo"}}'
printf '%s\n' '{"type":"event","event":{"type":"content_block_stop","index":0}}'
printf '%s\n' '{"type":"event","event":{"type":"message_delta","stop_reason":"end_turn","usage":{"input_tokens":7,"output_tokens":2}}}'
printf '%s\n' '{"type":"event","event":{"type":"message_stop"}}'
`)

	adapter, err := NewScriptAdapter(ScriptAdapterConfig{
		Name:    "script-stream",
		Command: script,
	})
	if err != nil {
		t.Fatalf("new script adapter failed: %v", err)
	}

	events, errs := adapter.Stream(context.Background(), orchestrator.Request{
		Model:     "stream-model",
		MaxTokens: 64,
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hello"},
		},
	})

	var got []orchestrator.StreamEvent
	for ev := range events {
		got = append(got, ev)
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("stream failed: %v", err)
		}
	}

	if len(got) < 7 {
		t.Fatalf("expected at least 7 events, got %d", len(got))
	}
	if got[0].Type != "message_start" {
		t.Fatalf("unexpected first event: %+v", got[0])
	}
	if got[2].Type != "content_block_delta" || got[2].DeltaText != "hel" {
		t.Fatalf("unexpected first delta: %+v", got[2])
	}
	if got[3].Type != "content_block_delta" || got[3].DeltaText != "lo" {
		t.Fatalf("unexpected second delta: %+v", got[3])
	}
	if got[len(got)-1].Type != "message_stop" {
		t.Fatalf("unexpected last event: %+v", got[len(got)-1])
	}
}

func TestScriptAdapterStreamFallbackResponse(t *testing.T) {
	script := writeScript(t, `#!/bin/sh
set -eu
cat >/dev/null
printf '%s\n' '{"response":{"model":"script-fallback","text":"fallback text","stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":1}}}'
`)

	adapter, err := NewScriptAdapter(ScriptAdapterConfig{
		Name:    "script-fallback",
		Command: script,
	})
	if err != nil {
		t.Fatalf("new script adapter failed: %v", err)
	}

	events, errs := adapter.Stream(context.Background(), orchestrator.Request{
		Model:     "request-model",
		MaxTokens: 32,
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hello"},
		},
	})

	var text strings.Builder
	var stopReason string
	for ev := range events {
		if ev.Type == "content_block_delta" {
			text.WriteString(ev.DeltaText)
		}
		if ev.Type == "message_delta" {
			stopReason = ev.StopReason
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("stream failed: %v", err)
		}
	}

	if text.String() != "fallback text" {
		t.Fatalf("unexpected fallback text: %q", text.String())
	}
	if stopReason != "end_turn" {
		t.Fatalf("unexpected stop reason: %q", stopReason)
	}
}

func writeScript(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "adapter.sh")
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatalf("write script failed: %v", err)
	}
	return path
}
