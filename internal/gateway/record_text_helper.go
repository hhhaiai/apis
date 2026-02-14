package gateway

import (
	"fmt"
	"strings"
)

func valueAsString(v any) string {
	if v == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(v))
	if text == "<nil>" {
		return ""
	}
	return text
}

func normalizeSpaces(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return strings.Join(strings.Fields(text), " ")
}

func truncateText(text string, limit int) string {
	text = strings.TrimSpace(text)
	if text == "" || limit <= 0 {
		return ""
	}
	rs := []rune(text)
	if len(rs) <= limit {
		return text
	}
	return strings.TrimSpace(string(rs[:limit])) + "..."
}
