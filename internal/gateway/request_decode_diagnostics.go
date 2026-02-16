package gateway

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"ccgateway/internal/ccevent"
	"ccgateway/internal/runlog"
)

var unknownFieldPattern = regexp.MustCompile(`json: unknown field "([^"]+)"`)

type requestDecodeIssue struct {
	Reason            string
	UnsupportedFields []string
	RequestBody       string
	CurlCommand       string
}

func (s *server) reportRequestDecodeIssue(r *http.Request, err error) requestDecodeIssue {
	issue := buildRequestDecodeIssue(r, err)
	if s == nil {
		return issue
	}

	eventType := "request.decode_failed"
	if len(issue.UnsupportedFields) > 0 {
		eventType = "request.unsupported_fields"
	}

	data := map[string]any{
		"reason":             issue.Reason,
		"method":             requestMethod(r),
		"path":               requestPathWithQuery(r),
		"unsupported_fields": issue.UnsupportedFields,
		"unsupported_count":  len(issue.UnsupportedFields),
		"error":              strings.TrimSpace(errorString(err)),
		"request_body":       issue.RequestBody,
		"curl_command":       issue.CurlCommand,
		"record_text":        buildRequestDecodeRecordText(r, issue),
	}
	if query := requestQueryMap(r); len(query) > 0 {
		data["query"] = query
	}

	if s.eventStore != nil {
		s.appendEvent(ccevent.AppendInput{
			EventType: eventType,
			SessionID: requestSessionID(r, nil),
			Data:      data,
		})
	}
	s.logRun(runlog.Entry{
		Path:        requestPathWithQuery(r),
		Reason:      issue.Reason,
		Mode:        "request_decode",
		Stream:      false,
		ToolCount:   0,
		Status:      http.StatusBadRequest,
		Error:       strings.TrimSpace(errorString(err)),
		RecordText:  buildRequestDecodeRecordText(r, issue),
		Unsupported: append([]string(nil), issue.UnsupportedFields...),
		RequestBody: issue.RequestBody,
		CurlCommand: issue.CurlCommand,
	})
	return issue
}

func buildRequestDecodeIssue(r *http.Request, err error) requestDecodeIssue {
	rawBody := truncateText(strings.TrimSpace(extractDecodeErrorBody(err)), 4000)
	fields := extractUnknownFields(err)
	issue := requestDecodeIssue{
		Reason:            classifyDecodeFailureReason(err, fields),
		UnsupportedFields: fields,
		RequestBody:       rawBody,
	}
	issue.CurlCommand = buildReproCurlCommand(r, rawBody)
	return issue
}

func buildRequestDecodeRecordText(r *http.Request, issue requestDecodeIssue) string {
	parts := []string{
		"request.decode_failed",
		"path=" + requestPathWithQuery(r),
		"reason=" + strings.TrimSpace(issue.Reason),
	}
	if method := requestMethod(r); method != "" {
		parts = append(parts, "method="+method)
	}
	if len(issue.UnsupportedFields) > 0 {
		parts = append(parts, "unsupported="+strings.Join(issue.UnsupportedFields, ","))
	}
	if issue.CurlCommand != "" {
		parts = append(parts, fmt.Sprintf(`curl="%s"`, truncateText(issue.CurlCommand, 260)))
	}
	return strings.Join(parts, " | ")
}

func classifyDecodeFailureReason(err error, unsupported []string) string {
	if len(unsupported) > 0 {
		return "unsupported_fields"
	}
	switch {
	case errors.Is(err, io.EOF):
		return "empty_body"
	case errors.Is(err, errExtraJSONValues):
		return "trailing_json"
	case strings.Contains(strings.ToLower(errorString(err)), "invalid trailing json"):
		return "trailing_json"
	default:
		return "invalid_json"
	}
}

func extractUnknownFields(err error) []string {
	text := errorString(err)
	if text == "" {
		return nil
	}
	matches := unknownFieldPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		field := strings.TrimSpace(match[1])
		if field == "" {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	sort.Strings(out)
	return out
}

func extractDecodeErrorBody(err error) string {
	var decodeErr *jsonBodyDecodeError
	if errors.As(err, &decodeErr) {
		return decodeErr.body
	}
	return ""
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

func requestPathWithQuery(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	path := strings.TrimSpace(r.URL.Path)
	if path == "" {
		path = "/"
	}
	if rawQuery := strings.TrimSpace(r.URL.RawQuery); rawQuery != "" {
		return path + "?" + rawQuery
	}
	return path
}

func requestMethod(r *http.Request) string {
	if r == nil {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(r.Method))
}

func requestQueryMap(r *http.Request) map[string]any {
	if r == nil || r.URL == nil {
		return nil
	}
	values := r.URL.Query()
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, list := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if len(list) == 1 {
			out[key] = strings.TrimSpace(list[0])
			continue
		}
		trimmed := make([]string, 0, len(list))
		for _, item := range list {
			trimmed = append(trimmed, strings.TrimSpace(item))
		}
		out[key] = trimmed
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func buildReproCurlCommand(r *http.Request, body string) string {
	if r == nil || r.URL == nil {
		return ""
	}
	targetURL := buildRequestURLForCurl(r)
	if targetURL == "" {
		return ""
	}
	method := strings.ToUpper(strings.TrimSpace(r.Method))
	if method == "" {
		method = http.MethodPost
	}

	parts := []string{"curl", "-X", shellSingleQuote(method), shellSingleQuote(targetURL)}

	headers := []string{}
	if ct := strings.TrimSpace(r.Header.Get("content-type")); ct != "" {
		headers = append(headers, "Content-Type: "+ct)
	}
	if av := strings.TrimSpace(r.Header.Get("anthropic-version")); av != "" {
		headers = append(headers, "anthropic-version: "+av)
	}
	if auth := strings.TrimSpace(r.Header.Get("authorization")); auth != "" {
		headers = append(headers, "Authorization: [REDACTED]")
	}
	if admin := strings.TrimSpace(r.Header.Get("x-admin-token")); admin != "" {
		headers = append(headers, "x-admin-token: [REDACTED]")
	}
	if key := strings.TrimSpace(r.Header.Get("x-api-key")); key != "" {
		headers = append(headers, "x-api-key: [REDACTED]")
	}
	sort.Strings(headers)
	for _, header := range headers {
		parts = append(parts, "-H", shellSingleQuote(header))
	}

	if strings.TrimSpace(body) != "" {
		parts = append(parts, "--data-raw", shellSingleQuote(body))
	}

	return strings.Join(parts, " ")
}

func buildRequestURLForCurl(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := strings.ToLower(strings.TrimSpace(r.Header.Get("x-forwarded-proto"))); proto == "http" || proto == "https" {
		scheme = proto
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		host = "127.0.0.1:8080"
	}
	return scheme + "://" + host + requestPathWithQuery(r)
}

func shellSingleQuote(text string) string {
	if text == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(text, "'", `'"'"'`) + "'"
}
