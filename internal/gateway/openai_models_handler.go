package gateway

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"ccgateway/internal/upstream"
)

func (s *server) handleOpenAIModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	models := s.openAIModelIDs()
	resp := OpenAIModelsResponse{
		Object: "list",
		Data:   make([]OpenAIModel, 0, len(models)),
	}
	for _, model := range models {
		resp.Data = append(resp.Data, OpenAIModel{
			ID:      model,
			Object:  "model",
			Created: 0,
			OwnedBy: "cc-gateway",
		})
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *server) handleOpenAIModelByPath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	model := strings.TrimPrefix(r.URL.Path, "/v1/models/")
	model = strings.TrimPrefix(model, "/openai/v1/models/")
	model = strings.Trim(model, "/")
	if model == "" {
		s.handleOpenAIModels(w, r)
		return
	}
	if decoded, err := url.PathUnescape(model); err == nil {
		model = decoded
	}

	resp := OpenAIModel{
		ID:      strings.TrimSpace(model),
		Object:  "model",
		Created: 0,
		OwnedBy: "cc-gateway",
	}
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *server) openAIModelIDs() []string {
	seen := map[string]bool{}
	var out []string
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" || model == "*" || seen[model] {
			return
		}
		seen[model] = true
		out = append(out, model)
	}

	if s.settings != nil {
		cfg := s.settings.Get()
		add(cfg.ModelMapFallback)
		for mode, model := range cfg.ModeModels {
			add(mode)
			add(model)
		}
		for from, to := range cfg.ModelMappings {
			add(from)
			add(to)
		}
		for model := range cfg.IntelligentDispatch.ModelPolicies {
			add(model)
		}
	}

	if s.channelStore != nil {
		for _, model := range s.channelStore.GetEnabledModels(defaultChannelGroup) {
			add(model)
		}
	}

	if routes, err := upstream.ParseRoutesFromEnv(); err == nil {
		for model := range routes {
			add(model)
		}
	}
	if specs, err := upstream.ParseAdapterSpecsFromEnv(); err == nil {
		for _, spec := range specs {
			add(spec.Model)
		}
	}

	if len(out) == 0 {
		add("default")
	}
	sort.Strings(out)
	return out
}
