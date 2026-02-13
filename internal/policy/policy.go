package policy

import (
	"context"
	"errors"
	"strings"

	"ccgateway/internal/settings"
	"ccgateway/internal/toolcatalog"
)

type Engine interface {
	Authorize(ctx context.Context, action Action) error
}

type Action struct {
	Path      string
	Model     string
	Mode      string
	ToolNames []string
}

type NoopEngine struct{}

func NewNoopEngine() *NoopEngine {
	return &NoopEngine{}
}

func (e *NoopEngine) Authorize(_ context.Context, action Action) error {
	for _, t := range action.ToolNames {
		if strings.EqualFold(strings.TrimSpace(t), "forbidden_tool") {
			return errors.New("tool forbidden by policy")
		}
	}
	return nil
}

type DynamicEngine struct {
	settings *settings.Store
	catalog  *toolcatalog.Catalog
}

func NewDynamicEngine(settingsStore *settings.Store, catalog *toolcatalog.Catalog) *DynamicEngine {
	return &DynamicEngine{
		settings: settingsStore,
		catalog:  catalog,
	}
}

func (e *DynamicEngine) Authorize(_ context.Context, action Action) error {
	for _, t := range action.ToolNames {
		if strings.EqualFold(strings.TrimSpace(t), "forbidden_tool") {
			return errors.New("tool forbidden by policy")
		}
	}

	if e.catalog == nil {
		return nil
	}
	allowExperimental := false
	allowUnknown := true
	if e.settings != nil {
		cfg := e.settings.Get()
		allowExperimental = cfg.AllowExperimentalTools
		allowUnknown = cfg.AllowUnknownTools
	}
	for _, t := range action.ToolNames {
		if err := e.catalog.CheckAllowed(t, allowExperimental, allowUnknown); err != nil {
			return err
		}
	}
	return nil
}
