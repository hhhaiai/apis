package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"ccgateway/internal/ccevent"
	"ccgateway/internal/probe"
	"ccgateway/internal/scheduler"
	"ccgateway/internal/settings"
	"ccgateway/internal/toolcatalog"
	"ccgateway/internal/upstream"
)

// Request helper structs for intelligent dispatch
type modelPolicyReq struct {
	PreferredAdapter string `json:"preferred_adapter,omitempty"`
	ForceScheduler   bool   `json:"force_scheduler,omitempty"`
	ComplexityLevel  string `json:"complexity_level,omitempty"`
}

type complexityThresholdReq struct {
	LongContextChars   *int `json:"long_context_chars,omitempty"`
	ToolCountThreshold *int `json:"tool_count_threshold,omitempty"`
}

func (s *server) handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if s.settings == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "settings store is not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(s.settings.Get())
	case http.MethodPut:
		var req settings.RuntimeSettings
		if err := decodeJSONBodyStrict(r, &req, false); err != nil {
			s.reportRequestDecodeIssue(r, err)
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		s.settings.Put(req)

		// Propagate intelligent dispatch settings to dispatcher if available
		if dispUpd, ok := s.orchestrator.(interface {
			UpdateDispatchConfigFull(cfg upstream.DispatchConfig) error
		}); ok {
			_ = dispUpd.UpdateDispatchConfigFull(upstream.DispatchConfig{
				Enabled:             req.IntelligentDispatch.Enabled,
				FallbackToScheduler: req.IntelligentDispatch.FallbackToScheduler,
				MinScoreDifference:  req.IntelligentDispatch.MinScoreDifference,
				ReElectIntervalMS:   req.IntelligentDispatch.ReElectIntervalMS,
			})
		}

		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(s.settings.Get())
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleAdminModelMapping(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if s.settings == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "settings store is not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		cfg := s.settings.Get()
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model_mappings":     cfg.ModelMappings,
			"model_map_strict":   cfg.ModelMapStrict,
			"model_map_fallback": cfg.ModelMapFallback,
		})
	case http.MethodPut:
		var req struct {
			ModelMappings    map[string]string `json:"model_mappings"`
			ModelMapStrict   bool              `json:"model_map_strict"`
			ModelMapFallback string            `json:"model_map_fallback"`
		}
		if err := decodeJSONBodyStrict(r, &req, false); err != nil {
			s.reportRequestDecodeIssue(r, err)
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		cfg := s.settings.Get()
		cfg.ModelMappings = req.ModelMappings
		cfg.ModelMapStrict = req.ModelMapStrict
		cfg.ModelMapFallback = strings.TrimSpace(req.ModelMapFallback)
		s.settings.Put(cfg)

		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model_mappings":     cfg.ModelMappings,
			"model_map_strict":   cfg.ModelMapStrict,
			"model_map_fallback": cfg.ModelMapFallback,
		})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleAdminUpstream(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	upstreamAdmin, ok := s.orchestrator.(interface {
		GetUpstreamConfig() upstream.UpstreamAdminConfig
		UpdateUpstreamConfig(cfg upstream.UpstreamAdminConfig) (upstream.UpstreamAdminConfig, error)
	})
	if !ok {
		s.writeError(w, http.StatusNotImplemented, "api_error", "orchestrator does not support upstream admin config")
		return
	}

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(upstreamAdmin.GetUpstreamConfig())
	case http.MethodPut:
		var cfg upstream.UpstreamAdminConfig
		if err := decodeJSONBodyStrict(r, &cfg, false); err != nil {
			s.reportRequestDecodeIssue(r, err)
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		updated, err := upstreamAdmin.UpdateUpstreamConfig(cfg)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(updated)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleAdminCapabilities(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	if mode == "" {
		mode = "chat"
	}
	model := strings.TrimSpace(r.URL.Query().Get("model"))
	includeMCP := parseQueryBool(r.URL.Query().Get("include_mcp"))

	snapshot, err := s.buildAdminCapabilitiesSnapshot(r.Context(), mode, model, includeMCP)
	if err != nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", err.Error())
		return
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(snapshot)
}

func (s *server) handleAdminTools(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if s.toolCatalog == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "tool catalog is not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		scopeSel := resolveScopeSelection(r)
		tools := s.toolCatalog.Snapshot()
		if scoped, ok := s.toolCatalog.(interface {
			SnapshotForProject(projectID string) []toolcatalog.ToolSpec
		}); ok && scopeSel.Scope == scopeProject {
			tools = scoped.SnapshotForProject(scopeSel.ProjectID)
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"scope":      scopeSel.Scope,
			"project_id": scopeSel.ProjectID,
			"tools":      tools,
		})
	case http.MethodPut:
		var req struct {
			Tools []toolcatalog.ToolSpec `json:"tools"`
		}
		if err := decodeJSONBodyStrict(r, &req, false); err != nil {
			s.reportRequestDecodeIssue(r, err)
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		scopeSel := resolveScopeSelection(r)
		if scoped, ok := s.toolCatalog.(interface {
			ReplaceForProject(projectID string, tools []toolcatalog.ToolSpec)
			SnapshotForProject(projectID string) []toolcatalog.ToolSpec
		}); ok && scopeSel.Scope == scopeProject {
			scoped.ReplaceForProject(scopeSel.ProjectID, req.Tools)
			w.Header().Set("content-type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"scope":      scopeSel.Scope,
				"project_id": scopeSel.ProjectID,
				"tools":      scoped.SnapshotForProject(scopeSel.ProjectID),
			})
			return
		}
		s.toolCatalog.Replace(req.Tools)
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"scope":      scopeSel.Scope,
			"project_id": scopeSel.ProjectID,
			"tools":      s.toolCatalog.Snapshot(),
		})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleAdminToolGaps(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if s.eventStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "event store is not configured")
		return
	}
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	limit, ok := parseNonNegativeInt(r.URL.Query().Get("limit"))
	if !ok {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "limit must be an integer >= 0")
		return
	}
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	nameFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("name")))
	reasonFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("reason")))
	pathFilter := strings.TrimSpace(r.URL.Query().Get("path"))
	if pathFilter != "" {
		pathFilter = strings.ToLower(pathFilter)
	}

	events := s.eventStore.List(ccevent.ListFilter{
		EventType: "tool.gap_detected",
		Limit:     limit,
		SessionID: sessionID,
	})

	type aggregate struct {
		Name     string
		Reason   string
		Count    int
		LastSeen time.Time
		Paths    map[string]struct{}
	}
	byKey := map[string]*aggregate{}
	byTool := map[string]int{}
	byReason := map[string]int{}
	matched := 0

	for _, ev := range events {
		name := strings.ToLower(strings.TrimSpace(fmt.Sprint(ev.Data["name"])))
		reason := strings.ToLower(strings.TrimSpace(fmt.Sprint(ev.Data["reason"])))
		path := strings.ToLower(strings.TrimSpace(fmt.Sprint(ev.Data["path"])))
		if name == "" {
			name = "(unknown)"
		}
		if reason == "" {
			reason = "(unknown)"
		}
		if nameFilter != "" && nameFilter != name {
			continue
		}
		if reasonFilter != "" && reasonFilter != reason {
			continue
		}
		if pathFilter != "" && pathFilter != path {
			continue
		}

		matched++
		byTool[name]++
		byReason[reason]++

		key := name + "|" + reason
		item := byKey[key]
		if item == nil {
			item = &aggregate{
				Name:   name,
				Reason: reason,
				Paths:  map[string]struct{}{},
			}
			byKey[key] = item
		}
		item.Count++
		if ev.CreatedAt.After(item.LastSeen) {
			item.LastSeen = ev.CreatedAt
		}
		if path != "" {
			item.Paths[path] = struct{}{}
		}
	}

	type gapSummary struct {
		Name     string   `json:"name"`
		Reason   string   `json:"reason"`
		Count    int      `json:"count"`
		LastSeen string   `json:"last_seen,omitempty"`
		Paths    []string `json:"paths,omitempty"`
	}
	summary := make([]gapSummary, 0, len(byKey))
	for _, item := range byKey {
		paths := make([]string, 0, len(item.Paths))
		for p := range item.Paths {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		row := gapSummary{
			Name:   item.Name,
			Reason: item.Reason,
			Count:  item.Count,
			Paths:  paths,
		}
		if !item.LastSeen.IsZero() {
			row.LastSeen = item.LastSeen.UTC().Format(time.RFC3339)
		}
		summary = append(summary, row)
	}
	sort.Slice(summary, func(i, j int) bool {
		if summary[i].Count != summary[j].Count {
			return summary[i].Count > summary[j].Count
		}
		if summary[i].Name != summary[j].Name {
			return summary[i].Name < summary[j].Name
		}
		return summary[i].Reason < summary[j].Reason
	})

	resp := map[string]any{
		"event_type":    "tool.gap_detected",
		"scanned":       len(events),
		"matched":       matched,
		"by_tool":       byTool,
		"by_reason":     byReason,
		"gap_summaries": summary,
	}

	if parseQueryBool(r.URL.Query().Get("include_suggestions")) {
		aliases := map[string]string{}
		if s.settings != nil {
			cfg := s.settings.Get()
			for k, v := range cfg.ToolAliases {
				k = strings.ToLower(strings.TrimSpace(k))
				v = strings.ToLower(strings.TrimSpace(v))
				if k != "" && v != "" {
					aliases[k] = v
				}
			}
		}

		mcpTools := s.collectMCPToolNames(r.Context(), 128)
		mcpSet := map[string]struct{}{}
		for _, name := range mcpTools {
			mcpSet[name] = struct{}{}
		}

		replacements := map[string][]string{}
		unresolved := make([]string, 0, len(summary))
		for _, row := range summary {
			candidates := suggestToolCandidates(row.Name, aliases, mcpSet)
			if len(candidates) == 0 {
				unresolved = append(unresolved, row.Name)
				continue
			}
			replacements[row.Name] = candidates
		}
		sort.Strings(unresolved)
		resp["tool_aliases"] = aliases
		resp["mcp_tools"] = mcpTools
		resp["replacement_candidates"] = replacements
		resp["unresolved_tools"] = unresolved
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *server) handleAdminScheduler(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if s.schedulerStatus == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "scheduler status is not configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"scheduler": schedulerSnapshot(s.schedulerStatus),
		})
	case http.MethodPut:
		updater, ok := s.schedulerStatus.(interface {
			UpdateConfigPatch(patch scheduler.ConfigPatch) (scheduler.Config, error)
			AdminSnapshot() map[string]any
		})
		if !ok {
			s.writeError(w, http.StatusNotImplemented, "api_error", "scheduler update is not supported")
			return
		}
		var patch scheduler.ConfigPatch
		if err := decodeJSONBodyStrict(r, &patch, false); err != nil {
			s.reportRequestDecodeIssue(r, err)
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		if _, err := updater.UpdateConfigPatch(patch); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"scheduler": updater.AdminSnapshot(),
		})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleAdminProbe(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if s.probeStatus == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "probe status is not configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"probe": s.probeStatus.Snapshot(),
		})
	case http.MethodPut:
		updater, ok := s.probeStatus.(interface {
			UpdateConfigPatch(patch probe.ConfigPatch) (probe.Config, error)
			Snapshot() map[string]any
		})
		if !ok {
			s.writeError(w, http.StatusNotImplemented, "api_error", "probe update is not supported")
			return
		}
		var patch probe.ConfigPatch
		if err := decodeJSONBodyStrict(r, &patch, false); err != nil {
			s.reportRequestDecodeIssue(r, err)
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		if _, err := updater.UpdateConfigPatch(patch); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"probe": updater.Snapshot(),
		})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

// handleAdminIntelligentDispatch manages intelligent dispatch settings
func (s *server) handleAdminIntelligentDispatch(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if s.settings == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "settings store is not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Get current intelligent dispatch settings with detailed status
		cfg := s.settings.Get()

		// Try to get dispatcher status if available
		dispatchStatus := s.getDispatchStatus()

		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"settings": cfg.IntelligentDispatch,
			"status":   dispatchStatus,
		})
	case http.MethodPut:
		var req struct {
			Enabled              *bool                     `json:"enabled,omitempty"`
			MinScoreDifference   *float64                  `json:"min_score_difference,omitempty"`
			ReElectIntervalMS    *int64                    `json:"re_elect_interval_ms,omitempty"`
			FallbackToScheduler  *bool                     `json:"fallback_to_scheduler,omitempty"`
			ModelPolicies        map[string]modelPolicyReq `json:"model_policies,omitempty"`
			ComplexityThresholds *complexityThresholdReq   `json:"complexity_thresholds,omitempty"`
		}
		if err := decodeJSONBodyStrict(r, &req, false); err != nil {
			s.reportRequestDecodeIssue(r, err)
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		cfg := s.settings.Get()
		if req.Enabled != nil {
			cfg.IntelligentDispatch.Enabled = *req.Enabled
		}
		if req.MinScoreDifference != nil && *req.MinScoreDifference > 0 {
			cfg.IntelligentDispatch.MinScoreDifference = *req.MinScoreDifference
		}
		if req.ReElectIntervalMS != nil && *req.ReElectIntervalMS > 0 {
			cfg.IntelligentDispatch.ReElectIntervalMS = *req.ReElectIntervalMS
		}
		if req.FallbackToScheduler != nil {
			cfg.IntelligentDispatch.FallbackToScheduler = *req.FallbackToScheduler
		}
		if req.ModelPolicies != nil {
			cfg.IntelligentDispatch.ModelPolicies = make(map[string]settings.ModelDispatchPolicy)
			for k, v := range req.ModelPolicies {
				cfg.IntelligentDispatch.ModelPolicies[k] = settings.ModelDispatchPolicy{
					PreferredAdapter: v.PreferredAdapter,
					ForceScheduler:   v.ForceScheduler,
					ComplexityLevel:  v.ComplexityLevel,
				}
			}
		}
		if req.ComplexityThresholds != nil {
			if req.ComplexityThresholds.LongContextChars != nil && *req.ComplexityThresholds.LongContextChars > 0 {
				cfg.IntelligentDispatch.ComplexityThresholds.LongContextChars = *req.ComplexityThresholds.LongContextChars
			}
			if req.ComplexityThresholds.ToolCountThreshold != nil && *req.ComplexityThresholds.ToolCountThreshold > 0 {
				cfg.IntelligentDispatch.ComplexityThresholds.ToolCountThreshold = *req.ComplexityThresholds.ToolCountThreshold
			}
		}
		s.settings.Put(cfg)

		// Try to update dispatcher if available
		if dispUpd, ok := s.orchestrator.(interface {
			UpdateDispatchConfigFull(cfg upstream.DispatchConfig) error
		}); ok {
			_ = dispUpd.UpdateDispatchConfigFull(upstream.DispatchConfig{
				Enabled:             cfg.IntelligentDispatch.Enabled,
				FallbackToScheduler: cfg.IntelligentDispatch.FallbackToScheduler,
				MinScoreDifference:  cfg.IntelligentDispatch.MinScoreDifference,
				ReElectIntervalMS:   cfg.IntelligentDispatch.ReElectIntervalMS,
			})
		}

		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"settings": cfg.IntelligentDispatch,
		})
	case http.MethodPost:
		// Handle special actions: rerun, reset-stats
		action := strings.TrimSpace(r.URL.Query().Get("action"))

		switch action {
		case "rerun":
			// Trigger re-election
			s.triggerDispatchRerun(w)
		case "reset-stats":
			// Reset statistics
			s.resetDispatchStats(w)
		default:
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "unknown action")
		}
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

// triggerDispatchRerun triggers a manual re-election
func (s *server) triggerDispatchRerun(w http.ResponseWriter) {
	if s.orchestrator == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "orchestrator not available")
		return
	}

	if rerun, ok := s.orchestrator.(interface {
		TriggerDispatchRerun() error
	}); ok {
		if err := rerun.TriggerDispatchRerun(); err != nil {
			s.writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"message": "re-election triggered",
		})
		return
	}

	s.writeError(w, http.StatusNotImplemented, "api_error", "rerun not supported")
}

// resetDispatchStats resets dispatch statistics
func (s *server) resetDispatchStats(w http.ResponseWriter) {
	if s.orchestrator == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "orchestrator not available")
		return
	}

	if reset, ok := s.orchestrator.(interface {
		ResetDispatchStats() error
	}); ok {
		if err := reset.ResetDispatchStats(); err != nil {
			s.writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"message": "stats reset",
		})
		return
	}

	s.writeError(w, http.StatusNotImplemented, "api_error", "reset not supported")
}

// getDispatchStatus retrieves current dispatch status from orchestrator
func (s *server) getDispatchStatus() map[string]any {
	if s.orchestrator == nil {
		return map[string]any{"available": false}
	}

	// Try to get dispatch status via interface
	if dispStatus, ok := s.orchestrator.(interface {
		GetDispatchStatus() map[string]any
	}); ok {
		status := dispStatus.GetDispatchStatus()
		status["available"] = true
		return status
	}

	return map[string]any{
		"available": false,
		"reason":    "orchestrator does not support dispatch status",
	}
}

func (s *server) handleAdminAuthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}
	token := strings.TrimSpace(s.adminToken)
	authRequired := token != ""
	defaultTokenEnabled := token == DefaultAdminToken
	providedToken := adminTokenFromRequest(r)
	tokenProvided := providedToken != ""
	tokenValid := !authRequired || (tokenProvided && providedToken == token)

	resp := map[string]any{
		"auth_required":         authRequired,
		"default_token_enabled": defaultTokenEnabled,
		"token_provided":        tokenProvided,
		"token_valid":           tokenValid,
	}
	if defaultTokenEnabled {
		resp["default_token_warning"] = "default admin password is enabled; set ADMIN_TOKEN to a custom value"
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *server) authorizeAdmin(w http.ResponseWriter, r *http.Request) bool {
	if s.adminToken == "" {
		return true
	}

	token := adminTokenFromRequest(r)
	if token != s.adminToken {
		s.writeError(w, http.StatusUnauthorized, "authentication_error", "admin token is invalid")
		return false
	}
	return true
}

func adminTokenFromRequest(r *http.Request) string {
	token := strings.TrimSpace(r.Header.Get("x-admin-token"))
	if token != "" {
		return token
	}
	auth := strings.TrimSpace(r.Header.Get("authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return ""
}

func schedulerSnapshot(status gatewayStatusProvider) map[string]any {
	if status == nil {
		return nil
	}
	if ext, ok := status.(interface{ AdminSnapshot() map[string]any }); ok {
		return ext.AdminSnapshot()
	}
	return status.Snapshot()
}

func parseQueryBool(raw string) bool {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (s *server) collectMCPToolNames(ctx context.Context, limit int) []string {
	if s.mcpRegistry == nil {
		return nil
	}
	if limit <= 0 {
		limit = 128
	}
	servers := s.mcpRegistry.List(limit)
	if len(servers) == 0 {
		return nil
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	seen := map[string]struct{}{}
	projectID := projectIDFromContext(ctx)
	for _, srv := range servers {
		if !srv.Enabled {
			continue
		}
		if !mcpServerBelongsToProject(projectID, srv) {
			continue
		}
		tools, err := s.mcpRegistry.ListTools(timeoutCtx, srv.ID)
		if err != nil {
			continue
		}
		for _, tool := range tools {
			name := strings.ToLower(strings.TrimSpace(tool.Name))
			if name == "" {
				continue
			}
			seen[name] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func suggestToolCandidates(name string, aliases map[string]string, mcpTools map[string]struct{}) []string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return nil
	}
	out := []string{}
	if mapped := strings.TrimSpace(aliases[name]); mapped != "" {
		out = append(out, "alias:"+mapped)
	}
	if _, ok := mcpTools[name]; ok {
		out = append(out, "mcp:"+name)
	}
	if len(out) < 3 {
		fuzzy := make([]string, 0, 2)
		for candidate := range mcpTools {
			if candidate == name {
				continue
			}
			if strings.Contains(candidate, name) || strings.Contains(name, candidate) {
				fuzzy = append(fuzzy, "mcp:"+candidate)
			}
		}
		sort.Strings(fuzzy)
		for _, item := range fuzzy {
			if len(out) >= 3 {
				break
			}
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}

func (s *server) buildAdminCapabilitiesSnapshot(ctx context.Context, mode, model string, includeMCP bool) (map[string]any, error) {
	type upstreamConfigProvider interface {
		GetUpstreamConfig() upstream.UpstreamAdminConfig
	}
	provider, ok := s.orchestrator.(upstreamConfigProvider)
	if !ok {
		return nil, fmt.Errorf("orchestrator does not support upstream admin config")
	}

	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "chat"
	}
	model = strings.TrimSpace(model)

	upstreamCfg := provider.GetUpstreamConfig()
	modeRoute := []string(nil)
	if s.settings != nil {
		modeRoute = cleanRouteLocal(s.settings.ModeRoute(mode))
	}
	modelRoute, modelRouteSource := resolveRouteByModelWithSource(upstreamCfg, model)

	resolvedRoute := modelRoute
	routeSource := modelRouteSource
	if len(modeRoute) > 0 {
		resolvedRoute = modeRoute
		routeSource = "runtime.mode_routes:" + mode
	}

	specByName := make(map[string]upstream.AdapterSpec, len(upstreamCfg.Adapters))
	for _, spec := range upstreamCfg.Adapters {
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			continue
		}
		specByName[name] = spec
	}

	toolsSupported, toolsKnown, unknownToolAdapters, missingRouteAdapters := resolveRouteSupport(
		resolvedRoute,
		specByName,
		func(spec upstream.AdapterSpec) *bool { return spec.SupportsTools },
	)
	visionSupported, visionKnown, unknownVisionAdapters, missingRouteAdaptersVision := resolveRouteSupport(
		resolvedRoute,
		specByName,
		func(spec upstream.AdapterSpec) *bool { return spec.SupportsVision },
	)
	missingRouteAdapters = mergeStringSets(missingRouteAdapters, missingRouteAdaptersVision)
	visionSource := "route_capability"
	if !visionKnown && s.settings != nil {
		if hinted, ok := s.settings.ResolveVisionSupport(model); ok {
			visionSupported = hinted
			visionKnown = true
			visionSource = "runtime.vision_support_hints"
		}
	}
	toolsFallbackNeeded := toolsKnown && !toolsSupported
	visionFallbackNeeded := visionKnown && !visionSupported

	type adapterCapabilityRow struct {
		Name                string `json:"name"`
		Kind                string `json:"kind"`
		ModelHint           string `json:"model_hint,omitempty"`
		SupportsTools       *bool  `json:"supports_tools,omitempty"`
		SupportsToolsKnown  bool   `json:"supports_tools_known"`
		SupportsVision      *bool  `json:"supports_vision,omitempty"`
		SupportsVisionKnown bool   `json:"supports_vision_known"`
		OnResolvedRoute     bool   `json:"on_resolved_route"`
	}
	routeSet := map[string]struct{}{}
	for _, name := range resolvedRoute {
		name = strings.TrimSpace(name)
		if name != "" {
			routeSet[name] = struct{}{}
		}
	}
	rows := make([]adapterCapabilityRow, 0, len(specByName))
	for _, spec := range upstreamCfg.Adapters {
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			continue
		}
		_, onRoute := routeSet[name]
		rows = append(rows, adapterCapabilityRow{
			Name:                name,
			Kind:                strings.TrimSpace(string(spec.Kind)),
			ModelHint:           strings.TrimSpace(spec.Model),
			SupportsTools:       copyBoolPtrLocal(spec.SupportsTools),
			SupportsToolsKnown:  spec.SupportsTools != nil,
			SupportsVision:      copyBoolPtrLocal(spec.SupportsVision),
			SupportsVisionKnown: spec.SupportsVision != nil,
			OnResolvedRoute:     onRoute,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Name < rows[j].Name
	})

	runtimeSummary := map[string]any{}
	if s.settings != nil {
		cfg := s.settings.Get()
		runtimeSummary["tool_loop_mode"] = cfg.ToolLoop.Mode
		runtimeSummary["tool_emulation_mode"] = cfg.ToolLoop.EmulationMode
		runtimeSummary["tool_loop_max_steps"] = cfg.ToolLoop.MaxSteps
		runtimeSummary["tool_aliases_count"] = len(cfg.ToolAliases)
		runtimeSummary["vision_hints_count"] = len(cfg.VisionSupportHints)
		runtimeSummary["allow_unknown_tools"] = cfg.AllowUnknownTools
	}

	diagnostics := make([]string, 0, 8)
	if len(resolvedRoute) == 0 {
		diagnostics = append(diagnostics, "resolved route is empty; configure runtime routing.mode_routes or upstream default/model routes")
	}
	if len(missingRouteAdapters) > 0 {
		diagnostics = append(diagnostics, "resolved route references unknown adapters: "+strings.Join(missingRouteAdapters, ", "))
	}
	if len(unknownToolAdapters) > 0 {
		diagnostics = append(diagnostics, "adapters missing supports_tools declaration: "+strings.Join(unknownToolAdapters, ", "))
	}
	if len(unknownVisionAdapters) > 0 {
		diagnostics = append(diagnostics, "adapters missing supports_vision declaration: "+strings.Join(unknownVisionAdapters, ", "))
	}
	if !toolsKnown {
		diagnostics = append(diagnostics, "effective tools capability is unknown; set supports_tools for deterministic tool fallback")
	} else if toolsFallbackNeeded {
		diagnostics = append(diagnostics, "tool fallback is recommended for this route")
	}
	if !visionKnown {
		diagnostics = append(diagnostics, "effective vision capability is unknown; set supports_vision or runtime vision_support_hints")
	} else if visionFallbackNeeded {
		diagnostics = append(diagnostics, "vision fallback is expected for this route")
	}

	payload := map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"mode":         mode,
		"model":        model,
		"routes": map[string]any{
			"source":             routeSource,
			"resolved":           resolvedRoute,
			"mode_route":         modeRoute,
			"model_route":        modelRoute,
			"model_route_source": modelRouteSource,
			"default_route":      cleanRouteLocal(upstreamCfg.DefaultRoute),
		},
		"effective": map[string]any{
			"supports_tools":         toolsSupported,
			"supports_tools_known":   toolsKnown,
			"supports_vision":        visionSupported,
			"supports_vision_known":  visionKnown,
			"vision_source":          visionSource,
			"tool_fallback_needed":   toolsFallbackNeeded,
			"vision_fallback_needed": visionFallbackNeeded,
		},
		"diagnostics": diagnostics,
		"adapters":    rows,
		"runtime":     runtimeSummary,
		"overview": map[string]any{
			"adapter_count":        len(rows),
			"resolved_route_count": len(resolvedRoute),
			"unknown_route_count":  len(missingRouteAdapters),
		},
	}

	if includeMCP {
		mcpTools := s.collectMCPToolNames(ctx, 128)
		payload["mcp_tools"] = mcpTools
		payload["mcp_tool_count"] = len(mcpTools)
	}
	return payload, nil
}

func resolveRouteByModelWithSource(cfg upstream.UpstreamAdminConfig, model string) ([]string, string) {
	model = strings.TrimSpace(model)
	if model == "" {
		if route := cleanRouteLocal(cfg.DefaultRoute); len(route) > 0 {
			return route, "upstream.default_route"
		}
		return nil, "none"
	}
	routes := cfg.ModelRoutes
	if len(routes) == 0 {
		if route := cleanRouteLocal(cfg.DefaultRoute); len(route) > 0 {
			return route, "upstream.default_route"
		}
		return nil, "none"
	}
	if seq := cleanRouteLocal(routes[model]); len(seq) > 0 {
		return seq, "upstream.model_routes.exact:" + model
	}

	type patternRoute struct {
		pattern     string
		specificity int
		route       []string
	}
	patterns := make([]patternRoute, 0, len(routes))
	for p, r := range routes {
		p = strings.TrimSpace(p)
		if p == "" || p == "*" || !strings.Contains(p, "*") {
			continue
		}
		patterns = append(patterns, patternRoute{
			pattern:     p,
			specificity: len(strings.ReplaceAll(p, "*", "")),
			route:       cleanRouteLocal(r),
		})
	}
	sort.Slice(patterns, func(i, j int) bool {
		if patterns[i].specificity == patterns[j].specificity {
			return patterns[i].pattern < patterns[j].pattern
		}
		return patterns[i].specificity > patterns[j].specificity
	})
	for _, p := range patterns {
		if len(p.route) == 0 {
			continue
		}
		matched, err := path.Match(p.pattern, model)
		if err != nil || !matched {
			continue
		}
		return p.route, "upstream.model_routes.pattern:" + p.pattern
	}
	if seq := cleanRouteLocal(routes["*"]); len(seq) > 0 {
		return seq, "upstream.model_routes.wildcard:*"
	}
	if route := cleanRouteLocal(cfg.DefaultRoute); len(route) > 0 {
		return route, "upstream.default_route"
	}
	return nil, "none"
}

func resolveRouteSupport(
	route []string,
	specByName map[string]upstream.AdapterSpec,
	pick func(upstream.AdapterSpec) *bool,
) (supported bool, known bool, unknownAdapters []string, missingAdapters []string) {
	unknownSet := map[string]struct{}{}
	missingSet := map[string]struct{}{}
	anyKnown := false
	anySupported := false
	anyUnsupported := false
	for _, name := range route {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		spec, ok := specByName[name]
		if !ok {
			missingSet[name] = struct{}{}
			continue
		}
		v := pick(spec)
		if v == nil {
			unknownSet[name] = struct{}{}
			continue
		}
		anyKnown = true
		if *v {
			anySupported = true
		} else {
			anyUnsupported = true
		}
	}
	if anySupported {
		supported = true
		known = true
	} else if anyKnown && anyUnsupported {
		supported = false
		known = true
	}

	unknownAdapters = make([]string, 0, len(unknownSet))
	for name := range unknownSet {
		unknownAdapters = append(unknownAdapters, name)
	}
	sort.Strings(unknownAdapters)

	missingAdapters = make([]string, 0, len(missingSet))
	for name := range missingSet {
		missingAdapters = append(missingAdapters, name)
	}
	sort.Strings(missingAdapters)
	return supported, known, unknownAdapters, missingAdapters
}

func mergeStringSets(a, b []string) []string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	for _, item := range a {
		item = strings.TrimSpace(item)
		if item != "" {
			seen[item] = struct{}{}
		}
	}
	for _, item := range b {
		item = strings.TrimSpace(item)
		if item != "" {
			seen[item] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for item := range seen {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func copyBoolPtrLocal(v *bool) *bool {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

type gatewayStatusProvider interface {
	Snapshot() map[string]any
}
