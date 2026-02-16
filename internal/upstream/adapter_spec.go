package upstream

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type AdapterSpec struct {
	Name               string            `json:"name"`
	Kind               AdapterKind       `json:"kind"`
	SupportsVision     *bool             `json:"supports_vision,omitempty"`
	SupportsTools      *bool             `json:"supports_tools,omitempty"`
	BaseURL            string            `json:"base_url,omitempty"`
	Endpoint           string            `json:"endpoint,omitempty"`
	APIKey             string            `json:"api_key,omitempty"`
	APIKeyEnv          string            `json:"api_key_env,omitempty"`
	Headers            map[string]string `json:"headers,omitempty"`
	Model              string            `json:"model,omitempty"`
	UserAgent          string            `json:"user_agent,omitempty"`
	APIKeyHeader       string            `json:"api_key_header,omitempty"`
	ForceStream        bool              `json:"force_stream,omitempty"`
	StreamOptions      map[string]any    `json:"stream_options,omitempty"`
	InsecureSkipVerify bool              `json:"insecure_skip_verify,omitempty"`
	Command            string            `json:"command,omitempty"`
	Args               []string          `json:"args,omitempty"`
	Env                map[string]string `json:"env,omitempty"`
	WorkDir            string            `json:"work_dir,omitempty"`
	TimeoutMS          int               `json:"timeout_ms,omitempty"`
	MaxOutputBytes     int               `json:"max_output_bytes,omitempty"`
}

type UpstreamAdminConfig struct {
	Adapters     []AdapterSpec       `json:"adapters"`
	DefaultRoute []string            `json:"default_route,omitempty"`
	ModelRoutes  map[string][]string `json:"model_routes,omitempty"`
}

func ParseAdapterSpecsFromEnv() ([]AdapterSpec, error) {
	raw := strings.TrimSpace(os.Getenv("UPSTREAM_ADAPTERS_JSON"))
	if raw == "" {
		return nil, nil
	}
	var specs []AdapterSpec
	if err := json.Unmarshal([]byte(raw), &specs); err != nil {
		return nil, fmt.Errorf("invalid UPSTREAM_ADAPTERS_JSON: %w", err)
	}
	if len(specs) == 0 {
		return nil, nil
	}
	out := make([]AdapterSpec, 0, len(specs))
	for _, spec := range specs {
		out = append(out, sanitizeAdapterSpec(spec))
	}
	return out, nil
}

func BuildAdaptersFromSpecs(specs []AdapterSpec) ([]Adapter, error) {
	out := make([]Adapter, 0, len(specs))
	for _, spec := range specs {
		adapter, err := BuildAdapterFromSpec(spec)
		if err != nil {
			return nil, err
		}
		out = append(out, adapter)
	}
	return out, nil
}

func BuildAdapterFromSpec(spec AdapterSpec) (Adapter, error) {
	spec = sanitizeAdapterSpec(spec)
	switch spec.Kind {
	case AdapterKindScript:
		return NewScriptAdapter(ScriptAdapterConfig{
			Name:           spec.Name,
			Command:        spec.Command,
			Args:           append([]string(nil), spec.Args...),
			Env:            copyHeaders(spec.Env),
			WorkDir:        spec.WorkDir,
			Model:          spec.Model,
			SupportsVision: cloneBoolPtr(spec.SupportsVision),
			SupportsTools:  cloneBoolPtr(spec.SupportsTools),
			TimeoutMS:      spec.TimeoutMS,
			MaxOutputBytes: spec.MaxOutputBytes,
		})
	case AdapterKindOpenAI, AdapterKindAnthropic, AdapterKindGemini, AdapterKindCanonical:
		apiKey := strings.TrimSpace(spec.APIKey)
		if apiKey == "" && strings.TrimSpace(spec.APIKeyEnv) != "" {
			apiKey = strings.TrimSpace(os.Getenv(spec.APIKeyEnv))
		}
		return NewHTTPAdapter(HTTPAdapterConfig{
			Name:               spec.Name,
			Kind:               spec.Kind,
			BaseURL:            spec.BaseURL,
			Endpoint:           spec.Endpoint,
			APIKey:             apiKey,
			Headers:            copyHeaders(spec.Headers),
			Model:              spec.Model,
			UserAgent:          spec.UserAgent,
			APIKeyHeader:       spec.APIKeyHeader,
			SupportsVision:     cloneBoolPtr(spec.SupportsVision),
			SupportsTools:      cloneBoolPtr(spec.SupportsTools),
			ForceStream:        spec.ForceStream,
			StreamOptions:      copyAnyMap(spec.StreamOptions),
			InsecureSkipVerify: spec.InsecureSkipVerify,
		}, nil)
	default:
		return nil, fmt.Errorf("unsupported adapter kind %q", spec.Kind)
	}
}

func sanitizeAdapterSpec(in AdapterSpec) AdapterSpec {
	out := in
	out.Name = strings.TrimSpace(in.Name)
	out.Kind = AdapterKind(strings.TrimSpace(string(in.Kind)))
	out.SupportsVision = cloneBoolPtr(in.SupportsVision)
	out.SupportsTools = cloneBoolPtr(in.SupportsTools)
	out.BaseURL = strings.TrimSpace(in.BaseURL)
	out.Endpoint = strings.TrimSpace(in.Endpoint)
	out.APIKey = strings.TrimSpace(in.APIKey)
	out.APIKeyEnv = strings.TrimSpace(in.APIKeyEnv)
	out.Headers = copyHeaders(in.Headers)
	out.Model = strings.TrimSpace(in.Model)
	out.UserAgent = strings.TrimSpace(in.UserAgent)
	out.APIKeyHeader = strings.TrimSpace(in.APIKeyHeader)
	out.StreamOptions = copyAnyMap(in.StreamOptions)
	out.Command = strings.TrimSpace(in.Command)
	out.Args = append([]string(nil), in.Args...)
	out.Env = copyHeaders(in.Env)
	out.WorkDir = strings.TrimSpace(in.WorkDir)
	return out
}

func cleanRoute(route []string) []string {
	out := make([]string, 0, len(route))
	for _, item := range route {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func cleanModelRoutes(routes map[string][]string) map[string][]string {
	if len(routes) == 0 {
		return map[string][]string{}
	}
	out := make(map[string][]string, len(routes))
	for model, route := range routes {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		out[model] = cleanRoute(route)
	}
	return out
}

func cloneBoolPtr(in *bool) *bool {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}
