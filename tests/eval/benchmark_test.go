package eval_test

import (
	. "ccgateway/internal/eval"
	"testing"
	"time"
)

func TestDefaultBenchmarkSuite(t *testing.T) {
	suite := DefaultBenchmarkSuite()
	if suite.Name == "" {
		t.Fatal("suite name is empty")
	}
	if len(suite.Cases) != 10 {
		t.Fatalf("expected 10 test cases, got %d", len(suite.Cases))
	}
	for _, tc := range suite.Cases {
		if tc.ID == "" || tc.Prompt == "" {
			t.Fatalf("test case missing id or prompt: %+v", tc)
		}
	}
}

func TestRegressionRunner_SaveAndLoad(t *testing.T) {
	rr := NewRegressionRunner(1.0)
	report := BenchmarkReport{
		Model:    "test-model",
		AvgScore: 7.5,
		Results: []BenchmarkResult{
			{CaseID: "test-1", Score: 8.0},
			{CaseID: "test-2", Score: 7.0},
		},
		Timestamp: time.Now(),
	}
	rr.SaveBaseline(report)

	b, ok := rr.LoadBaseline("test-model")
	if !ok {
		t.Fatal("baseline not found")
	}
	if b.Scores["test-1"] != 8.0 {
		t.Fatalf("expected 8.0, got %f", b.Scores["test-1"])
	}
}

func TestRegressionRunner_CheckRegression(t *testing.T) {
	rr := NewRegressionRunner(1.0)

	// Save baseline
	rr.SaveBaseline(BenchmarkReport{
		Model:    "test-model",
		AvgScore: 8.0,
		Results: []BenchmarkResult{
			{CaseID: "c1", Score: 9.0},
			{CaseID: "c2", Score: 7.0},
		},
	})

	// Current run with regression on c1
	current := BenchmarkReport{
		Model:    "test-model",
		AvgScore: 6.5,
		Results: []BenchmarkResult{
			{CaseID: "c1", Score: 6.0}, // dropped 3 points
			{CaseID: "c2", Score: 7.0}, // same
		},
	}

	report, err := rr.CheckRegression(current)
	if err != nil {
		t.Fatal(err)
	}
	if report.Regressions != 1 {
		t.Fatalf("expected 1 regression, got %d", report.Regressions)
	}
	if report.Results[0].Delta != -3.0 {
		t.Fatalf("expected delta -3.0, got %f", report.Results[0].Delta)
	}
}

func TestRegressionRunner_NoBaseline(t *testing.T) {
	rr := NewRegressionRunner(1.0)
	_, err := rr.CheckRegression(BenchmarkReport{Model: "unknown"})
	if err == nil {
		t.Fatal("expected error for missing baseline")
	}
}

func TestRegressionRunner_ExportImport(t *testing.T) {
	rr := NewRegressionRunner(1.0)
	rr.SaveBaseline(BenchmarkReport{
		Model:    "test-model",
		AvgScore: 7.0,
		Results:  []BenchmarkResult{{CaseID: "c1", Score: 7.0}},
	})

	data, err := rr.ExportBaseline("test-model")
	if err != nil {
		t.Fatal(err)
	}

	rr2 := NewRegressionRunner(1.0)
	err = rr2.ImportBaseline(data)
	if err != nil {
		t.Fatal(err)
	}
	b, ok := rr2.LoadBaseline("test-model")
	if !ok {
		t.Fatal("imported baseline not found")
	}
	if b.Scores["c1"] != 7.0 {
		t.Fatalf("expected 7.0, got %f", b.Scores["c1"])
	}
}
