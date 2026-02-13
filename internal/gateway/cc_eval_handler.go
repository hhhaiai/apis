package gateway

import (
	"encoding/json"
	"net/http"

	"ccgateway/internal/eval"
)

func (s *server) handleCCEval(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}
	if s.evaluator == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "evaluator is not configured")
		return
	}

	var req struct {
		Model    string `json:"model"`
		Prompt   string `json:"prompt"`
		Response string `json:"response"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON: "+err.Error())
		return
	}
	if req.Prompt == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "prompt is required")
		return
	}

	var result eval.EvalResult
	var err error

	if req.Response != "" {
		// Evaluate a provided response
		result, err = s.evaluator.Evaluate(r.Context(), req.Model, req.Prompt, req.Response)
	} else {
		// Generate a response first, then evaluate
		var response string
		result, response, err = s.evaluator.EvaluateWithGeneration(r.Context(), req.Model, req.Prompt)
		if err == nil {
			// Include the generated response in the output
			w.Header().Set("content-type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result":   result,
				"response": response,
			})
			return
		}
	}

	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "api_error", "evaluation failed: "+err.Error())
		return
	}

	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"result": result,
	})
}
