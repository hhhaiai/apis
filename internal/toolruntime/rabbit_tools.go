package toolruntime

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type rabbitConfig struct {
	APIBase  string
	VHost    string
	Username string
	Password string
	Timeout  time.Duration
}

func handleRabbitPublish(ctx context.Context, call Call) (Result, error) {
	cfg, err := rabbitConfigFromInput(call.Input)
	if err != nil {
		return Result{}, err
	}
	routingKey := firstString(call.Input, "routing_key", "queue")
	if routingKey == "" {
		return Result{}, fmt.Errorf("rabbit_publish requires routing_key")
	}
	exchange := firstString(call.Input, "exchange")
	if exchange == "" {
		exchange = "amq.default"
	}
	payloadRaw, ok := call.Input["payload"]
	if !ok {
		payloadRaw = firstString(call.Input, "content", "text")
	}
	payload, encoding := rabbitPayload(payloadRaw)
	props := map[string]any{}
	if rawProps, ok := call.Input["properties"].(map[string]any); ok {
		props = rawProps
	}

	var out map[string]any
	err = rabbitDoJSON(ctx, cfg, http.MethodPost,
		"exchanges/"+rabbitPathEscape(cfg.VHost)+"/"+rabbitPathEscape(exchange)+"/publish",
		map[string]any{
			"properties":       props,
			"routing_key":      routingKey,
			"payload":          payload,
			"payload_encoding": encoding,
		},
		&out,
	)
	if err != nil {
		return Result{IsError: true, Content: err.Error()}, nil
	}
	return Result{
		Content: map[string]any{
			"tool":        call.Name,
			"exchange":    exchange,
			"routing_key": routingKey,
			"routed":      out["routed"],
		},
	}, nil
}

func handleRabbitGet(ctx context.Context, call Call) (Result, error) {
	cfg, err := rabbitConfigFromInput(call.Input)
	if err != nil {
		return Result{}, err
	}
	queue := firstString(call.Input, "queue")
	if queue == "" {
		return Result{}, fmt.Errorf("rabbit_get requires queue")
	}
	count := intFromAny(call.Input["count"], 1)
	if count <= 0 {
		count = 1
	}
	if count > 50 {
		count = 50
	}
	ackMode := firstString(call.Input, "ackmode")
	if ackMode == "" {
		ackMode = "ack_requeue_false"
	}
	encoding := firstString(call.Input, "encoding")
	if encoding == "" {
		encoding = "auto"
	}
	truncate := intFromAny(call.Input["truncate"], 65536)
	if truncate <= 0 {
		truncate = 65536
	}

	messages, err := rabbitQueueGet(ctx, cfg, queue, count, ackMode, encoding, truncate)
	if err != nil {
		return Result{IsError: true, Content: err.Error()}, nil
	}
	return Result{
		Content: map[string]any{
			"tool":     call.Name,
			"queue":    queue,
			"count":    len(messages),
			"messages": messages,
		},
	}, nil
}

func handleRabbitRPC(ctx context.Context, call Call) (Result, error) {
	cfg, err := rabbitConfigFromInput(call.Input)
	if err != nil {
		return Result{}, err
	}
	requestQueue := firstString(call.Input, "request_queue", "queue")
	if requestQueue == "" {
		return Result{}, fmt.Errorf("rabbit_rpc requires request_queue")
	}

	replyQueue := firstString(call.Input, "reply_queue")
	ephemeralReplyQueue := false
	if replyQueue == "" {
		replyQueue = "ccgateway.reply." + randomHex(8)
		ephemeralReplyQueue = true
	}
	if ephemeralReplyQueue {
		if err := rabbitCreateQueue(ctx, cfg, replyQueue); err != nil {
			return Result{IsError: true, Content: err.Error()}, nil
		}
		defer rabbitDeleteQueue(context.Background(), cfg, replyQueue)
	}

	exchange := firstString(call.Input, "exchange")
	if exchange == "" {
		exchange = "amq.default"
	}
	correlationID := firstString(call.Input, "correlation_id")
	if correlationID == "" {
		correlationID = "ccg-" + randomHex(12)
	}
	payloadRaw, ok := call.Input["payload"]
	if !ok {
		payloadRaw = firstString(call.Input, "content", "text")
	}
	payload, encoding := rabbitPayload(payloadRaw)

	props := map[string]any{
		"reply_to":       replyQueue,
		"correlation_id": correlationID,
	}
	if userProps, ok := call.Input["properties"].(map[string]any); ok {
		for k, v := range userProps {
			props[k] = v
		}
	}

	var publishOut map[string]any
	if err := rabbitDoJSON(ctx, cfg, http.MethodPost,
		"exchanges/"+rabbitPathEscape(cfg.VHost)+"/"+rabbitPathEscape(exchange)+"/publish",
		map[string]any{
			"properties":       props,
			"routing_key":      requestQueue,
			"payload":          payload,
			"payload_encoding": encoding,
		},
		&publishOut,
	); err != nil {
		return Result{IsError: true, Content: err.Error()}, nil
	}

	timeoutMS := intFromAny(call.Input["timeout_ms"], 10000)
	if timeoutMS <= 0 {
		timeoutMS = 10000
	}
	pollMS := intFromAny(call.Input["poll_interval_ms"], 200)
	if pollMS <= 0 {
		pollMS = 200
	}
	deadline := time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
	var response map[string]any
	for {
		if time.Now().After(deadline) {
			return Result{IsError: true, Content: "rabbit_rpc timeout waiting for response"}, nil
		}
		msgs, err := rabbitQueueGet(ctx, cfg, replyQueue, 1, "ack_requeue_false", "auto", 65536)
		if err == nil && len(msgs) > 0 {
			response = msgs[0]
			break
		}
		select {
		case <-ctx.Done():
			return Result{IsError: true, Content: "rabbit_rpc cancelled: " + ctx.Err().Error()}, nil
		case <-time.After(time.Duration(pollMS) * time.Millisecond):
		}
	}

	return Result{
		Content: map[string]any{
			"tool":           call.Name,
			"request_queue":  requestQueue,
			"reply_queue":    replyQueue,
			"correlation_id": correlationID,
			"routed":         publishOut["routed"],
			"response":       response,
		},
	}, nil
}

func rabbitQueueGet(ctx context.Context, cfg rabbitConfig, queue string, count int, ackMode, encoding string, truncate int) ([]map[string]any, error) {
	var messages []map[string]any
	err := rabbitDoJSON(ctx, cfg, http.MethodPost,
		"queues/"+rabbitPathEscape(cfg.VHost)+"/"+rabbitPathEscape(queue)+"/get",
		map[string]any{
			"count":    count,
			"ackmode":  ackMode,
			"encoding": encoding,
			"truncate": truncate,
		},
		&messages,
	)
	if err != nil {
		return nil, err
	}
	return messages, nil
}

func rabbitCreateQueue(ctx context.Context, cfg rabbitConfig, queue string) error {
	return rabbitDoJSON(ctx, cfg, http.MethodPut,
		"queues/"+rabbitPathEscape(cfg.VHost)+"/"+rabbitPathEscape(queue),
		map[string]any{
			"durable":     false,
			"auto_delete": true,
			"arguments":   map[string]any{},
		},
		nil,
	)
}

func rabbitDeleteQueue(ctx context.Context, cfg rabbitConfig, queue string) {
	_ = rabbitDoJSON(ctx, cfg, http.MethodDelete,
		"queues/"+rabbitPathEscape(cfg.VHost)+"/"+rabbitPathEscape(queue),
		nil,
		nil,
	)
}

func rabbitConfigFromInput(input map[string]any) (rabbitConfig, error) {
	apiBase := firstString(input, "management_url", "api_url", "rabbit_url")
	if apiBase == "" {
		apiBase = strings.TrimSpace(os.Getenv("RABBIT_HTTP_API"))
	}
	if apiBase == "" {
		apiBase = "http://127.0.0.1:15672/api"
	}
	apiBase = strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if apiBase == "" {
		return rabbitConfig{}, fmt.Errorf("rabbit management_url is required")
	}
	if !strings.HasSuffix(apiBase, "/api") {
		apiBase += "/api"
	}
	if _, err := url.ParseRequestURI(apiBase); err != nil {
		return rabbitConfig{}, fmt.Errorf("invalid rabbit management_url: %w", err)
	}
	vhost := firstString(input, "vhost")
	if vhost == "" {
		vhost = strings.TrimSpace(os.Getenv("RABBIT_VHOST"))
	}
	if vhost == "" {
		vhost = "/"
	}
	username := firstString(input, "username", "user")
	if username == "" {
		username = strings.TrimSpace(os.Getenv("RABBIT_USERNAME"))
	}
	if username == "" {
		username = "guest"
	}
	password := firstString(input, "password", "pass")
	if password == "" {
		password = strings.TrimSpace(os.Getenv("RABBIT_PASSWORD"))
	}
	if password == "" {
		password = "guest"
	}
	timeoutMS := intFromAny(input["timeout_ms"], 5000)
	if timeoutMS <= 0 {
		timeoutMS = 5000
	}

	return rabbitConfig{
		APIBase:  apiBase,
		VHost:    vhost,
		Username: username,
		Password: password,
		Timeout:  time.Duration(timeoutMS) * time.Millisecond,
	}, nil
}

func rabbitPathEscape(raw string) string {
	escaped := url.PathEscape(strings.TrimSpace(raw))
	escaped = strings.ReplaceAll(escaped, "/", "%2F")
	return escaped
}

func rabbitPayload(payload any) (string, string) {
	switch v := payload.(type) {
	case nil:
		return "", "string"
	case string:
		return v, "string"
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", payload), "string"
		}
		return string(raw), "string"
	}
}

func rabbitDoJSON(ctx context.Context, cfg rabbitConfig, method, endpoint string, reqBody any, out any) error {
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	fullURL := cfg.APIBase + "/" + strings.TrimLeft(endpoint, "/")
	var bodyReader io.Reader
	if reqBody != nil {
		raw, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal rabbit request body: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return err
	}
	req.SetBasicAuth(cfg.Username, cfg.Password)
	req.Header.Set("content-type", "application/json")

	client := &http.Client{Timeout: cfg.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	rawResp, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("rabbit api status %d: %s", resp.StatusCode, strings.TrimSpace(string(rawResp)))
	}
	if out == nil || len(rawResp) == 0 {
		return nil
	}
	if err := json.Unmarshal(rawResp, out); err != nil {
		return fmt.Errorf("decode rabbit response: %w", err)
	}
	return nil
}

func intFromAny(v any, fallback int) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		x = strings.TrimSpace(x)
		if x == "" {
			return fallback
		}
		var out int
		if _, err := fmt.Sscanf(x, "%d", &out); err == nil {
			return out
		}
	}
	return fallback
}

func randomHex(n int) string {
	if n <= 0 {
		n = 8
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
