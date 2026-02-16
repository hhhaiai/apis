package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ccgateway/internal/marketplace"
	"ccgateway/internal/plugin"
	"ccgateway/internal/requestctx"
)

func (s *server) handleAdminMarketplaceCloudList(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}
	var req struct {
		URL string `json:"url"`
	}
	if err := decodeJSONBodyStrict(r, &req, false); err != nil {
		s.reportRequestDecodeIssue(r, err)
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}
	manifests, err := fetchCloudManifests(req.URL)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"url":   strings.TrimSpace(req.URL),
		"data":  manifests,
		"count": len(manifests),
	})
}

func (s *server) handleAdminMarketplaceCloudInstall(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}
	if s.pluginStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "plugin store is not configured")
		return
	}

	var req struct {
		URL       string   `json:"url"`
		Names     []string `json:"names"`
		Scope     string   `json:"scope,omitempty"`
		ProjectID string   `json:"project_id,omitempty"`
	}
	if err := decodeJSONBodyStrict(r, &req, false); err != nil {
		s.reportRequestDecodeIssue(r, err)
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}
	if len(req.Names) == 0 {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "names are required")
		return
	}
	manifests, err := fetchCloudManifests(req.URL)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	manifestByName := make(map[string]marketplace.PluginManifest, len(manifests))
	for _, item := range manifests {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		manifestByName[name] = item
	}

	scopeSel := resolveScopeSelection(r)
	if strings.TrimSpace(req.Scope) != "" {
		scopeRaw := strings.ToLower(strings.TrimSpace(req.Scope))
		if scopeRaw == scopeGlobal {
			scopeSel.Scope = scopeGlobal
			scopeSel.ProjectID = requestctx.DefaultProjectID
		} else {
			scopeSel.Scope = scopeProject
		}
	}
	if strings.TrimSpace(req.ProjectID) != "" {
		scopeSel.ProjectID = requestctx.NormalizeProjectID(req.ProjectID)
	}
	if scopeSel.Scope == scopeGlobal {
		scopeSel.ProjectID = requestctx.DefaultProjectID
	}

	type failedItem struct {
		Name  string `json:"name"`
		Error string `json:"error"`
	}
	installed := make([]string, 0, len(req.Names))
	failed := make([]failedItem, 0)
	for _, rawName := range req.Names {
		name := strings.TrimSpace(rawName)
		if name == "" {
			continue
		}
		manifest, ok := manifestByName[name]
		if !ok {
			failed = append(failed, failedItem{Name: name, Error: "plugin not found in cloud source"})
			continue
		}
		p := plugin.Plugin{
			Name:        pluginStorageName(scopeSel.ProjectID, manifest.Name),
			Version:     manifest.Version,
			Description: manifest.Description,
			Skills:      manifest.Skills,
			Hooks:       manifest.Hooks,
			MCPServers:  manifest.MCPServers,
			Enabled:     true,
		}
		if err := s.pluginStore.Install(p); err != nil {
			failed = append(failed, failedItem{Name: name, Error: strings.TrimSpace(err.Error())})
			continue
		}
		installed = append(installed, name)
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":         len(failed) == 0,
		"scope":      scopeSel.Scope,
		"project_id": scopeSel.ProjectID,
		"installed":  installed,
		"failures":   failed,
	})
}

func fetchCloudManifests(rawURL string) ([]marketplace.PluginManifest, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("url is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("url must use http or https")
	}
	if err := validateCloudManifestURL(parsed); err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("fetch cloud manifests failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cloud source returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read cloud response failed: %w", err)
	}
	var manifests []marketplace.PluginManifest
	if err := json.Unmarshal(body, &manifests); err != nil {
		return nil, fmt.Errorf("invalid cloud manifests JSON: %w", err)
	}
	validator := marketplace.NewValidator()
	out := make([]marketplace.PluginManifest, 0, len(manifests))
	for _, item := range manifests {
		if strings.TrimSpace(item.Name) == "" {
			continue
		}
		if err := validator.ValidateManifest(item); err != nil {
			return nil, fmt.Errorf("invalid cloud manifest %q: %w", item.Name, err)
		}
		out = append(out, item)
	}
	return out, nil
}

func validateCloudManifestURL(parsed *url.URL) error {
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return fmt.Errorf("url host is required")
	}
	if isLikelyLocalHostname(host) {
		return fmt.Errorf("private network hosts are not allowed")
	}
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicIP(ip) {
			return fmt.Errorf("private network hosts are not allowed")
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve cloud host failed: %w", err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("resolve cloud host returned no addresses")
	}
	for _, addr := range addrs {
		if !isPublicIP(addr.IP) {
			return fmt.Errorf("private network hosts are not allowed")
		}
	}
	return nil
}

func isLikelyLocalHostname(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "localhost" || h == "localhost.localdomain" {
		return true
	}
	return strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".internal")
}

func isPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}

	if ipv4 := ip.To4(); ipv4 != nil {
		// CGNAT 100.64.0.0/10
		if ipv4[0] == 100 && (ipv4[1]&0xc0) == 64 {
			return false
		}
		// Benchmark testing 198.18.0.0/15
		if ipv4[0] == 198 && (ipv4[1] == 18 || ipv4[1] == 19) {
			return false
		}
		// Documentation-only ranges
		if ipv4[0] == 192 && ipv4[1] == 0 && ipv4[2] == 2 {
			return false
		}
		if ipv4[0] == 198 && ipv4[1] == 51 && ipv4[2] == 100 {
			return false
		}
		if ipv4[0] == 203 && ipv4[1] == 0 && ipv4[2] == 113 {
			return false
		}
	}
	return true
}
