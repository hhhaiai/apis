package gateway_test

import (
	. "ccgateway/internal/gateway"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ccgateway/internal/eval"
	"ccgateway/internal/modelmap"
	"ccgateway/internal/orchestrator"
	"ccgateway/internal/policy"
)

type stubEvalCompleter struct{}

func (stubEvalCompleter) CompleteSimple(_ context.Context, _, _, _ string) (string, error) {
	return `{"accuracy":8,"completeness":8,"reasoning":8,"code_quality":8,"instruction_following":8,"analysis":"ok"}`, nil
}

func TestCCEvalRejectUnknownFields(t *testing.T) {
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Evaluator:    eval.NewEvaluator(stubEvalCompleter{}, "judge-model"),
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/cc/eval", strings.NewReader(`{"model":"m","prompt":"p","unknown_field":1}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestCCEvalRejectTrailingJSON(t *testing.T) {
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Evaluator:    eval.NewEvaluator(stubEvalCompleter{}, "judge-model"),
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/cc/eval", strings.NewReader(`{"model":"m","prompt":"p"} {}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for trailing JSON, got %d; body=%s", rr.Code, rr.Body.String())
	}
}
