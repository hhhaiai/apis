package mcpregistry

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type stdioConnector struct {
	mu    sync.Mutex
	procs map[string]*stdioProcess
}

type stdioProcess struct {
	id     string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr *bytes.Buffer
	nextID int64
	ready  bool
}

func newStdioConnector() *stdioConnector {
	return &stdioConnector{
		procs: map[string]*stdioProcess{},
	}
}

func (c *stdioConnector) Check(ctx context.Context, server Server) error {
	return c.check(ctx, server, false)
}

func (c *stdioConnector) Reconnect(ctx context.Context, server Server) error {
	return c.check(ctx, server, true)
}

func (c *stdioConnector) Request(ctx context.Context, server Server, method string, params map[string]any) (map[string]any, error) {
	return c.request(ctx, server, method, params, false)
}

func (c *stdioConnector) Stop(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopLocked(id)
}

func (c *stdioConnector) check(ctx context.Context, server Server, forceRestart bool) error {
	retries := server.Retries
	if retries < 0 {
		retries = 0
	}
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		restart := forceRestart || attempt > 0
		if err := c.checkOnce(ctx, server, restart); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("stdio connector check failed")
	}
	return lastErr
}

func (c *stdioConnector) request(ctx context.Context, server Server, method string, params map[string]any, forceRestart bool) (map[string]any, error) {
	retries := server.Retries
	if retries < 0 {
		retries = 0
	}
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		restart := forceRestart || attempt > 0
		result, err := c.requestOnce(ctx, server, method, params, restart)
		if err != nil {
			lastErr = err
			continue
		}
		return result, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("stdio connector request failed")
	}
	return nil, lastErr
}

func (c *stdioConnector) checkOnce(ctx context.Context, server Server, restart bool) error {
	timeout := time.Duration(server.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	hctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	c.mu.Lock()
	if restart {
		c.stopLocked(server.ID)
	}
	proc, err := c.ensureProcessLocked(server)
	if err != nil {
		c.mu.Unlock()
		return err
	}

	if !proc.ready {
		if _, err := c.sendRequestLocked(hctx, proc, "initialize", map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "ccgateway",
				"version": "0.1",
			},
		}); err != nil {
			err = fmt.Errorf("initialize failed: %w", err)
			c.stopLocked(server.ID)
			c.mu.Unlock()
			return err
		}
		proc.ready = true
	}

	if _, err := c.sendRequestLocked(hctx, proc, "ping", map[string]any{}); err != nil {
		err = fmt.Errorf("ping failed: %w", err)
		c.stopLocked(server.ID)
		c.mu.Unlock()
		return err
	}
	c.mu.Unlock()
	return nil
}

func (c *stdioConnector) requestOnce(ctx context.Context, server Server, method string, params map[string]any, restart bool) (map[string]any, error) {
	timeout := time.Duration(server.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	hctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	c.mu.Lock()
	if restart {
		c.stopLocked(server.ID)
	}
	proc, err := c.ensureProcessLocked(server)
	if err != nil {
		c.mu.Unlock()
		return nil, err
	}
	if !proc.ready {
		if _, err := c.sendRequestLocked(hctx, proc, "initialize", map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "ccgateway",
				"version": "0.1",
			},
		}); err != nil {
			err = fmt.Errorf("initialize failed: %w", err)
			c.stopLocked(server.ID)
			c.mu.Unlock()
			return nil, err
		}
		proc.ready = true
	}
	result, err := c.sendRequestLocked(hctx, proc, method, params)
	if err != nil {
		c.stopLocked(server.ID)
		c.mu.Unlock()
		return nil, err
	}
	c.mu.Unlock()
	return result, nil
}

func (c *stdioConnector) ensureProcessLocked(server Server) (*stdioProcess, error) {
	if proc, ok := c.procs[server.ID]; ok && proc != nil {
		if proc.cmd != nil && proc.cmd.Process != nil && proc.cmd.ProcessState == nil {
			return proc, nil
		}
		c.stopLocked(server.ID)
	}

	cmd := exec.Command(server.Command, server.Args...)
	cmd.Env = mergeEnv(os.Environ(), server.Env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderrBuf := &bytes.Buffer{}
	cmd.Stderr = stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	proc := &stdioProcess{
		id:     server.ID,
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
		stderr: stderrBuf,
		nextID: 1,
		ready:  false,
	}
	c.procs[server.ID] = proc

	// Reap process in background to avoid zombies.
	go func(p *stdioProcess, id string) {
		_ = p.cmd.Wait()
		c.mu.Lock()
		defer c.mu.Unlock()
		if existing := c.procs[id]; existing == p {
			delete(c.procs, id)
		}
	}(proc, server.ID)

	return proc, nil
}

func (c *stdioConnector) stopLocked(id string) {
	proc, ok := c.procs[id]
	if !ok || proc == nil {
		return
	}
	delete(c.procs, id)
	if proc.stdin != nil {
		_ = proc.stdin.Close()
	}
	if proc.cmd != nil && proc.cmd.Process != nil {
		_ = proc.cmd.Process.Kill()
	}
}

func (c *stdioConnector) sendRequestLocked(ctx context.Context, proc *stdioProcess, method string, params any) (map[string]any, error) {
	id := proc.nextID
	proc.nextID++
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	if err := writeJSONRPCFrame(proc.stdin, payload); err != nil {
		return nil, err
	}

	respRaw, err := readJSONRPCFrameWithContext(ctx, proc.stdout)
	if err != nil {
		return nil, err
	}
	var resp map[string]any
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		return nil, fmt.Errorf("invalid jsonrpc response: %w", err)
	}

	respID, ok := parseResponseID(resp["id"])
	if !ok {
		return nil, fmt.Errorf("jsonrpc response missing id")
	}
	if respID != id {
		return nil, fmt.Errorf("jsonrpc response id mismatch: got %d want %d", respID, id)
	}
	if rpcErr := extractRPCError(resp); rpcErr != "" {
		return nil, fmt.Errorf("rpc error: %s", rpcErr)
	}
	if result, ok := resp["result"].(map[string]any); ok {
		return result, nil
	}
	if resultAny, ok := resp["result"]; ok {
		return map[string]any{"_result": resultAny}, nil
	}
	return map[string]any{}, nil
}

func writeJSONRPCFrame(w io.Writer, payload map[string]any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(raw)); err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if f, ok := w.(interface{ Flush() error }); ok {
		_ = f.Flush()
	}
	return nil
}

func readJSONRPCFrameWithContext(ctx context.Context, r *bufio.Reader) ([]byte, error) {
	type result struct {
		raw []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		raw, err := readJSONRPCFrame(r)
		ch <- result{raw: raw, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case out := <-ch:
		return out.raw, out.err
	}
}

func readJSONRPCFrame(r *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			v := strings.TrimSpace(line[len("content-length:"):])
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("invalid content-length %q", v)
			}
			contentLength = n
		}
	}
	if contentLength <= 0 {
		return nil, fmt.Errorf("missing content-length")
	}
	raw := make([]byte, contentLength)
	if _, err := io.ReadFull(r, raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func parseResponseID(v any) (int64, bool) {
	switch x := v.(type) {
	case float64:
		return int64(x), true
	case float32:
		return int64(x), true
	case int:
		return int64(x), true
	case int32:
		return int64(x), true
	case int64:
		return x, true
	case json.Number:
		n, err := x.Int64()
		return n, err == nil
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func mergeEnv(base []string, extra map[string]string) []string {
	if len(extra) == 0 {
		return append([]string(nil), base...)
	}
	m := map[string]string{}
	for _, line := range base {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		m[parts[0]] = parts[1]
	}
	for k, v := range extra {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		m[k] = v
	}
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}
