package gateway_test

import (
	. "ccgateway/internal/gateway"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ccgateway/internal/modelmap"
	"ccgateway/internal/orchestrator"
	"ccgateway/internal/policy"
	"ccgateway/internal/skill"
)

func TestCCSkillsRejectUnknownFieldsOnCreate(t *testing.T) {
	engine := skill.NewEngine()
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		SkillEngine:  engine,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/cc/skills", strings.NewReader(`{"name":"echo","template":"{{text}}","unknown_field":1}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestCCSkillExecuteRejectTrailingJSON(t *testing.T) {
	engine := skill.NewEngine()
	if err := engine.Register(skill.Skill{
		Name:     "echo",
		Template: "{{text}}",
	}); err != nil {
		t.Fatalf("register skill: %v", err)
	}

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		SkillEngine:  engine,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/cc/skills/echo/execute", strings.NewReader(`{"parameters":{"text":"hello"}} {}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for trailing JSON, got %d; body=%s", rr.Code, rr.Body.String())
	}
}
