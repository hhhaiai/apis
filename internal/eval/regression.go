package eval

import (
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"
)

// Baseline stores historical benchmark results for regression comparison.
type Baseline struct {
	Model     string             `json:"model"`
	Scores    map[string]float64 `json:"scores"` // case_id -> score
	AvgScore  float64            `json:"avg_score"`
	Timestamp time.Time          `json:"timestamp"`
}

// RegressionResult reports regression status for a test case.
type RegressionResult struct {
	CaseID        string  `json:"case_id"`
	BaselineScore float64 `json:"baseline_score"`
	CurrentScore  float64 `json:"current_score"`
	Delta         float64 `json:"delta"`
	Regressed     bool    `json:"regressed"`
}

// RegressionReport is the full regression analysis.
type RegressionReport struct {
	Model        string             `json:"model"`
	Results      []RegressionResult `json:"results"`
	Regressions  int                `json:"regressions"`
	Improvements int                `json:"improvements"`
	BaselineAvg  float64            `json:"baseline_avg"`
	CurrentAvg   float64            `json:"current_avg"`
	OverallDelta float64            `json:"overall_delta"`
	Timestamp    time.Time          `json:"timestamp"`
}

// RegressionRunner manages baselines and runs regression checks.
type RegressionRunner struct {
	mu        sync.RWMutex
	baselines map[string]Baseline // model -> baseline
	threshold float64             // score drop threshold to flag as regression
}

// NewRegressionRunner creates a new regression runner.
func NewRegressionRunner(threshold float64) *RegressionRunner {
	if threshold <= 0 {
		threshold = 1.0 // default: flag if any criterion drops >1 point
	}
	return &RegressionRunner{
		baselines: make(map[string]Baseline),
		threshold: threshold,
	}
}

// SaveBaseline stores benchmark results as the baseline for a model.
func (r *RegressionRunner) SaveBaseline(report BenchmarkReport) {
	r.mu.Lock()
	defer r.mu.Unlock()

	scores := make(map[string]float64, len(report.Results))
	for _, br := range report.Results {
		scores[br.CaseID] = br.Score
	}
	r.baselines[report.Model] = Baseline{
		Model:     report.Model,
		Scores:    scores,
		AvgScore:  report.AvgScore,
		Timestamp: time.Now().UTC(),
	}
}

// LoadBaseline retrieves the baseline for a model.
func (r *RegressionRunner) LoadBaseline(model string) (Baseline, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.baselines[model]
	return b, ok
}

// CheckRegression compares current benchmark results against the stored baseline.
func (r *RegressionRunner) CheckRegression(report BenchmarkReport) (RegressionReport, error) {
	r.mu.RLock()
	baseline, ok := r.baselines[report.Model]
	r.mu.RUnlock()

	if !ok {
		return RegressionReport{}, fmt.Errorf("no baseline found for model %q; run SaveBaseline first", report.Model)
	}

	rr := RegressionReport{
		Model:        report.Model,
		BaselineAvg:  baseline.AvgScore,
		CurrentAvg:   report.AvgScore,
		OverallDelta: report.AvgScore - baseline.AvgScore,
		Timestamp:    time.Now().UTC(),
	}

	for _, br := range report.Results {
		baseScore, ok := baseline.Scores[br.CaseID]
		if !ok {
			continue // new test case, skip
		}
		delta := br.Score - baseScore
		regressed := delta < -r.threshold
		rr.Results = append(rr.Results, RegressionResult{
			CaseID:        br.CaseID,
			BaselineScore: baseScore,
			CurrentScore:  br.Score,
			Delta:         math.Round(delta*10) / 10,
			Regressed:     regressed,
		})
		if regressed {
			rr.Regressions++
		}
		if delta > r.threshold {
			rr.Improvements++
		}
	}

	return rr, nil
}

// ExportBaseline returns baseline as JSON bytes.
func (r *RegressionRunner) ExportBaseline(model string) ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.baselines[model]
	if !ok {
		return nil, fmt.Errorf("no baseline for model %q", model)
	}
	return json.Marshal(b)
}

// ImportBaseline loads a baseline from JSON bytes.
func (r *RegressionRunner) ImportBaseline(data []byte) error {
	var b Baseline
	if err := json.Unmarshal(data, &b); err != nil {
		return fmt.Errorf("invalid baseline data: %w", err)
	}
	if b.Model == "" {
		return fmt.Errorf("baseline must have a model name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.baselines[b.Model] = b
	return nil
}
