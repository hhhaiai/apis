package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type envelope struct {
	Mode    string         `json:"mode"`
	Request canonicalInput `json:"request"`
}

type canonicalInput struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    any                `json:"system"`
	Messages  []canonicalMessage `json:"messages"`
	Tools     []canonicalTool    `json:"tools"`
	Metadata  map[string]any     `json:"metadata"`
	Headers   map[string]string  `json:"headers"`
}

type canonicalMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type canonicalTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type canonicalResponse struct {
	Model      string           `json:"model,omitempty"`
	Blocks     []map[string]any `json:"blocks,omitempty"`
	StopReason string           `json:"stop_reason,omitempty"`
	Usage      map[string]int   `json:"usage,omitempty"`
}

func main() {
	var env envelope
	if err := json.NewDecoder(os.Stdin).Decode(&env); err != nil {
		failf("decode stdin failed: %v", err)
	}

	req := env.Request
	model := getenv("OPENAI_MODEL", strings.TrimSpace(req.Model))
	if model == "" {
		model = "gpt-4o-mini"
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	payload := map[string]any{
		"model":      model,
		"max_tokens": maxTokens,
		"messages":   canonicalToOpenAIMessages(req.System, req.Messages),
	}
	if tools := canonicalToOpenAITools(req.Tools); len(tools) > 0 {
		payload["tools"] = tools
	}

	baseURL := strings.TrimRight(getenv("OPENAI_BASE_URL", "http://127.0.0.1:8000"), "/")
	endpoint := getenv("OPENAI_ENDPOINT", "/v1/chat/completions")
	timeoutSec := getenvInt("OPENAI_TIMEOUT_SEC", 120)

	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequest(http.MethodPost, baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		failf("build request failed: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		failf("request failed: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		failf("http error %d: %s", resp.StatusCode, string(raw))
	}

	out, err := parseOpenAIResponse(raw)
	if err != nil {
		failf("decode upstream failed: %v", err)
	}
	if strings.EqualFold(strings.TrimSpace(env.Mode), "stream") {
		writeJSON(map[string]any{"response": out})
		return
	}
	writeJSON(out)
}

func canonicalToOpenAIMessages(system any, msgs []canonicalMessage) []map[string]any {
	out := make([]map[string]any, 0, len(msgs)+1)
	if text := toText(system); strings.TrimSpace(text) != "" {
		out = append(out, map[string]any{"role": "system", "content": text})
	}
	for _, m := range msgs {
		role := strings.TrimSpace(m.Role)
		if role == "" {
			role = "user"
		}
		out = append(out, map[string]any{
			"role":    role,
			"content": toText(m.Content),
		})
	}
	return out
}

func canonicalToOpenAITools(tools []canonicalTool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		desc := strings.TrimSpace(t.Description)
		if desc == "" {
			desc = "No description provided"
		}
		schema := t.InputSchema
		if len(schema) == 0 {
			schema = map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        strings.TrimSpace(t.Name),
				"description": desc,
				"parameters":  schema,
			},
		})
	}
	return out
}

func parseOpenAIResponse(raw []byte) (canonicalResponse, error) {
	var payload struct {
		Model   string `json:"model"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return canonicalResponse{}, err
	}

	out := canonicalResponse{
		Model:  payload.Model,
		Blocks: []map[string]any{},
		Usage: map[string]int{
			"input_tokens":  payload.Usage.PromptTokens,
			"output_tokens": payload.Usage.CompletionTokens,
		},
	}
	if len(payload.Choices) == 0 {
		out.StopReason = "end_turn"
		return out, nil
	}
	ch := payload.Choices[0]
	if strings.TrimSpace(ch.Message.Content) != "" {
		out.Blocks = append(out.Blocks, map[string]any{
			"type": "text",
			"text": ch.Message.Content,
		})
	}
	for _, tc := range ch.Message.ToolCalls {
		input := map[string]any{}
		if strings.TrimSpace(tc.Function.Arguments) != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
				input = map[string]any{"_raw": tc.Function.Arguments}
			}
		}
		out.Blocks = append(out.Blocks, map[string]any{
			"type":  "tool_use",
			"id":    tc.ID,
			"name":  tc.Function.Name,
			"input": input,
		})
	}

	switch strings.ToLower(strings.TrimSpace(ch.FinishReason)) {
	case "tool_calls", "function_call":
		out.StopReason = "tool_use"
	case "length":
		out.StopReason = "max_tokens"
	default:
		out.StopReason = "end_turn"
	}
	return out, nil
}

func toText(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	raw, _ := json.Marshal(v)
	return string(raw)
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func writeJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func failf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
