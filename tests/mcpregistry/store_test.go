package mcpregistry_test

import (
	. "ccgateway/internal/mcpregistry"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestStoreRegisterGetList(t *testing.T) {
	store := NewStore(nil)
	enabled := true
	a, err := store.Register(RegisterInput{
		ID:        "mcp_a",
		Name:      "A",
		Transport: TransportStdio,
		Command:   "sh",
		Enabled:   &enabled,
	})
	if err != nil {
		t.Fatalf("register a: %v", err)
	}
	b, err := store.Register(RegisterInput{
		ID:        "mcp_b",
		Name:      "B",
		Transport: TransportStdio,
		Command:   "sh",
	})
	if err != nil {
		t.Fatalf("register b: %v", err)
	}

	got, ok := store.Get(a.ID)
	if !ok {
		t.Fatalf("expected get success")
	}
	if got.Name != "A" {
		t.Fatalf("unexpected get name: %q", got.Name)
	}

	list := store.List(10)
	if len(list) != 2 {
		t.Fatalf("expected 2 items, got %d", len(list))
	}
	if list[0].ID != b.ID || list[1].ID != a.ID {
		t.Fatalf("expected reverse order, got %#v", []string{list[0].ID, list[1].ID})
	}
}

func TestStoreUpdateDelete(t *testing.T) {
	store := NewStore(nil)
	server, err := store.Register(RegisterInput{
		ID:        "mcp_x",
		Name:      "before",
		Transport: TransportStdio,
		Command:   "sh",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	name := "after"
	timeout := 12000
	updated, err := store.Update(server.ID, UpdateInput{
		Name:      &name,
		TimeoutMS: &timeout,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "after" || updated.TimeoutMS != 12000 {
		t.Fatalf("unexpected update result: %+v", updated)
	}

	if err := store.Delete(server.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := store.Get(server.ID); ok {
		t.Fatalf("expected deleted server not found")
	}
}

func TestStoreCheckHealthHTTP(t *testing.T) {
	healthyHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthyHTTP.Close()

	store := NewStore(healthyHTTP.Client())
	server, err := store.Register(RegisterInput{
		ID:        "mcp_http",
		Name:      "http-server",
		Transport: TransportHTTP,
		URL:       healthyHTTP.URL,
	})
	if err != nil {
		t.Fatalf("register http: %v", err)
	}

	checked, err := store.CheckHealth(context.Background(), server.ID)
	if err != nil {
		t.Fatalf("check health: %v", err)
	}
	if !checked.Status.Healthy {
		t.Fatalf("expected healthy status, got %+v", checked.Status)
	}
	if checked.Status.LastCheckedAt.IsZero() {
		t.Fatalf("expected last checked at set")
	}
}

func TestStoreCheckHealthStdioNotFound(t *testing.T) {
	store := NewStore(nil)
	server, err := store.Register(RegisterInput{
		ID:        "mcp_stdio",
		Name:      "stdio-server",
		Transport: TransportStdio,
		Command:   "command-that-does-not-exist-anywhere",
	})
	if err != nil {
		t.Fatalf("register stdio: %v", err)
	}

	checked, err := store.CheckHealth(context.Background(), server.ID)
	if err != nil {
		t.Fatalf("check health stdio: %v", err)
	}
	if checked.Status.Healthy {
		t.Fatalf("expected unhealthy for missing command")
	}
	if checked.Status.LastError == "" {
		t.Fatalf("expected health error message")
	}
}

func TestStoreCheckHealthStdioHandshakeAndReconnect(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cat command may not be available on windows CI")
	}

	store := NewStore(nil)
	server, err := store.Register(RegisterInput{
		ID:        "mcp_stdio_ok",
		Name:      "stdio-ok",
		Transport: TransportStdio,
		Command:   "cat",
		TimeoutMS: 1200,
		Retries:   1,
	})
	if err != nil {
		t.Fatalf("register stdio: %v", err)
	}

	checked, err := store.CheckHealth(context.Background(), server.ID)
	if err != nil {
		t.Fatalf("check health stdio: %v", err)
	}
	if !checked.Status.Healthy {
		t.Fatalf("expected healthy stdio, got %+v", checked.Status)
	}

	reconnected, err := store.Reconnect(context.Background(), server.ID)
	if err != nil {
		t.Fatalf("reconnect stdio: %v", err)
	}
	if !reconnected.Status.Healthy {
		t.Fatalf("expected reconnect healthy stdio, got %+v", reconnected.Status)
	}
}

func TestStoreToolsListAndCallHTTP(t *testing.T) {
	rpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		id := req["id"]
		method, _ := req["method"].(string)
		var resp map[string]any
		switch method {
		case "tools/list":
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"tools": []map[string]any{
						{
							"name":        "remote_calc",
							"description": "calc by mcp",
							"inputSchema": map[string]any{"type": "object"},
						},
					},
				},
			}
		case "tools/call":
			params, _ := req["params"].(map[string]any)
			arguments, _ := params["arguments"].(map[string]any)
			a, _ := arguments["a"].(float64)
			b, _ := arguments["b"].(float64)
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"isError": false,
					"content": map[string]any{
						"sum": a + b,
					},
				},
			}
		default:
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"error": map[string]any{
					"message": "unsupported method",
				},
			}
		}
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer rpcServer.Close()

	store := NewStore(rpcServer.Client())
	registered, err := store.Register(RegisterInput{
		ID:        "mcp_http_rpc",
		Name:      "rpc-http",
		Transport: TransportHTTP,
		URL:       rpcServer.URL,
		TimeoutMS: 3000,
		Retries:   1,
	})
	if err != nil {
		t.Fatalf("register rpc http: %v", err)
	}

	tools, err := store.ListTools(context.Background(), registered.ID)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "remote_calc" {
		t.Fatalf("unexpected tools list: %+v", tools)
	}

	called, err := store.CallTool(context.Background(), registered.ID, "remote_calc", map[string]any{
		"a": 2,
		"b": 3,
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if called.ServerID != registered.ID {
		t.Fatalf("unexpected server id: %q", called.ServerID)
	}
	content, ok := called.Content.(map[string]any)
	if !ok {
		t.Fatalf("unexpected call content type: %T", called.Content)
	}
	if content["sum"] != float64(5) {
		t.Fatalf("unexpected sum content: %#v", content["sum"])
	}
}

func TestStoreCallToolAny(t *testing.T) {
	rpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		id := req["id"]
		method, _ := req["method"].(string)
		switch method {
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"tools": []map[string]any{
						{"name": "remote_search"},
					},
				},
			})
		case "tools/call":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"isError": false,
					"content": "ok-from-any",
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"error":   map[string]any{"message": "unsupported"},
			})
		}
	}))
	defer rpcServer.Close()

	store := NewStore(rpcServer.Client())
	_, err := store.Register(RegisterInput{
		ID:        "mcp_http_any",
		Name:      "rpc-http-any",
		Transport: TransportHTTP,
		URL:       rpcServer.URL,
	})
	if err != nil {
		t.Fatalf("register rpc http any: %v", err)
	}

	got, err := store.CallToolAny(context.Background(), "remote_search", map[string]any{"q": "hi"})
	if err != nil {
		t.Fatalf("call tool any: %v", err)
	}
	if got.ServerID != "mcp_http_any" {
		t.Fatalf("unexpected server id: %q", got.ServerID)
	}
	if strings.TrimSpace(got.Content.(string)) != "ok-from-any" {
		t.Fatalf("unexpected content: %#v", got.Content)
	}
}

func TestStoreListToolsCacheAndInvalidateOnUpdate(t *testing.T) {
	var listCalls int64
	rpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		id := req["id"]
		method, _ := req["method"].(string)
		switch method {
		case "tools/list":
			atomic.AddInt64(&listCalls, 1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"tools": []map[string]any{{"name": "cached_tool"}},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"error":   map[string]any{"message": "unsupported"},
			})
		}
	}))
	defer rpcServer.Close()

	store := NewStore(rpcServer.Client())
	store.SetToolsCacheTTL(5 * time.Minute)
	registered, err := store.Register(RegisterInput{
		ID:        "mcp_cache_1",
		Name:      "cache-test",
		Transport: TransportHTTP,
		URL:       rpcServer.URL,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if _, err := store.ListTools(context.Background(), registered.ID); err != nil {
		t.Fatalf("first list tools: %v", err)
	}
	if _, err := store.ListTools(context.Background(), registered.ID); err != nil {
		t.Fatalf("second list tools: %v", err)
	}
	if got := atomic.LoadInt64(&listCalls); got != 1 {
		t.Fatalf("expected cached list call count=1, got %d", got)
	}

	timeout := 9000
	if _, err := store.Update(registered.ID, UpdateInput{TimeoutMS: &timeout}); err != nil {
		t.Fatalf("update to trigger invalidation: %v", err)
	}
	if _, err := store.ListTools(context.Background(), registered.ID); err != nil {
		t.Fatalf("third list tools after update: %v", err)
	}
	if got := atomic.LoadInt64(&listCalls); got != 2 {
		t.Fatalf("expected list call count=2 after invalidation, got %d", got)
	}
}

func TestStoreCallToolAnyFallbackOnToolNotFound(t *testing.T) {
	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		id := req["id"]
		method, _ := req["method"].(string)
		switch method {
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"tools": []map[string]any{{"name": "remote_search"}},
				},
			})
		case "tools/call":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"error":   map[string]any{"message": "tool not found"},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"error":   map[string]any{"message": "unsupported"},
			})
		}
	}))
	defer badServer.Close()

	goodServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		id := req["id"]
		method, _ := req["method"].(string)
		switch method {
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"tools": []map[string]any{{"name": "remote_search"}},
				},
			})
		case "tools/call":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"isError": false,
					"content": "ok-from-second-server",
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"error":   map[string]any{"message": "unsupported"},
			})
		}
	}))
	defer goodServer.Close()

	store := NewStore(goodServer.Client())
	_, err := store.Register(RegisterInput{
		ID:        "mcp_good",
		Name:      "good",
		Transport: TransportHTTP,
		URL:       goodServer.URL,
	})
	if err != nil {
		t.Fatalf("register good: %v", err)
	}
	_, err = store.Register(RegisterInput{
		ID:        "mcp_bad",
		Name:      "bad",
		Transport: TransportHTTP,
		URL:       badServer.URL,
	})
	if err != nil {
		t.Fatalf("register bad: %v", err)
	}

	got, err := store.CallToolAny(context.Background(), "remote_search", map[string]any{"q": "hello"})
	if err != nil {
		t.Fatalf("call tool any with fallback: %v", err)
	}
	if got.ServerID != "mcp_good" {
		t.Fatalf("expected fallback to mcp_good, got %s", got.ServerID)
	}
	if strings.TrimSpace(got.Content.(string)) != "ok-from-second-server" {
		t.Fatalf("unexpected fallback content: %#v", got.Content)
	}
}
