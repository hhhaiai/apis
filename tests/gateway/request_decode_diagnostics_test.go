package gateway_test

import (
	. "ccgateway/internal/gateway"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ccgateway/internal/ccevent"
	"ccgateway/internal/modelmap"
	"ccgateway/internal/orchestrator"
	"ccgateway/internal/policy"
	"ccgateway/internal/todo"
)

func TestUnsupportedFieldCreatesDiagnosticEventWithCurl(t *testing.T) {
	eventStore := ccevent.NewStore()
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		TodoStore:    todo.NewStore(),
		EventStore:   eventStore,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/cc/todos", strings.NewReader(`{"title":"ship","session_id":"sess_1","unknown_field":1}`))
	req.Header.Set("authorization", "Bearer secret-token")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body=%s", rr.Code, rr.Body.String())
	}

	events := eventStore.List(ccevent.ListFilter{EventType: "request.unsupported_fields", Limit: 10})
	if len(events) == 0 {
		t.Fatalf("expected request.unsupported_fields event")
	}
	ev := events[0]
	if got := strings.TrimSpace(fmt.Sprint(ev.Data["reason"])); got != "unsupported_fields" {
		t.Fatalf("expected reason unsupported_fields, got %q", got)
	}
	fields, ok := ev.Data["unsupported_fields"].([]string)
	if !ok {
		anyFields, okAny := ev.Data["unsupported_fields"].([]any)
		if !okAny {
			t.Fatalf("expected unsupported_fields list, got %#v", ev.Data["unsupported_fields"])
		}
		fields = make([]string, 0, len(anyFields))
		for _, item := range anyFields {
			fields = append(fields, strings.TrimSpace(item.(string)))
		}
	}
	if len(fields) == 0 || fields[0] != "unknown_field" {
		t.Fatalf("expected unknown_field in unsupported_fields, got %#v", fields)
	}

	bodyText := strings.TrimSpace(fmt.Sprint(ev.Data["request_body"]))
	if !strings.Contains(bodyText, `"unknown_field"`) {
		t.Fatalf("expected request_body to include unknown field, got %q", bodyText)
	}

	curlCommand := strings.TrimSpace(fmt.Sprint(ev.Data["curl_command"]))
	if curlCommand == "" {
		t.Fatalf("expected curl_command in diagnostics event")
	}
	if !strings.Contains(curlCommand, "[REDACTED]") {
		t.Fatalf("expected redacted secrets in curl command, got %q", curlCommand)
	}
	if strings.Contains(curlCommand, "secret-token") {
		t.Fatalf("expected auth token not leaked in curl command, got %q", curlCommand)
	}
}

func TestTrailingJSONCreatesDecodeFailedDiagnosticEvent(t *testing.T) {
	eventStore := ccevent.NewStore()
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		EventStore:   eventStore,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"claude-test",
		"max_tokens":64,
		"messages":[{"role":"user","content":"hello"}]
	} {}`))
	req.Header.Set("anthropic-version", "2023-06-01")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body=%s", rr.Code, rr.Body.String())
	}

	events := eventStore.List(ccevent.ListFilter{EventType: "request.decode_failed", Limit: 10})
	if len(events) == 0 {
		t.Fatalf("expected request.decode_failed event")
	}
	ev := events[0]
	reason := strings.TrimSpace(fmt.Sprint(ev.Data["reason"]))
	if reason != "trailing_json" {
		t.Fatalf("expected trailing_json reason, got %q", reason)
	}
}
