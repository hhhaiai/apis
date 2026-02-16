package toolruntime_test

import (
	. "ccgateway/internal/toolruntime"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRabbitPublishTool(t *testing.T) {
	var seenAuth int32
	rabbitAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/exchanges///amq.default/publish" && r.URL.Path != "/api/exchanges/%2F/amq.default/publish" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "guest" || pass != "guest" {
			t.Fatalf("unexpected basic auth: ok=%v user=%q", ok, user)
		}
		atomic.StoreInt32(&seenAuth, 1)
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		if !strings.Contains(string(body), `"routing_key":"demo.queue"`) {
			t.Fatalf("unexpected publish body: %s", string(body))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"routed": true})
	}))
	defer rabbitAPI.Close()

	ex := NewDefaultExecutor()
	out, err := ex.Execute(context.Background(), Call{
		Name: "rabbit_publish",
		Input: map[string]any{
			"management_url": rabbitAPI.URL,
			"routing_key":    "demo.queue",
			"payload": map[string]any{
				"hello": "world",
			},
		},
	})
	if err != nil {
		t.Fatalf("execute rabbit_publish: %v", err)
	}
	if atomic.LoadInt32(&seenAuth) != 1 {
		t.Fatalf("expected rabbit_publish to authenticate via basic auth")
	}
	content, ok := out.Content.(map[string]any)
	if !ok {
		t.Fatalf("expected map content, got %T", out.Content)
	}
	if routed, _ := content["routed"].(bool); !routed {
		t.Fatalf("expected routed=true, got %#v", content["routed"])
	}
}

func TestRabbitGetTool(t *testing.T) {
	rabbitAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/queues///demo.queue/get" && r.URL.Path != "/api/queues/%2F/demo.queue/get" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"payload": "hello",
			},
		})
	}))
	defer rabbitAPI.Close()

	ex := NewDefaultExecutor()
	out, err := ex.Execute(context.Background(), Call{
		Name: "rabbit_get",
		Input: map[string]any{
			"management_url": rabbitAPI.URL,
			"queue":          "demo.queue",
			"count":          1,
		},
	})
	if err != nil {
		t.Fatalf("execute rabbit_get: %v", err)
	}
	content, ok := out.Content.(map[string]any)
	if !ok {
		t.Fatalf("expected map content, got %T", out.Content)
	}
	switch count := content["count"].(type) {
	case int:
		if count != 1 {
			t.Fatalf("expected count=1, got %#v", content["count"])
		}
	case float64:
		if int(count) != 1 {
			t.Fatalf("expected count=1, got %#v", content["count"])
		}
	default:
		t.Fatalf("unexpected count type: %T", content["count"])
	}
}

func TestRabbitRPCTool(t *testing.T) {
	var getCalls int32
	rabbitAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && (strings.Contains(r.URL.Path, "/api/queues/%2F/ccgateway.reply.") || strings.Contains(r.URL.Path, "/api/queues///ccgateway.reply.")):
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodDelete && (strings.Contains(r.URL.Path, "/api/queues/%2F/ccgateway.reply.") || strings.Contains(r.URL.Path, "/api/queues///ccgateway.reply.")):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && (r.URL.Path == "/api/exchanges/%2F/amq.default/publish" || r.URL.Path == "/api/exchanges///amq.default/publish"):
			_ = json.NewEncoder(w).Encode(map[string]any{"routed": true})
		case r.Method == http.MethodPost && (strings.Contains(r.URL.Path, "/api/queues/%2F/ccgateway.reply.") || strings.Contains(r.URL.Path, "/api/queues///ccgateway.reply.")):
			n := atomic.AddInt32(&getCalls, 1)
			if n == 1 {
				_ = json.NewEncoder(w).Encode([]map[string]any{})
				return
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"payload": "rpc-ok",
				},
			})
		default:
			t.Fatalf("unexpected rabbit rpc request: method=%s path=%s", r.Method, r.URL.Path)
		}
	}))
	defer rabbitAPI.Close()

	ex := NewDefaultExecutor()
	out, err := ex.Execute(context.Background(), Call{
		Name: "rabbit_rpc",
		Input: map[string]any{
			"management_url":   rabbitAPI.URL,
			"request_queue":    "rpc.queue",
			"payload":          map[string]any{"q": "hello"},
			"timeout_ms":       1500,
			"poll_interval_ms": 20,
		},
	})
	if err != nil {
		t.Fatalf("execute rabbit_rpc: %v", err)
	}
	content, ok := out.Content.(map[string]any)
	if !ok {
		t.Fatalf("expected map content, got %T", out.Content)
	}
	if _, ok := content["response"].(map[string]any); !ok {
		t.Fatalf("expected response map, got %#v", content["response"])
	}
}
