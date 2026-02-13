package upstream

import (
	"context"
	"errors"

	"ccgateway/internal/orchestrator"
)

// Adapter maps provider-specific request/response formats to canonical schema.
type Adapter interface {
	Name() string
	Complete(ctx context.Context, req orchestrator.Request) (orchestrator.Response, error)
}

type StreamingAdapter interface {
	Adapter
	Stream(ctx context.Context, req orchestrator.Request) (<-chan orchestrator.StreamEvent, <-chan error)
}

var ErrStrictPassthroughUnsupported = errors.New("strict anthropic passthrough unsupported")
