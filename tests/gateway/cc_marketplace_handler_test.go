package gateway_test

import (
	. "ccgateway/internal/gateway"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ccgateway/internal/marketplace"
	"ccgateway/internal/modelmap"
	"ccgateway/internal/orchestrator"
	"ccgateway/internal/policy"
)

type marketplaceServiceStub struct {
	installCalls int
	lastConfig   map[string]string
}

func (m *marketplaceServiceStub) ListAvailable() ([]marketplace.PluginManifest, error) {
	return nil, nil
}

func (m *marketplaceServiceStub) GetManifest(name string) (marketplace.PluginManifest, error) {
	return marketplace.PluginManifest{Name: name, Version: "1.0.0"}, nil
}

func (m *marketplaceServiceStub) Search(string, []string) ([]marketplace.SearchResult, error) {
	return nil, nil
}

func (m *marketplaceServiceStub) Install(_ string, config map[string]string) error {
	m.installCalls++
	m.lastConfig = config
	return nil
}

func (m *marketplaceServiceStub) Uninstall(string) error {
	return nil
}

func (m *marketplaceServiceStub) Update(string) error {
	return nil
}

func (m *marketplaceServiceStub) CheckUpdates() ([]marketplace.UpdateInfo, error) {
	return nil, nil
}

func (m *marketplaceServiceStub) GetRecommendations() ([]marketplace.PluginManifest, error) {
	return nil, nil
}

func (m *marketplaceServiceStub) GetStats(string) (marketplace.PluginStats, bool) {
	return marketplace.PluginStats{}, false
}

func (m *marketplaceServiceStub) GetPopularPlugins(int) []marketplace.PluginStats {
	return nil
}

func TestCCMarketplaceInstallAllowsEmptyBody(t *testing.T) {
	svc := &marketplaceServiceStub{}
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator:       orchestrator.NewSimpleService(),
		Policy:             policy.NewNoopEngine(),
		ModelMapper:        modelmap.NewIdentityMapper(),
		MarketplaceService: svc,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/cc/marketplace/plugins/demo/install", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 for empty install body, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if svc.installCalls != 1 {
		t.Fatalf("expected one install call, got %d", svc.installCalls)
	}
}

func TestCCMarketplaceInstallRejectUnknownFields(t *testing.T) {
	svc := &marketplaceServiceStub{}
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator:       orchestrator.NewSimpleService(),
		Policy:             policy.NewNoopEngine(),
		ModelMapper:        modelmap.NewIdentityMapper(),
		MarketplaceService: svc,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/cc/marketplace/plugins/demo/install", strings.NewReader(`{"unknown_field":1}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestCCMarketplaceInstallRejectTrailingJSON(t *testing.T) {
	svc := &marketplaceServiceStub{}
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator:       orchestrator.NewSimpleService(),
		Policy:             policy.NewNoopEngine(),
		ModelMapper:        modelmap.NewIdentityMapper(),
		MarketplaceService: svc,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/cc/marketplace/plugins/demo/install", strings.NewReader(`{"config":{"k":"v"}} {}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for trailing JSON, got %d; body=%s", rr.Code, rr.Body.String())
	}
}
