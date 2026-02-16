package requestctx

import (
	"context"
	"strings"
)

type projectContextKey struct{}

const DefaultProjectID = "default"

// NormalizeProjectID normalizes project id to a safe, stable token.
func NormalizeProjectID(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return DefaultProjectID
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		}
		if b.Len() >= 64 {
			break
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return DefaultProjectID
	}
	return out
}

func WithProjectID(ctx context.Context, projectID string) context.Context {
	return context.WithValue(ctx, projectContextKey{}, NormalizeProjectID(projectID))
}

func ProjectID(ctx context.Context) string {
	if ctx == nil {
		return DefaultProjectID
	}
	if v, ok := ctx.Value(projectContextKey{}).(string); ok {
		return NormalizeProjectID(v)
	}
	return DefaultProjectID
}
