package upstream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"ccgateway/internal/orchestrator"
)

const (
	scriptAdapterProtocolVersion = "ccgateway.script_adapter.v1"
	defaultScriptTimeout         = 90 * time.Second
	defaultScriptMaxOutputBytes  = 4 << 20 // 4MB
)

// ScriptAdapterConfig defines a generic command-based adapter.
//
// The command reads a JSON payload from stdin and writes JSON to stdout.
// Payload format:
//
//	{
//	  "version": "ccgateway.script_adapter.v1",
//	  "mode": "complete" | "stream",
//	  "request": { ... canonical request ... }
//	}
//
// For complete mode, the script should output one JSON object:
//
//	{
//	  "model": "...",
//	  "blocks": [{"type":"text","text":"..."}],
//	  "stop_reason": "end_turn",
//	  "usage": {"input_tokens": 1, "output_tokens": 2}
//	}
//
// For stream mode, the script may output NDJSON frames:
// {"type":"event","event":{...stream event...}}
// {"type":"response","response":{...final canonical response...}}
// If stream mode is not implemented, returning a single complete response is also supported.
type ScriptAdapterConfig struct {
	Name           string            `json:"name"`
	Command        string            `json:"command"`
	Args           []string          `json:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	WorkDir        string            `json:"work_dir,omitempty"`
	Model          string            `json:"model,omitempty"`
	SupportsVision *bool             `json:"supports_vision,omitempty"`
	SupportsTools  *bool             `json:"supports_tools,omitempty"`
	TimeoutMS      int               `json:"timeout_ms,omitempty"`
	MaxOutputBytes int               `json:"max_output_bytes,omitempty"`
}

type ScriptAdapter struct {
	name           string
	command        string
	args           []string
	env            map[string]string
	workDir        string
	model          string
	supportsVision *bool
	supportsTools  *bool
	timeout        time.Duration
	maxOutputBytes int
}

func NewScriptAdapter(cfg ScriptAdapterConfig) (*ScriptAdapter, error) {
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return nil, fmt.Errorf("script adapter name is required")
	}
	command := strings.TrimSpace(cfg.Command)
	if command == "" {
		return nil, fmt.Errorf("script adapter %q command is required", name)
	}
	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = defaultScriptTimeout
	}
	maxOutput := cfg.MaxOutputBytes
	if maxOutput <= 0 {
		maxOutput = defaultScriptMaxOutputBytes
	}

	return &ScriptAdapter{
		name:           name,
		command:        command,
		args:           append([]string(nil), cfg.Args...),
		env:            copyStringMap(cfg.Env),
		workDir:        strings.TrimSpace(cfg.WorkDir),
		model:          strings.TrimSpace(cfg.Model),
		supportsVision: cloneBoolPtr(cfg.SupportsVision),
		supportsTools:  cloneBoolPtr(cfg.SupportsTools),
		timeout:        timeout,
		maxOutputBytes: maxOutput,
	}, nil
}

func (a *ScriptAdapter) Name() string {
	return a.name
}

func (a *ScriptAdapter) ModelHint() string {
	return a.model
}

func (a *ScriptAdapter) AdminSpec() AdapterSpec {
	timeoutMS := int(a.timeout / time.Millisecond)
	return AdapterSpec{
		Name:           a.name,
		Kind:           AdapterKindScript,
		Command:        a.command,
		Args:           append([]string(nil), a.args...),
		Env:            copyStringMap(a.env),
		WorkDir:        a.workDir,
		Model:          a.model,
		SupportsVision: cloneBoolPtr(a.supportsVision),
		SupportsTools:  cloneBoolPtr(a.supportsTools),
		TimeoutMS:      timeoutMS,
		MaxOutputBytes: a.maxOutputBytes,
	}
}

func (a *ScriptAdapter) Complete(ctx context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	runCtx, cancel := a.withTimeout(ctx)
	defer cancel()

	raw, stderr, err := a.runOnce(runCtx, scriptEnvelope{
		Version: scriptAdapterProtocolVersion,
		Mode:    "complete",
		Request: buildScriptRequest(req),
	})
	if err != nil {
		return orchestrator.Response{}, withScriptStderr(err, stderr)
	}
	resp, err := decodeScriptResponse(raw)
	if err != nil {
		return orchestrator.Response{}, fmt.Errorf("script adapter %q response decode failed: %w", a.name, err)
	}
	if strings.TrimSpace(resp.Model) == "" {
		resp.Model = req.Model
	}
	return resp, nil
}

func (a *ScriptAdapter) Stream(ctx context.Context, req orchestrator.Request) (<-chan orchestrator.StreamEvent, <-chan error) {
	events := make(chan orchestrator.StreamEvent, 32)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		runCtx, cancel := a.withTimeout(ctx)
		defer cancel()

		cmd := exec.CommandContext(runCtx, a.command, a.args...)
		cmd.Env = mergeEnv(os.Environ(), a.env)
		if a.workDir != "" {
			cmd.Dir = a.workDir
		}

		stdin, err := cmd.StdinPipe()
		if err != nil {
			errs <- fmt.Errorf("script adapter %q stdin pipe failed: %w", a.name, err)
			return
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			errs <- fmt.Errorf("script adapter %q stdout pipe failed: %w", a.name, err)
			return
		}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Start(); err != nil {
			errs <- fmt.Errorf("script adapter %q start failed: %w", a.name, err)
			return
		}

		if err := json.NewEncoder(stdin).Encode(scriptEnvelope{
			Version: scriptAdapterProtocolVersion,
			Mode:    "stream",
			Request: buildScriptRequest(req),
		}); err != nil {
			_ = stdin.Close()
			_ = cmd.Wait()
			errs <- fmt.Errorf("script adapter %q write request failed: %w", a.name, err)
			return
		}
		_ = stdin.Close()

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), a.maxOutputBytes)

		emitted := false
		var fallbackResp *orchestrator.Response

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			ev, resp, err := decodeScriptStreamLine([]byte(line))
			if err != nil {
				_ = cmd.Wait()
				errs <- withScriptStderr(fmt.Errorf("script adapter %q stream decode failed: %w", a.name, err), stderr.String())
				return
			}
			if ev != nil {
				events <- *ev
				emitted = true
				continue
			}
			if resp != nil {
				copyResp := *resp
				fallbackResp = &copyResp
			}
		}
		if err := scanner.Err(); err != nil {
			_ = cmd.Wait()
			errs <- withScriptStderr(fmt.Errorf("script adapter %q stream read failed: %w", a.name, err), stderr.String())
			return
		}

		if err := cmd.Wait(); err != nil {
			errs <- withScriptStderr(fmt.Errorf("script adapter %q stream execution failed: %w", a.name, err), stderr.String())
			return
		}

		if fallbackResp != nil && strings.TrimSpace(fallbackResp.Model) == "" {
			fallbackResp.Model = req.Model
		}
		if !emitted {
			if fallbackResp == nil {
				errs <- withScriptStderr(fmt.Errorf("script adapter %q stream produced no events or response", a.name), stderr.String())
				return
			}
			emitResponseAsStream(events, *fallbackResp)
		}
	}()

	return events, errs
}

func (a *ScriptAdapter) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if deadline, ok := ctx.Deadline(); ok {
		if time.Until(deadline) <= a.timeout {
			return context.WithCancel(ctx)
		}
	}
	return context.WithTimeout(ctx, a.timeout)
}

func (a *ScriptAdapter) runOnce(ctx context.Context, payload scriptEnvelope) ([]byte, string, error) {
	cmd := exec.CommandContext(ctx, a.command, a.args...)
	cmd.Env = mergeEnv(os.Environ(), a.env)
	if a.workDir != "" {
		cmd.Dir = a.workDir
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, stderr.String(), fmt.Errorf("script adapter %q stdin pipe failed: %w", a.name, err)
	}
	if err := cmd.Start(); err != nil {
		return nil, stderr.String(), fmt.Errorf("script adapter %q start failed: %w", a.name, err)
	}

	enc := json.NewEncoder(stdin)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		return nil, stderr.String(), fmt.Errorf("script adapter %q write request failed: %w", a.name, err)
	}
	_ = stdin.Close()

	if err := cmd.Wait(); err != nil {
		return nil, stderr.String(), fmt.Errorf("script adapter %q execution failed: %w", a.name, err)
	}
	if stdout.Len() > a.maxOutputBytes {
		return nil, stderr.String(), fmt.Errorf("script adapter %q output exceeds limit %d bytes", a.name, a.maxOutputBytes)
	}
	return stdout.Bytes(), stderr.String(), nil
}

type scriptEnvelope struct {
	Version string             `json:"version"`
	Mode    string             `json:"mode"`
	Request scriptRequestInput `json:"request"`
}

type scriptRequestInput struct {
	RunID     string               `json:"run_id,omitempty"`
	Model     string               `json:"model"`
	MaxTokens int                  `json:"max_tokens"`
	System    any                  `json:"system,omitempty"`
	Messages  []scriptMessageInput `json:"messages"`
	Tools     []scriptToolInput    `json:"tools,omitempty"`
	Metadata  map[string]any       `json:"metadata,omitempty"`
	Headers   map[string]string    `json:"headers,omitempty"`
}

type scriptMessageInput struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type scriptToolInput struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

func buildScriptRequest(req orchestrator.Request) scriptRequestInput {
	messages := make([]scriptMessageInput, 0, len(req.Messages))
	for _, m := range req.Messages {
		messages = append(messages, scriptMessageInput{
			Role:    strings.TrimSpace(m.Role),
			Content: m.Content,
		})
	}
	tools := make([]scriptToolInput, 0, len(req.Tools))
	for _, t := range req.Tools {
		desc := strings.TrimSpace(t.Description)
		if desc == "" {
			desc = "No description provided"
		}
		tools = append(tools, scriptToolInput{
			Name:        strings.TrimSpace(t.Name),
			Description: desc,
			InputSchema: copyAnyMap(t.InputSchema),
		})
	}
	return scriptRequestInput{
		RunID:     strings.TrimSpace(req.RunID),
		Model:     strings.TrimSpace(req.Model),
		MaxTokens: req.MaxTokens,
		System:    req.System,
		Messages:  messages,
		Tools:     tools,
		Metadata:  copyAnyMap(req.Metadata),
		Headers:   copyHeaders(req.Headers),
	}
}

type scriptResponsePayload struct {
	Model        string                 `json:"model"`
	Blocks       []scriptAssistantBlock `json:"blocks"`
	Content      []scriptAssistantBlock `json:"content"`
	Text         string                 `json:"text"`
	StopReason   string                 `json:"stop_reason"`
	Stop         string                 `json:"stop"`
	FinishReason string                 `json:"finish_reason"`
	Usage        scriptUsagePayload     `json:"usage"`
}

type scriptAssistantBlock struct {
	Type  string         `json:"type"`
	Text  string         `json:"text,omitempty"`
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`
}

type scriptUsagePayload struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

func decodeScriptResponse(raw []byte) (orchestrator.Response, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return orchestrator.Response{}, fmt.Errorf("empty response")
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return orchestrator.Response{}, err
	}
	if nested, ok := top["response"]; ok && len(bytes.TrimSpace(nested)) > 0 {
		return decodeScriptResponseObject(nested)
	}
	return decodeScriptResponseObject(raw)
}

func decodeScriptResponseObject(raw json.RawMessage) (orchestrator.Response, error) {
	var payload scriptResponsePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return orchestrator.Response{}, err
	}
	blocks := payload.Blocks
	if len(blocks) == 0 && len(payload.Content) > 0 {
		blocks = payload.Content
	}
	if len(blocks) == 0 && strings.TrimSpace(payload.Text) != "" {
		blocks = []scriptAssistantBlock{{Type: "text", Text: payload.Text}}
	}

	outBlocks := make([]orchestrator.AssistantBlock, 0, len(blocks))
	for _, b := range blocks {
		t := strings.TrimSpace(b.Type)
		if t == "" {
			if strings.TrimSpace(b.Name) != "" {
				t = "tool_use"
			} else {
				t = "text"
			}
		}
		outBlocks = append(outBlocks, orchestrator.AssistantBlock{
			Type:  t,
			Text:  b.Text,
			ID:    strings.TrimSpace(b.ID),
			Name:  strings.TrimSpace(b.Name),
			Input: copyAnyMap(b.Input),
		})
	}

	stopReason := strings.TrimSpace(payload.StopReason)
	if stopReason == "" {
		stopReason = strings.TrimSpace(payload.Stop)
	}
	if stopReason == "" {
		stopReason = strings.TrimSpace(payload.FinishReason)
	}
	if stopReason == "" {
		stopReason = "end_turn"
	}

	inputTokens := payload.Usage.InputTokens
	if inputTokens <= 0 {
		inputTokens = payload.Usage.PromptTokens
	}
	outputTokens := payload.Usage.OutputTokens
	if outputTokens <= 0 {
		outputTokens = payload.Usage.CompletionTokens
	}

	return orchestrator.Response{
		Model:      strings.TrimSpace(payload.Model),
		Blocks:     outBlocks,
		StopReason: stopReason,
		Usage: orchestrator.Usage{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		},
	}, nil
}

func decodeScriptStreamLine(line []byte) (*orchestrator.StreamEvent, *orchestrator.Response, error) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return nil, nil, nil
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(line, &top); err != nil {
		return nil, nil, err
	}

	if rawType, ok := top["type"]; ok {
		var typ string
		if err := json.Unmarshal(rawType, &typ); err == nil {
			typ = strings.TrimSpace(strings.ToLower(typ))
			switch typ {
			case "error":
				msg := decodeErrorMessage(top)
				if msg == "" {
					msg = "script stream returned error"
				}
				return nil, nil, fmt.Errorf(msg)
			case "event":
				if rawEvent, ok := top["event"]; ok {
					ev, err := decodeStreamEvent(rawEvent)
					return ev, nil, err
				}
			case "response", "final":
				if rawResp, ok := top["response"]; ok {
					resp, err := decodeScriptResponse(rawResp)
					if err != nil {
						return nil, nil, err
					}
					return nil, &resp, nil
				}
			}
			if isStreamEventType(typ) {
				ev, err := decodeStreamEvent(line)
				return ev, nil, err
			}
		}
	}

	if rawEvent, ok := top["event"]; ok {
		ev, err := decodeStreamEvent(rawEvent)
		return ev, nil, err
	}
	if rawResp, ok := top["response"]; ok {
		resp, err := decodeScriptResponse(rawResp)
		if err != nil {
			return nil, nil, err
		}
		return nil, &resp, nil
	}
	if looksLikeResponse(top) {
		resp, err := decodeScriptResponse(line)
		if err != nil {
			return nil, nil, err
		}
		return nil, &resp, nil
	}
	if looksLikeEvent(top) {
		ev, err := decodeStreamEvent(line)
		return ev, nil, err
	}

	return nil, nil, fmt.Errorf("unknown stream frame format")
}

func decodeStreamEvent(raw json.RawMessage) (*orchestrator.StreamEvent, error) {
	var payload struct {
		Type        string               `json:"type"`
		Index       int                  `json:"index"`
		Block       scriptAssistantBlock `json:"block"`
		DeltaText   string               `json:"delta_text"`
		DeltaJSON   string               `json:"delta_json"`
		StopReason  string               `json:"stop_reason"`
		Usage       scriptUsagePayload   `json:"usage"`
		RawEvent    string               `json:"raw_event"`
		RawData     json.RawMessage      `json:"raw_data"`
		PassThrough bool                 `json:"pass_through"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	evType := strings.TrimSpace(payload.Type)
	if evType == "" {
		return nil, fmt.Errorf("stream event type is required")
	}
	ev := &orchestrator.StreamEvent{
		Type:       evType,
		Index:      payload.Index,
		DeltaText:  payload.DeltaText,
		DeltaJSON:  payload.DeltaJSON,
		StopReason: payload.StopReason,
		Usage: orchestrator.Usage{
			InputTokens:  firstPositive(payload.Usage.InputTokens, payload.Usage.PromptTokens),
			OutputTokens: firstPositive(payload.Usage.OutputTokens, payload.Usage.CompletionTokens),
		},
		RawEvent:    payload.RawEvent,
		PassThrough: payload.PassThrough,
		Block: orchestrator.AssistantBlock{
			Type:  strings.TrimSpace(payload.Block.Type),
			Text:  payload.Block.Text,
			ID:    strings.TrimSpace(payload.Block.ID),
			Name:  strings.TrimSpace(payload.Block.Name),
			Input: copyAnyMap(payload.Block.Input),
		},
	}
	if len(payload.RawData) > 0 {
		ev.RawData = decodeRawData(payload.RawData)
	}
	return ev, nil
}

func decodeRawData(raw json.RawMessage) []byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}
	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		return []byte(text)
	}
	out := make([]byte, len(trimmed))
	copy(out, trimmed)
	return out
}

func decodeErrorMessage(top map[string]json.RawMessage) string {
	if raw, ok := top["error"]; ok {
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			return strings.TrimSpace(text)
		}
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err == nil {
			if msg, ok := obj["message"].(string); ok {
				return strings.TrimSpace(msg)
			}
		}
	}
	return ""
}

func looksLikeResponse(top map[string]json.RawMessage) bool {
	if len(top) == 0 {
		return false
	}
	if _, ok := top["response"]; ok {
		return true
	}
	if _, ok := top["blocks"]; ok {
		return true
	}
	if _, ok := top["content"]; ok {
		return true
	}
	if _, ok := top["text"]; ok {
		return true
	}
	if _, ok := top["stop_reason"]; ok {
		return true
	}
	if _, ok := top["usage"]; ok {
		return true
	}
	return false
}

func looksLikeEvent(top map[string]json.RawMessage) bool {
	rawType, ok := top["type"]
	if !ok {
		return false
	}
	var typ string
	if err := json.Unmarshal(rawType, &typ); err != nil {
		return false
	}
	return isStreamEventType(strings.ToLower(strings.TrimSpace(typ)))
}

func isStreamEventType(typ string) bool {
	switch strings.TrimSpace(strings.ToLower(typ)) {
	case "message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop":
		return true
	default:
		return false
	}
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mergeEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return append([]string(nil), base...)
	}
	out := append([]string(nil), base...)
	index := make(map[string]int, len(out))
	for i, item := range out {
		key := item
		if pos := strings.IndexByte(item, '='); pos >= 0 {
			key = item[:pos]
		}
		index[key] = i
	}
	for k, v := range overrides {
		entry := k + "=" + v
		if i, ok := index[k]; ok {
			out[i] = entry
			continue
		}
		index[k] = len(out)
		out = append(out, entry)
	}
	return out
}

func withScriptStderr(err error, stderr string) error {
	stderr = strings.TrimSpace(stderr)
	if err == nil || stderr == "" {
		return err
	}
	return fmt.Errorf("%w (stderr: %s)", err, truncateForError(stderr, 800))
}

func truncateForError(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}

func firstPositive(primary, fallback int) int {
	if primary > 0 {
		return primary
	}
	if fallback > 0 {
		return fallback
	}
	return 0
}
