package mcpregistry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrNotFound      = errors.New("mcp server not found")
	ErrAlreadyExists = errors.New("mcp server already exists")
	ErrToolNotFound  = errors.New("mcp tool not found")
)

const defaultToolsCacheTTL = 15 * time.Second

type Transport string

const (
	TransportHTTP  Transport = "http"
	TransportStdio Transport = "stdio"
)

type HealthStatus struct {
	Healthy       bool      `json:"healthy"`
	LastError     string    `json:"last_error,omitempty"`
	LastCheckedAt time.Time `json:"last_checked_at,omitempty"`
	LastLatencyMS int64     `json:"last_latency_ms"`
}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

type ToolCallResult struct {
	ServerID string `json:"server_id"`
	ToolName string `json:"tool_name"`
	Content  any    `json:"content"`
	IsError  bool   `json:"is_error"`
}

type Server struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	Name      string            `json:"name"`
	Transport Transport         `json:"transport"`
	URL       string            `json:"url,omitempty"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	TimeoutMS int               `json:"timeout_ms"`
	Retries   int               `json:"retries"`
	Enabled   bool              `json:"enabled"`
	Metadata  map[string]any    `json:"metadata,omitempty"`
	Status    HealthStatus      `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type RegisterInput struct {
	ID        string            `json:"id,omitempty"`
	Name      string            `json:"name"`
	Transport Transport         `json:"transport"`
	URL       string            `json:"url,omitempty"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	TimeoutMS int               `json:"timeout_ms,omitempty"`
	Retries   int               `json:"retries,omitempty"`
	Enabled   *bool             `json:"enabled,omitempty"`
	Metadata  map[string]any    `json:"metadata,omitempty"`
}

type UpdateInput struct {
	Name      *string            `json:"name,omitempty"`
	Transport *Transport         `json:"transport,omitempty"`
	URL       *string            `json:"url,omitempty"`
	Command   *string            `json:"command,omitempty"`
	Args      *[]string          `json:"args,omitempty"`
	Env       *map[string]string `json:"env,omitempty"`
	Headers   *map[string]string `json:"headers,omitempty"`
	TimeoutMS *int               `json:"timeout_ms,omitempty"`
	Retries   *int               `json:"retries,omitempty"`
	Enabled   *bool              `json:"enabled,omitempty"`
	Metadata  *map[string]any    `json:"metadata,omitempty"`
}

type Store struct {
	mu            sync.RWMutex
	servers       map[string]Server
	order         []string
	counter       uint64
	client        *http.Client
	stdio         *stdioConnector
	toolsCache    map[string]toolsCacheEntry
	toolsCacheTTL time.Duration
}

type toolsCacheEntry struct {
	tools     []Tool
	expiresAt time.Time
}

func NewStore(client *http.Client) *Store {
	if client == nil {
		client = http.DefaultClient
	}
	return &Store{
		servers:       map[string]Server{},
		order:         []string{},
		client:        client,
		stdio:         newStdioConnector(),
		toolsCache:    map[string]toolsCacheEntry{},
		toolsCacheTTL: defaultToolsCacheTTL,
	}
}

func NewFromEnv(client *http.Client) (*Store, error) {
	store := NewStore(client)
	if rawTTL := strings.TrimSpace(os.Getenv("MCP_TOOLS_CACHE_TTL_MS")); rawTTL != "" {
		ms, err := strconv.Atoi(rawTTL)
		if err != nil || ms <= 0 {
			return nil, fmt.Errorf("invalid MCP_TOOLS_CACHE_TTL_MS: %q", rawTTL)
		}
		store.SetToolsCacheTTL(time.Duration(ms) * time.Millisecond)
	}
	raw := strings.TrimSpace(os.Getenv("MCP_SERVERS_JSON"))
	if raw == "" {
		return store, nil
	}
	var inputs []RegisterInput
	if err := json.Unmarshal([]byte(raw), &inputs); err != nil {
		return nil, fmt.Errorf("invalid MCP_SERVERS_JSON: %w", err)
	}
	for _, in := range inputs {
		if _, err := store.Register(in); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func (s *Store) SetToolsCacheTTL(ttl time.Duration) {
	if ttl <= 0 {
		ttl = defaultToolsCacheTTL
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolsCacheTTL = ttl
}

func (s *Store) Register(in RegisterInput) (Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = s.nextIDLocked()
	}
	if _, exists := s.servers[id]; exists {
		return Server{}, fmt.Errorf("%w: %s", ErrAlreadyExists, id)
	}
	now := time.Now().UTC()
	server := Server{
		ID:        id,
		Type:      "mcp_server",
		Name:      strings.TrimSpace(in.Name),
		Transport: normalizeTransport(in.Transport),
		URL:       strings.TrimSpace(in.URL),
		Command:   strings.TrimSpace(in.Command),
		Args:      sanitizeList(in.Args),
		Env:       copyStringMap(in.Env),
		Headers:   copyStringMap(in.Headers),
		TimeoutMS: in.TimeoutMS,
		Retries:   in.Retries,
		Enabled:   true,
		Metadata:  copyAnyMap(in.Metadata),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if in.Enabled != nil {
		server.Enabled = *in.Enabled
	}
	if err := sanitizeAndValidate(&server); err != nil {
		return Server{}, err
	}

	s.servers[id] = server
	s.order = append(s.order, id)
	return cloneServer(server), nil
}

func (s *Store) Update(id string, in UpdateInput) (Server, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Server{}, fmt.Errorf("server id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	server, ok := s.servers[id]
	if !ok {
		return Server{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	prevTransport := server.Transport
	if in.Name != nil {
		server.Name = strings.TrimSpace(*in.Name)
	}
	if in.Transport != nil {
		server.Transport = normalizeTransport(*in.Transport)
	}
	if in.URL != nil {
		server.URL = strings.TrimSpace(*in.URL)
	}
	if in.Command != nil {
		server.Command = strings.TrimSpace(*in.Command)
	}
	if in.Args != nil {
		server.Args = sanitizeList(*in.Args)
	}
	if in.Env != nil {
		server.Env = copyStringMap(*in.Env)
	}
	if in.Headers != nil {
		server.Headers = copyStringMap(*in.Headers)
	}
	if in.TimeoutMS != nil {
		server.TimeoutMS = *in.TimeoutMS
	}
	if in.Retries != nil {
		server.Retries = *in.Retries
	}
	if in.Enabled != nil {
		server.Enabled = *in.Enabled
	}
	if in.Metadata != nil {
		server.Metadata = copyAnyMap(*in.Metadata)
	}
	server.UpdatedAt = time.Now().UTC()
	if err := sanitizeAndValidate(&server); err != nil {
		return Server{}, err
	}
	if prevTransport == TransportStdio || server.Transport == TransportStdio {
		s.stdio.Stop(id)
	}
	s.invalidateToolsCacheLocked(id)
	s.servers[id] = server
	return cloneServer(server), nil
}

func (s *Store) Delete(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("server id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.servers[id]; !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if existing, ok := s.servers[id]; ok && existing.Transport == TransportStdio {
		s.stdio.Stop(id)
	}
	delete(s.servers, id)
	s.invalidateToolsCacheLocked(id)
	next := make([]string, 0, len(s.order))
	for _, existing := range s.order {
		if existing != id {
			next = append(next, existing)
		}
	}
	s.order = next
	return nil
}

func (s *Store) Get(id string) (Server, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Server{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	server, ok := s.servers[id]
	if !ok {
		return Server{}, false
	}
	return cloneServer(server), true
}

func (s *Store) List(limit int) []Server {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.order) {
		limit = len(s.order)
	}
	out := make([]Server, 0, limit)
	for i := len(s.order) - 1; i >= 0 && len(out) < limit; i-- {
		id := s.order[i]
		if server, ok := s.servers[id]; ok {
			out = append(out, cloneServer(server))
		}
	}
	return out
}

func (s *Store) CheckHealth(ctx context.Context, id string) (Server, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Server{}, fmt.Errorf("server id is required")
	}

	s.mu.RLock()
	server, ok := s.servers[id]
	s.mu.RUnlock()
	if !ok {
		return Server{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}

	status := s.runHealth(ctx, server)

	s.mu.Lock()
	defer s.mu.Unlock()
	server, ok = s.servers[id]
	if !ok {
		return Server{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	server.Status = status
	server.UpdatedAt = time.Now().UTC()
	if !status.Healthy {
		s.invalidateToolsCacheLocked(id)
	}
	s.servers[id] = server
	return cloneServer(server), nil
}

func (s *Store) Reconnect(ctx context.Context, id string) (Server, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Server{}, fmt.Errorf("server id is required")
	}

	s.mu.RLock()
	server, ok := s.servers[id]
	s.mu.RUnlock()
	if !ok {
		return Server{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	s.mu.Lock()
	s.invalidateToolsCacheLocked(id)
	s.mu.Unlock()

	started := time.Now()
	status := HealthStatus{}
	if !server.Enabled {
		status.Healthy = false
		status.LastError = "server is disabled"
		status.LastCheckedAt = time.Now().UTC()
		status.LastLatencyMS = time.Since(started).Milliseconds()
	} else {
		var err error
		if server.Transport == TransportStdio {
			err = s.stdio.Reconnect(ctx, server)
		} else {
			err = s.checkHTTP(ctx, server)
		}
		status.Healthy = err == nil
		if err != nil {
			status.LastError = err.Error()
		}
		status.LastCheckedAt = time.Now().UTC()
		status.LastLatencyMS = time.Since(started).Milliseconds()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	server, ok = s.servers[id]
	if !ok {
		return Server{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	server.Status = status
	server.UpdatedAt = time.Now().UTC()
	if !status.Healthy {
		s.invalidateToolsCacheLocked(id)
	}
	s.servers[id] = server
	return cloneServer(server), nil
}

func (s *Store) ListTools(ctx context.Context, id string) ([]Tool, error) {
	server, err := s.serverByID(id)
	if err != nil {
		return nil, err
	}
	if !server.Enabled {
		return nil, fmt.Errorf("mcp server %q is disabled", server.ID)
	}
	if cached, ok := s.getCachedTools(server.ID); ok {
		return cached, nil
	}
	result, err := s.rpcRequest(ctx, server, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	tools := parseTools(result)
	s.putCachedTools(server.ID, tools)
	return cloneTools(tools), nil
}

func (s *Store) CallTool(ctx context.Context, id, name string, input map[string]any) (ToolCallResult, error) {
	server, err := s.serverByID(id)
	if err != nil {
		return ToolCallResult{}, err
	}
	if !server.Enabled {
		return ToolCallResult{}, fmt.Errorf("mcp server %q is disabled", server.ID)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return ToolCallResult{}, fmt.Errorf("tool name is required")
	}
	if input == nil {
		input = map[string]any{}
	}
	result, err := s.rpcRequest(ctx, server, "tools/call", map[string]any{
		"name":      name,
		"arguments": input,
	})
	if err != nil {
		if isToolNotFoundError(err) {
			s.mu.Lock()
			s.invalidateToolsCacheLocked(server.ID)
			s.mu.Unlock()
			return ToolCallResult{}, fmt.Errorf("%w: %s", ErrToolNotFound, name)
		}
		return ToolCallResult{}, err
	}
	parsed := parseToolCallResult(result)
	parsed.ServerID = server.ID
	parsed.ToolName = name
	return parsed, nil
}

func (s *Store) CallToolAny(ctx context.Context, name string, input map[string]any) (ToolCallResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ToolCallResult{}, fmt.Errorf("tool name is required")
	}
	servers := s.List(0)
	var lastErr error
	for _, server := range servers {
		if !server.Enabled {
			continue
		}
		tools, err := s.ListTools(ctx, server.ID)
		if err != nil {
			continue
		}
		if !toolExists(tools, name) {
			continue
		}
		res, err := s.CallTool(ctx, server.ID, name, input)
		if err != nil {
			lastErr = err
			continue
		}
		return res, nil
	}
	if lastErr != nil {
		return ToolCallResult{}, lastErr
	}
	return ToolCallResult{}, fmt.Errorf("%w: %s", ErrToolNotFound, name)
}

func (s *Store) runHealth(ctx context.Context, server Server) HealthStatus {
	started := time.Now()
	status := HealthStatus{}
	if !server.Enabled {
		status.Healthy = false
		status.LastError = "server is disabled"
		status.LastCheckedAt = time.Now().UTC()
		status.LastLatencyMS = time.Since(started).Milliseconds()
		return status
	}

	var err error
	switch server.Transport {
	case TransportHTTP:
		err = s.checkHTTP(ctx, server)
	case TransportStdio:
		err = s.checkStdio(ctx, server)
	default:
		err = fmt.Errorf("unsupported transport %q", server.Transport)
	}
	status.Healthy = err == nil
	if err != nil {
		status.LastError = err.Error()
	}
	status.LastCheckedAt = time.Now().UTC()
	status.LastLatencyMS = time.Since(started).Milliseconds()
	return status
}

func (s *Store) checkHTTP(ctx context.Context, server Server) error {
	timeout := time.Duration(server.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	hctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(hctx, http.MethodGet, server.URL, nil)
	if err != nil {
		return err
	}
	for k, v := range server.Headers {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k != "" && v != "" {
			req.Header.Set(k, v)
		}
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}
	return nil
}

func (s *Store) checkStdio(ctx context.Context, server Server) error {
	return s.stdio.Check(ctx, server)
}

func (s *Store) serverByID(id string) (Server, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Server{}, fmt.Errorf("server id is required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	server, ok := s.servers[id]
	if !ok {
		return Server{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return cloneServer(server), nil
}

func (s *Store) rpcRequest(ctx context.Context, server Server, method string, params map[string]any) (map[string]any, error) {
	var (
		result map[string]any
		err    error
	)
	retries := server.Retries
	if retries < 0 {
		retries = 0
	}
	for attempt := 0; attempt <= retries; attempt++ {
		switch server.Transport {
		case TransportHTTP:
			result, err = s.requestHTTPRPC(ctx, server, method, params)
		case TransportStdio:
			result, err = s.stdio.Request(ctx, server, method, params)
		default:
			err = fmt.Errorf("unsupported transport %q", server.Transport)
		}
		if err == nil {
			return result, nil
		}
	}
	return nil, err
}

func (s *Store) requestHTTPRPC(ctx context.Context, server Server, method string, params map[string]any) (map[string]any, error) {
	timeout := time.Duration(server.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	hctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	reqID := atomic.AddUint64(&s.counter, 1)
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      reqID,
		"method":  method,
		"params":  params,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(hctx, http.MethodPost, server.URL, strings.NewReader(string(raw)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	for k, v := range server.Headers {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k != "" && v != "" {
			req.Header.Set(k, v)
		}
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("rpc status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("invalid rpc response: %w", err)
	}
	if rpcErr := extractRPCError(out); rpcErr != "" {
		return nil, fmt.Errorf("rpc error: %s", rpcErr)
	}
	if result, ok := out["result"].(map[string]any); ok {
		return result, nil
	}
	if resultAny, ok := out["result"]; ok {
		return map[string]any{"_result": resultAny}, nil
	}
	return map[string]any{}, nil
}

func (s *Store) getCachedTools(serverID string) ([]Tool, bool) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.toolsCache[serverID]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return cloneTools(entry.tools), true
}

func (s *Store) putCachedTools(serverID string, tools []Tool) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolsCache[serverID] = toolsCacheEntry{
		tools:     cloneTools(tools),
		expiresAt: time.Now().Add(s.toolsCacheTTL),
	}
}

func (s *Store) invalidateToolsCacheLocked(serverID string) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return
	}
	delete(s.toolsCache, serverID)
}

func extractRPCError(resp map[string]any) string {
	if len(resp) == 0 {
		return ""
	}
	v, ok := resp["error"]
	if !ok || v == nil {
		return ""
	}
	switch e := v.(type) {
	case string:
		return strings.TrimSpace(e)
	case map[string]any:
		if msg, ok := e["message"].(string); ok && strings.TrimSpace(msg) != "" {
			return strings.TrimSpace(msg)
		}
		raw, _ := json.Marshal(e)
		return string(raw)
	default:
		raw, _ := json.Marshal(e)
		return string(raw)
	}
}

func parseTools(result map[string]any) []Tool {
	raw, ok := result["tools"]
	if !ok {
		if embedded, ok := result["_result"].(map[string]any); ok {
			raw = embedded["tools"]
		}
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]Tool, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := obj["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		desc, _ := obj["description"].(string)
		schema, _ := obj["inputSchema"].(map[string]any)
		if schema == nil {
			schema, _ = obj["input_schema"].(map[string]any)
		}
		out = append(out, Tool{
			Name:        name,
			Description: strings.TrimSpace(desc),
			InputSchema: copyAnyMap(schema),
		})
	}
	return out
}

func parseToolCallResult(result map[string]any) ToolCallResult {
	out := ToolCallResult{}
	if isErr, ok := result["isError"].(bool); ok {
		out.IsError = isErr
	}
	if !out.IsError {
		if isErr, ok := result["is_error"].(bool); ok {
			out.IsError = isErr
		}
	}
	if content, ok := result["content"]; ok {
		out.Content = content
		return out
	}
	if embedded, ok := result["_result"]; ok {
		out.Content = embedded
		return out
	}
	out.Content = result
	return out
}

func toolExists(tools []Tool, name string) bool {
	name = strings.TrimSpace(name)
	for _, tool := range tools {
		if strings.EqualFold(strings.TrimSpace(tool.Name), name) {
			return true
		}
	}
	return false
}

func isToolNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrToolNotFound) {
		return true
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "tool not found") ||
		strings.Contains(msg, "unknown tool") ||
		strings.Contains(msg, "no such tool")
}

func (s *Store) nextIDLocked() string {
	n := atomic.AddUint64(&s.counter, 1)
	return fmt.Sprintf("mcp_%d_%x", time.Now().Unix(), n)
}

func sanitizeAndValidate(server *Server) error {
	if server == nil {
		return fmt.Errorf("server is nil")
	}
	server.Name = strings.TrimSpace(server.Name)
	if server.Name == "" {
		return fmt.Errorf("name is required")
	}
	server.Transport = normalizeTransport(server.Transport)
	if server.Transport == "" {
		return fmt.Errorf("transport must be one of: http, stdio")
	}
	server.Args = sanitizeList(server.Args)
	server.Env = copyStringMap(server.Env)
	server.Headers = copyStringMap(server.Headers)
	server.Metadata = copyAnyMap(server.Metadata)
	if server.TimeoutMS <= 0 {
		server.TimeoutMS = 8000
	}
	if server.Retries < 0 {
		server.Retries = 0
	}
	if server.Retries == 0 {
		server.Retries = 1
	}
	switch server.Transport {
	case TransportHTTP:
		server.URL = strings.TrimSpace(server.URL)
		if server.URL == "" {
			return fmt.Errorf("url is required for http transport")
		}
		u, err := url.Parse(server.URL)
		if err != nil || strings.TrimSpace(u.Scheme) == "" || strings.TrimSpace(u.Host) == "" {
			return fmt.Errorf("url is invalid")
		}
	case TransportStdio:
		server.Command = strings.TrimSpace(server.Command)
		if server.Command == "" {
			return fmt.Errorf("command is required for stdio transport")
		}
	}
	return nil
}

func normalizeTransport(t Transport) Transport {
	switch strings.ToLower(strings.TrimSpace(string(t))) {
	case string(TransportHTTP):
		return TransportHTTP
	case string(TransportStdio):
		return TransportStdio
	default:
		return ""
	}
}

func cloneServer(in Server) Server {
	out := in
	out.Args = append([]string(nil), in.Args...)
	out.Env = copyStringMap(in.Env)
	out.Headers = copyStringMap(in.Headers)
	out.Metadata = copyAnyMap(in.Metadata)
	return out
}

func cloneTools(in []Tool) []Tool {
	if len(in) == 0 {
		return nil
	}
	out := make([]Tool, 0, len(in))
	for _, item := range in {
		cloned := item
		cloned.Name = strings.TrimSpace(cloned.Name)
		cloned.Description = strings.TrimSpace(cloned.Description)
		cloned.InputSchema = copyAnyMap(cloned.InputSchema)
		out = append(out, cloned)
	}
	return out
}

func sanitizeList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" {
			continue
		}
		out[k] = v
	}
	return out
}

func copyAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = v
	}
	return out
}
