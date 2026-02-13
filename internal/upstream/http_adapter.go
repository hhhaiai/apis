package upstream

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"ccgateway/internal/orchestrator"
)

type AdapterKind string

const (
	AdapterKindOpenAI    AdapterKind = "openai"
	AdapterKindAnthropic AdapterKind = "anthropic"
	AdapterKindGemini    AdapterKind = "gemini"
	AdapterKindCanonical AdapterKind = "canonical"
)

type HTTPAdapterConfig struct {
	Name               string            `json:"name"`
	Kind               AdapterKind       `json:"kind"`
	BaseURL            string            `json:"base_url"`
	Endpoint           string            `json:"endpoint,omitempty"`
	APIKey             string            `json:"api_key,omitempty"`
	Headers            map[string]string `json:"headers,omitempty"`
	Model              string            `json:"model,omitempty"`
	UserAgent          string            `json:"user_agent,omitempty"`
	APIKeyHeader       string            `json:"api_key_header,omitempty"`
	ForceStream        bool              `json:"force_stream,omitempty"`
	StreamOptions      map[string]any    `json:"stream_options,omitempty"`
	InsecureSkipVerify bool              `json:"insecure_skip_verify,omitempty"`
}

type HTTPAdapter struct {
	name          string
	kind          AdapterKind
	baseURL       string
	endpoint      string
	apiKey        string
	headers       map[string]string
	model         string
	userAgent     string
	apiKeyHeader  string
	forceStream   bool
	streamOptions map[string]any
	client        *http.Client
}

func NewHTTPAdapter(cfg HTTPAdapterConfig, client *http.Client) (*HTTPAdapter, error) {
	if strings.TrimSpace(cfg.Name) == "" {
		return nil, fmt.Errorf("adapter name is required")
	}
	if strings.TrimSpace(string(cfg.Kind)) == "" {
		return nil, fmt.Errorf("adapter kind is required")
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("adapter base_url is required")
	}
	if _, err := url.Parse(cfg.BaseURL); err != nil {
		return nil, fmt.Errorf("invalid base_url for adapter %q: %w", cfg.Name, err)
	}

	ep := strings.TrimSpace(cfg.Endpoint)
	if ep == "" {
		switch cfg.Kind {
		case AdapterKindOpenAI:
			ep = "/v1/chat/completions"
		case AdapterKindAnthropic:
			ep = "/v1/messages"
		case AdapterKindGemini:
			ep = "/v1beta/models/{model}:generateContent"
		case AdapterKindCanonical:
			ep = "/v1/complete"
		default:
			return nil, fmt.Errorf("unsupported adapter kind %q", cfg.Kind)
		}
	}

	if client == nil {
		if cfg.InsecureSkipVerify {
			client = &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
				},
			}
		} else {
			client = http.DefaultClient
		}
	}

	return &HTTPAdapter{
		name:          cfg.Name,
		kind:          cfg.Kind,
		baseURL:       strings.TrimRight(cfg.BaseURL, "/"),
		endpoint:      ep,
		apiKey:        cfg.APIKey,
		headers:       copyHeaders(cfg.Headers),
		model:         strings.TrimSpace(cfg.Model),
		userAgent:     strings.TrimSpace(cfg.UserAgent),
		apiKeyHeader:  strings.TrimSpace(cfg.APIKeyHeader),
		forceStream:   cfg.ForceStream,
		streamOptions: copyAnyMap(cfg.StreamOptions),
		client:        client,
	}, nil
}

func (a *HTTPAdapter) Name() string {
	return a.name
}

func (a *HTTPAdapter) ModelHint() string {
	return a.model
}

func (a *HTTPAdapter) Complete(ctx context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	switch a.kind {
	case AdapterKindOpenAI:
		return a.completeOpenAI(ctx, req)
	case AdapterKindAnthropic:
		return a.completeAnthropic(ctx, req)
	case AdapterKindGemini:
		return a.completeGemini(ctx, req)
	case AdapterKindCanonical:
		return a.completeCanonical(ctx, req)
	default:
		return orchestrator.Response{}, fmt.Errorf("unsupported adapter kind %q", a.kind)
	}
}

func (a *HTTPAdapter) Stream(ctx context.Context, req orchestrator.Request) (<-chan orchestrator.StreamEvent, <-chan error) {
	events := make(chan orchestrator.StreamEvent, 32)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		strict := boolFromAny(req.Metadata["strict_stream_passthrough"])
		switch a.kind {
		case AdapterKindAnthropic:
			if err := a.streamAnthropic(ctx, req, events); err != nil {
				errs <- err
			}
			return
		default:
			if strict {
				errs <- fmt.Errorf("%w: adapter %q kind %q", ErrStrictPassthroughUnsupported, a.name, a.kind)
				return
			}
			resp, err := a.Complete(ctx, req)
			if err != nil {
				errs <- err
				return
			}
			emitResponseAsStream(events, resp)
		}
	}()

	return events, errs
}

func (a *HTTPAdapter) completeOpenAI(ctx context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	model := req.Model
	if a.model != "" {
		model = a.model
	}

	payload := map[string]any{
		"model":      model,
		"max_tokens": req.MaxTokens,
		"messages":   canonicalToOpenAIMessages(req.System, req.Messages),
	}
	if len(req.Tools) > 0 {
		payload["tools"] = canonicalToOpenAITools(req.Tools)
	}
	if v, ok := req.Metadata["temperature"]; ok {
		payload["temperature"] = v
	}
	if v, ok := req.Metadata["top_p"]; ok {
		payload["top_p"] = v
	}

	useStream := a.forceStream || boolFromAny(req.Metadata["upstream_force_stream"])
	if useStream {
		streamOptions := mergeStreamOptions(a.streamOptions, req.Metadata["stream_options"])
		if len(streamOptions) == 0 {
			streamOptions = map[string]any{"include_usage": true}
		}
		payload["stream"] = true
		payload["stream_options"] = streamOptions

		agg, err := a.doOpenAIStream(ctx, payload, req.Headers, model)
		if err != nil {
			return orchestrator.Response{}, err
		}
		blocks := openAIBlocksFromAggregate(agg)
		stop := normalizeOpenAIStopReason(agg.FinishReason, len(agg.ToolCalls) > 0)
		return orchestrator.Response{
			Model:      req.Model,
			Blocks:     blocks,
			StopReason: stop,
			Usage:      agg.Usage,
		}, nil
	}

	raw, err := a.doJSON(ctx, payload, req.Headers, model)
	if err != nil {
		return orchestrator.Response{}, err
	}
	parsed, err := parseOpenAIJSONResponse(raw)
	if err != nil {
		return orchestrator.Response{}, err
	}

	blocks := openAIBlocksFromParsed(parsed)
	stop := normalizeOpenAIStopReason(parsed.FinishReason, len(parsed.ToolCalls) > 0)
	return orchestrator.Response{
		Model:      req.Model,
		Blocks:     blocks,
		StopReason: stop,
		Usage: orchestrator.Usage{
			InputTokens:  parsed.PromptTokens,
			OutputTokens: parsed.CompletionTokens,
		},
	}, nil
}

func (a *HTTPAdapter) completeAnthropic(ctx context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	model := req.Model
	if a.model != "" {
		model = a.model
	}

	payload := map[string]any{
		"model":      model,
		"max_tokens": req.MaxTokens,
		"messages":   canonicalToAnthropicMessages(req.Messages),
	}
	if req.System != nil {
		payload["system"] = req.System
	}
	if len(req.Tools) > 0 {
		payload["tools"] = canonicalToAnthropicTools(req.Tools)
	}
	if v, ok := req.Metadata["temperature"]; ok {
		payload["temperature"] = v
	}
	if v, ok := req.Metadata["top_p"]; ok {
		payload["top_p"] = v
	}

	raw, err := a.doJSON(ctx, payload, req.Headers, model)
	if err != nil {
		return orchestrator.Response{}, err
	}

	var out struct {
		Model   string `json:"model"`
		Content []struct {
			Type  string         `json:"type"`
			Text  string         `json:"text"`
			ID    string         `json:"id"`
			Name  string         `json:"name"`
			Input map[string]any `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return orchestrator.Response{}, fmt.Errorf("anthropic adapter decode failed: %w", err)
	}

	blocks := make([]orchestrator.AssistantBlock, 0, len(out.Content))
	for _, b := range out.Content {
		switch b.Type {
		case "text":
			blocks = append(blocks, orchestrator.AssistantBlock{
				Type: "text",
				Text: b.Text,
			})
		case "tool_use":
			blocks = append(blocks, orchestrator.AssistantBlock{
				Type:  "tool_use",
				ID:    b.ID,
				Name:  b.Name,
				Input: b.Input,
			})
		}
	}
	if len(blocks) == 0 {
		blocks = append(blocks, orchestrator.AssistantBlock{Type: "text", Text: ""})
	}
	stop := out.StopReason
	if strings.TrimSpace(stop) == "" {
		stop = "end_turn"
	}

	return orchestrator.Response{
		Model:      req.Model,
		Blocks:     blocks,
		StopReason: stop,
		Usage: orchestrator.Usage{
			InputTokens:  out.Usage.InputTokens,
			OutputTokens: out.Usage.OutputTokens,
		},
	}, nil
}

func (a *HTTPAdapter) completeGemini(ctx context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	model := req.Model
	if a.model != "" {
		model = a.model
	}

	payload := map[string]any{
		"contents": canonicalToGeminiContents(req.Messages),
		"generationConfig": map[string]any{
			"maxOutputTokens": req.MaxTokens,
		},
	}
	if sys := strings.TrimSpace(renderSystemToString(req.System)); sys != "" {
		payload["systemInstruction"] = map[string]any{
			"parts": []map[string]any{
				{"text": sys},
			},
		}
	}
	if v, ok := req.Metadata["temperature"]; ok {
		payload["generationConfig"].(map[string]any)["temperature"] = v
	}
	if v, ok := req.Metadata["top_p"]; ok {
		payload["generationConfig"].(map[string]any)["topP"] = v
	}
	if len(req.Tools) > 0 {
		payload["tools"] = []map[string]any{
			{
				"functionDeclarations": canonicalToGeminiToolDecls(req.Tools),
			},
		}
	}

	raw, err := a.doJSON(ctx, payload, req.Headers, model)
	if err != nil {
		return orchestrator.Response{}, err
	}

	var out struct {
		Candidates []struct {
			FinishReason string `json:"finishReason"`
			Content      struct {
				Parts []struct {
					Text         string `json:"text"`
					FunctionCall struct {
						Name string         `json:"name"`
						Args map[string]any `json:"args"`
					} `json:"functionCall"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return orchestrator.Response{}, fmt.Errorf("gemini adapter decode failed: %w", err)
	}
	if len(out.Candidates) == 0 {
		return orchestrator.Response{}, fmt.Errorf("gemini adapter returned empty candidates")
	}
	c := out.Candidates[0]

	blocks := make([]orchestrator.AssistantBlock, 0, len(c.Content.Parts))
	for _, part := range c.Content.Parts {
		if strings.TrimSpace(part.Text) != "" {
			blocks = append(blocks, orchestrator.AssistantBlock{
				Type: "text",
				Text: part.Text,
			})
		}
		if strings.TrimSpace(part.FunctionCall.Name) != "" {
			blocks = append(blocks, orchestrator.AssistantBlock{
				Type:  "tool_use",
				ID:    "toolu_gemini",
				Name:  part.FunctionCall.Name,
				Input: part.FunctionCall.Args,
			})
		}
	}
	if len(blocks) == 0 {
		blocks = append(blocks, orchestrator.AssistantBlock{Type: "text", Text: ""})
	}
	stop := normalizeGeminiStopReason(c.FinishReason, hasToolUse(blocks))
	return orchestrator.Response{
		Model:      req.Model,
		Blocks:     blocks,
		StopReason: stop,
		Usage: orchestrator.Usage{
			InputTokens:  out.UsageMetadata.PromptTokenCount,
			OutputTokens: out.UsageMetadata.CandidatesTokenCount,
		},
	}, nil
}

func (a *HTTPAdapter) completeCanonical(ctx context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	payload := map[string]any{
		"model":      req.Model,
		"max_tokens": req.MaxTokens,
		"messages":   req.Messages,
		"system":     req.System,
		"tools":      req.Tools,
		"metadata":   req.Metadata,
	}
	raw, err := a.doJSON(ctx, payload, req.Headers, req.Model)
	if err != nil {
		return orchestrator.Response{}, err
	}
	var out struct {
		Blocks     []orchestrator.AssistantBlock `json:"blocks"`
		StopReason string                        `json:"stop_reason"`
		Usage      orchestrator.Usage            `json:"usage"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return orchestrator.Response{}, fmt.Errorf("canonical adapter decode failed: %w", err)
	}
	if len(out.Blocks) == 0 {
		out.Blocks = []orchestrator.AssistantBlock{{Type: "text", Text: ""}}
	}
	if strings.TrimSpace(out.StopReason) == "" {
		out.StopReason = "end_turn"
	}
	return orchestrator.Response{
		Model:      req.Model,
		Blocks:     out.Blocks,
		StopReason: out.StopReason,
		Usage:      out.Usage,
	}, nil
}

func (a *HTTPAdapter) streamAnthropic(ctx context.Context, req orchestrator.Request, out chan<- orchestrator.StreamEvent) error {
	model := req.Model
	if a.model != "" {
		model = a.model
	}

	payload := map[string]any{
		"model":      model,
		"max_tokens": req.MaxTokens,
		"messages":   canonicalToAnthropicMessages(req.Messages),
		"stream":     true,
	}
	if req.System != nil {
		payload["system"] = req.System
	}
	if len(req.Tools) > 0 {
		payload["tools"] = canonicalToAnthropicTools(req.Tools)
	}
	if v, ok := req.Metadata["temperature"]; ok {
		payload["temperature"] = v
	}
	if v, ok := req.Metadata["top_p"]; ok {
		payload["top_p"] = v
	}

	httpReq, err := a.newJSONRequest(ctx, payload, req.Headers, model)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		return fmt.Errorf("adapter %s upstream status %d: %s", a.name, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return readSSE(resp.Body, func(eventName string, data []byte) error {
		if len(data) == 0 {
			return nil
		}
		if strings.TrimSpace(string(data)) == "[DONE]" {
			return nil
		}

		if strings.TrimSpace(eventName) == "" {
			var fallback struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(data, &fallback); err == nil && strings.TrimSpace(fallback.Type) != "" {
				eventName = fallback.Type
			}
		}
		if strings.TrimSpace(eventName) == "" {
			return nil
		}

		raw := append([]byte(nil), data...)
		out <- orchestrator.StreamEvent{
			Type:        eventName,
			RawEvent:    eventName,
			RawData:     raw,
			PassThrough: true,
		}
		return nil
	})
}

func (a *HTTPAdapter) doJSON(ctx context.Context, payload any, reqHeaders map[string]string, upstreamModel string) ([]byte, error) {
	httpReq, err := a.newJSONRequest(ctx, payload, reqHeaders, upstreamModel)
	if err != nil {
		return nil, err
	}
	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("adapter %s upstream status %d: %s", a.name, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func (a *HTTPAdapter) doOpenAIStream(ctx context.Context, payload any, reqHeaders map[string]string, upstreamModel string) (openAIStreamAggregate, error) {
	httpReq, err := a.newJSONRequest(ctx, payload, reqHeaders, upstreamModel)
	if err != nil {
		return openAIStreamAggregate{}, err
	}
	resp, err := a.client.Do(httpReq)
	if err != nil {
		return openAIStreamAggregate{}, err
	}
	defer resp.Body.Close()

	ctype := strings.ToLower(strings.TrimSpace(resp.Header.Get("content-type")))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		return openAIStreamAggregate{}, fmt.Errorf("adapter %s upstream status %d: %s", a.name, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	// Some upstreams may ignore stream=true and return JSON directly.
	if !strings.Contains(ctype, "text/event-stream") {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if err != nil {
			return openAIStreamAggregate{}, err
		}
		parsed, err := parseOpenAIJSONResponse(body)
		if err != nil {
			return openAIStreamAggregate{}, err
		}
		return openAIStreamAggregate{
			Content:      parsed.Content,
			FinishReason: parsed.FinishReason,
			ToolCalls:    parsed.ToolCalls,
			Usage: orchestrator.Usage{
				InputTokens:  parsed.PromptTokens,
				OutputTokens: parsed.CompletionTokens,
			},
		}, nil
	}

	agg := openAIStreamAggregate{}
	toolByIndex := map[int]*openAIToolCallPartial{}
	seen := false

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}
		seen = true

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Usage.PromptTokens > 0 {
			agg.Usage.InputTokens = chunk.Usage.PromptTokens
		}
		if chunk.Usage.CompletionTokens > 0 {
			agg.Usage.OutputTokens = chunk.Usage.CompletionTokens
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				agg.Content += choice.Delta.Content
			}
			if strings.TrimSpace(choice.FinishReason) != "" {
				agg.FinishReason = choice.FinishReason
			}
			for _, tc := range choice.Delta.ToolCalls {
				p := toolByIndex[tc.Index]
				if p == nil {
					p = &openAIToolCallPartial{}
					toolByIndex[tc.Index] = p
				}
				if tc.ID != "" {
					p.ID = tc.ID
				}
				if tc.Function.Name != "" {
					p.Name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					p.Arguments += tc.Function.Arguments
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return openAIStreamAggregate{}, err
	}
	if !seen {
		return openAIStreamAggregate{}, fmt.Errorf("adapter %s returned empty stream", a.name)
	}
	agg.ToolCalls = orderedToolCalls(toolByIndex)
	return agg, nil
}

func (a *HTTPAdapter) newJSONRequest(ctx context.Context, payload any, reqHeaders map[string]string, upstreamModel string) (*http.Request, error) {
	rawBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	endpoint := a.endpoint
	if upstreamModel != "" && strings.Contains(endpoint, "{model}") {
		endpoint = strings.ReplaceAll(endpoint, "{model}", url.PathEscape(upstreamModel))
	}
	url := a.baseURL + endpoint

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(rawBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("content-type", "application/json")
	if a.userAgent != "" {
		httpReq.Header.Set("user-agent", a.userAgent)
	}

	// Headers from adapter config override defaults.
	for k, v := range a.headers {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		httpReq.Header.Set(k, v)
	}
	if a.apiKey != "" && a.apiKeyHeader != "" && httpReq.Header.Get(a.apiKeyHeader) == "" {
		httpReq.Header.Set(a.apiKeyHeader, a.apiKey)
	}

	switch a.kind {
	case AdapterKindOpenAI:
		if a.apiKey != "" && httpReq.Header.Get("authorization") == "" {
			httpReq.Header.Set("authorization", "Bearer "+a.apiKey)
		}
	case AdapterKindAnthropic:
		if a.apiKey != "" && httpReq.Header.Get("x-api-key") == "" {
			httpReq.Header.Set("x-api-key", a.apiKey)
		}
		version := reqHeaders["anthropic-version"]
		if strings.TrimSpace(version) == "" {
			version = "2023-06-01"
		}
		httpReq.Header.Set("anthropic-version", version)
		if beta := reqHeaders["anthropic-beta"]; strings.TrimSpace(beta) != "" {
			httpReq.Header.Set("anthropic-beta", beta)
		}
	case AdapterKindGemini:
		if a.apiKey != "" && httpReq.Header.Get("x-goog-api-key") == "" && a.apiKeyHeader == "" {
			httpReq.Header.Set("x-goog-api-key", a.apiKey)
		}
	}
	return httpReq, nil
}

func emitResponseAsStream(events chan<- orchestrator.StreamEvent, resp orchestrator.Response) {
	events <- orchestrator.StreamEvent{Type: "message_start"}
	for i, b := range resp.Blocks {
		events <- orchestrator.StreamEvent{Type: "content_block_start", Index: i, Block: b}
		switch b.Type {
		case "text":
			for _, c := range splitTextDeltas(b.Text, 24) {
				if c == "" {
					continue
				}
				events <- orchestrator.StreamEvent{
					Type:      "content_block_delta",
					Index:     i,
					DeltaText: c,
				}
			}
		case "tool_use":
			raw, _ := json.Marshal(b.Input)
			events <- orchestrator.StreamEvent{
				Type:      "content_block_delta",
				Index:     i,
				DeltaJSON: string(raw),
			}
		}
		events <- orchestrator.StreamEvent{Type: "content_block_stop", Index: i}
	}
	events <- orchestrator.StreamEvent{
		Type:       "message_delta",
		StopReason: resp.StopReason,
		Usage:      resp.Usage,
	}
	events <- orchestrator.StreamEvent{Type: "message_stop"}
}

func readSSE(r io.Reader, onFrame func(event string, data []byte) error) error {
	reader := bufio.NewReader(r)
	var eventName string
	var dataLines []string

	flush := func() error {
		if len(dataLines) == 0 {
			eventName = ""
			return nil
		}
		payload := strings.Join(dataLines, "\n")
		err := onFrame(eventName, []byte(payload))
		eventName = ""
		dataLines = nil
		return err
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			if fErr := flush(); fErr != nil {
				return fErr
			}
		} else if strings.HasPrefix(line, ":") {
			// comment line
		} else if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(line[len("event:"):])
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(line[len("data:"):]))
		}

		if err == io.EOF {
			if fErr := flush(); fErr != nil {
				return fErr
			}
			return nil
		}
	}
}

func splitTextDeltas(text string, n int) []string {
	if n <= 0 {
		return []string{text}
	}
	var out []string
	for len(text) > n {
		out = append(out, text[:n])
		text = text[n:]
	}
	if text != "" {
		out = append(out, text)
	}
	return out
}

type openAIToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type openAIParsed struct {
	Content          string
	ToolCalls        []openAIToolCall
	FinishReason     string
	PromptTokens     int
	CompletionTokens int
}

type openAIToolCallPartial struct {
	ID        string
	Name      string
	Arguments string
}

type openAIStreamAggregate struct {
	Content      string
	ToolCalls    []openAIToolCall
	FinishReason string
	Usage        orchestrator.Usage
}

type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func parseOpenAIJSONResponse(raw []byte) (openAIParsed, error) {
	var out struct {
		Model   string `json:"model"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
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
	if err := json.Unmarshal(raw, &out); err != nil {
		return openAIParsed{}, fmt.Errorf("openai adapter decode failed: %w", err)
	}
	if len(out.Choices) == 0 {
		return openAIParsed{}, fmt.Errorf("openai adapter returned empty choices")
	}
	ch := out.Choices[0]
	toolCalls := make([]openAIToolCall, 0, len(ch.Message.ToolCalls))
	for _, tc := range ch.Message.ToolCalls {
		toolCalls = append(toolCalls, openAIToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return openAIParsed{
		Content:          ch.Message.Content,
		ToolCalls:        toolCalls,
		FinishReason:     ch.FinishReason,
		PromptTokens:     out.Usage.PromptTokens,
		CompletionTokens: out.Usage.CompletionTokens,
	}, nil
}

func openAIBlocksFromParsed(parsed openAIParsed) []orchestrator.AssistantBlock {
	blocks := make([]orchestrator.AssistantBlock, 0, 1+len(parsed.ToolCalls))
	if strings.TrimSpace(parsed.Content) != "" {
		blocks = append(blocks, orchestrator.AssistantBlock{
			Type: "text",
			Text: parsed.Content,
		})
	}
	for _, tc := range parsed.ToolCalls {
		input := map[string]any{}
		if strings.TrimSpace(tc.Arguments) != "" {
			_ = json.Unmarshal([]byte(tc.Arguments), &input)
		}
		blocks = append(blocks, orchestrator.AssistantBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Name,
			Input: input,
		})
	}
	if len(blocks) == 0 {
		blocks = append(blocks, orchestrator.AssistantBlock{Type: "text", Text: ""})
	}
	return blocks
}

func openAIBlocksFromAggregate(agg openAIStreamAggregate) []orchestrator.AssistantBlock {
	return openAIBlocksFromParsed(openAIParsed{
		Content:   agg.Content,
		ToolCalls: agg.ToolCalls,
	})
}

func orderedToolCalls(toolByIndex map[int]*openAIToolCallPartial) []openAIToolCall {
	if len(toolByIndex) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(toolByIndex))
	for idx := range toolByIndex {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)
	out := make([]openAIToolCall, 0, len(indexes))
	for _, idx := range indexes {
		p := toolByIndex[idx]
		out = append(out, openAIToolCall{
			ID:        p.ID,
			Name:      p.Name,
			Arguments: p.Arguments,
		})
	}
	return out
}

func mergeStreamOptions(base map[string]any, fromMeta any) map[string]any {
	out := copyAnyMap(base)
	metaMap, ok := fromMeta.(map[string]any)
	if !ok {
		return out
	}
	if out == nil {
		out = map[string]any{}
	}
	for k, v := range metaMap {
		if strings.TrimSpace(k) == "" {
			continue
		}
		out[k] = v
	}
	return out
}

func boolFromAny(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(strings.TrimSpace(x), "true")
	default:
		return false
	}
}

func copyHeaders(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
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
		out[k] = v
	}
	return out
}

func hasToolUse(blocks []orchestrator.AssistantBlock) bool {
	for _, b := range blocks {
		if b.Type == "tool_use" {
			return true
		}
	}
	return false
}

func normalizeOpenAIStopReason(finish string, hasToolCalls bool) string {
	switch strings.TrimSpace(finish) {
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	case "content_filter":
		return "stop_sequence"
	}
	if hasToolCalls {
		return "tool_use"
	}
	return "end_turn"
}

func normalizeGeminiStopReason(finish string, hasToolCalls bool) string {
	switch strings.TrimSpace(strings.ToUpper(finish)) {
	case "STOP":
		if hasToolCalls {
			return "tool_use"
		}
		return "end_turn"
	case "MAX_TOKENS":
		return "max_tokens"
	default:
		if hasToolCalls {
			return "tool_use"
		}
		return "end_turn"
	}
}

func canonicalToOpenAITools(tools []orchestrator.Tool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.InputSchema,
			},
		})
	}
	return out
}

func canonicalToAnthropicTools(tools []orchestrator.Tool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		out = append(out, map[string]any{
			"name":         t.Name,
			"description":  t.Description,
			"input_schema": t.InputSchema,
		})
	}
	return out
}

func canonicalToGeminiToolDecls(tools []orchestrator.Tool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		out = append(out, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"parameters":  t.InputSchema,
		})
	}
	return out
}

func canonicalToOpenAIMessages(system any, messages []orchestrator.Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages)+1)
	if sys := renderSystemToString(system); strings.TrimSpace(sys) != "" {
		out = append(out, map[string]any{
			"role":    "system",
			"content": sys,
		})
	}
	for _, m := range messages {
		role := m.Role
		switch c := m.Content.(type) {
		case string:
			out = append(out, map[string]any{
				"role":    role,
				"content": c,
			})
		case []any:
			textParts := make([]string, 0, len(c))
			for _, item := range c {
				block, ok := item.(map[string]any)
				if !ok {
					continue
				}
				blockType, _ := block["type"].(string)
				switch blockType {
				case "text":
					if text, ok := block["text"].(string); ok {
						textParts = append(textParts, text)
					}
				case "tool_result":
					toolCallID, _ := block["tool_use_id"].(string)
					content := fmt.Sprintf("%v", block["content"])
					out = append(out, map[string]any{
						"role":         "tool",
						"tool_call_id": toolCallID,
						"content":      content,
					})
				case "tool_use":
					toolID, _ := block["id"].(string)
					name, _ := block["name"].(string)
					args, _ := json.Marshal(block["input"])
					out = append(out, map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{
							{
								"id":   toolID,
								"type": "function",
								"function": map[string]any{
									"name":      name,
									"arguments": string(args),
								},
							},
						},
						"content": "",
					})
				}
			}
			if len(textParts) > 0 {
				out = append(out, map[string]any{
					"role":    role,
					"content": strings.Join(textParts, "\n"),
				})
			}
		default:
			out = append(out, map[string]any{
				"role":    role,
				"content": fmt.Sprintf("%v", c),
			})
		}
	}
	return out
}

func renderSystemToString(system any) string {
	switch s := system.(type) {
	case nil:
		return ""
	case string:
		return s
	case []any:
		var parts []string
		for _, item := range s {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if block["type"] == "text" {
				if text, ok := block["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return fmt.Sprintf("%v", s)
	}
}

func canonicalToAnthropicMessages(messages []orchestrator.Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		switch c := m.Content.(type) {
		case string:
			out = append(out, map[string]any{
				"role": m.Role,
				"content": []map[string]any{
					{
						"type": "text",
						"text": c,
					},
				},
			})
		case []any:
			out = append(out, map[string]any{
				"role":    m.Role,
				"content": c,
			})
		default:
			out = append(out, map[string]any{
				"role": m.Role,
				"content": []map[string]any{
					{
						"type": "text",
						"text": fmt.Sprintf("%v", c),
					},
				},
			})
		}
	}
	return out
}

func canonicalToGeminiContents(messages []orchestrator.Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		role := "user"
		switch strings.ToLower(strings.TrimSpace(m.Role)) {
		case "assistant":
			role = "model"
		case "user":
			role = "user"
		default:
			role = "user"
		}

		switch c := m.Content.(type) {
		case string:
			out = append(out, map[string]any{
				"role": role,
				"parts": []map[string]any{
					{"text": c},
				},
			})
		case []any:
			parts := make([]map[string]any, 0, len(c))
			for _, item := range c {
				block, ok := item.(map[string]any)
				if !ok {
					continue
				}
				blockType, _ := block["type"].(string)
				switch blockType {
				case "text":
					if text, ok := block["text"].(string); ok {
						parts = append(parts, map[string]any{"text": text})
					}
				case "tool_result":
					if content, ok := block["content"].(string); ok {
						parts = append(parts, map[string]any{"text": content})
					}
				}
			}
			if len(parts) == 0 {
				parts = append(parts, map[string]any{"text": ""})
			}
			out = append(out, map[string]any{
				"role":  role,
				"parts": parts,
			})
		default:
			out = append(out, map[string]any{
				"role": role,
				"parts": []map[string]any{
					{"text": fmt.Sprintf("%v", c)},
				},
			})
		}
	}
	return out
}
