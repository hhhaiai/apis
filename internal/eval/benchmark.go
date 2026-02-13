package eval

import (
	"context"
	"fmt"
	"time"
)

// TestCase represents a single evaluation test case.
type TestCase struct {
	ID               string  `json:"id"`
	Category         string  `json:"category"` // bug_fix, refactor, test_write, pr_review, code_gen
	Prompt           string  `json:"prompt"`
	ExpectedBehavior string  `json:"expected_behavior"`
	MinScore         float64 `json:"min_score"` // minimum acceptable overall score
}

// BenchmarkSuite is a collection of test cases.
type BenchmarkSuite struct {
	Name    string     `json:"name"`
	Cases   []TestCase `json:"cases"`
	Version string     `json:"version"`
}

// BenchmarkResult holds the result of running one test case.
type BenchmarkResult struct {
	CaseID   string             `json:"case_id"`
	Category string             `json:"category"`
	Score    float64            `json:"score"`
	Criteria map[string]float64 `json:"criteria"`
	Pass     bool               `json:"pass"`
	Duration int64              `json:"duration_ms"`
}

// BenchmarkReport holds the full benchmark run results.
type BenchmarkReport struct {
	Suite     string            `json:"suite"`
	Model     string            `json:"model"`
	Results   []BenchmarkResult `json:"results"`
	AvgScore  float64           `json:"avg_score"`
	PassRate  float64           `json:"pass_rate"`
	TotalTime int64             `json:"total_time_ms"`
	Timestamp time.Time         `json:"timestamp"`
}

// DefaultBenchmarkSuite returns a built-in suite with 10 standard test cases.
func DefaultBenchmarkSuite() BenchmarkSuite {
	return BenchmarkSuite{
		Name:    "cc-standard-v1",
		Version: "1.0",
		Cases: []TestCase{
			{
				ID: "bug-fix-1", Category: "bug_fix", MinScore: 6,
				Prompt:           "Find and fix the bug in this Go code that causes an index out of range panic:\n\nfunc getLastItem(items []string) string {\n    return items[len(items)]\n}",
				ExpectedBehavior: "Should identify off-by-one error and fix to items[len(items)-1] with nil/empty check",
			},
			{
				ID: "bug-fix-2", Category: "bug_fix", MinScore: 6,
				Prompt:           "Fix the race condition in this Go code:\n\nvar counter int\nfunc increment() { counter++ }\n// called from multiple goroutines",
				ExpectedBehavior: "Should use sync.Mutex or atomic operations to fix the race condition",
			},
			{
				ID: "refactor-1", Category: "refactor", MinScore: 5,
				Prompt:           "Refactor this function to improve readability and reduce complexity:\n\nfunc process(x int) string {\n    if x > 0 { if x > 10 { if x > 100 { return \"huge\" } else { return \"big\" } } else { return \"small\" } } else { return \"negative\" }\n}",
				ExpectedBehavior: "Should flatten nested ifs using early returns or switch/case",
			},
			{
				ID: "test-write-1", Category: "test_write", MinScore: 5,
				Prompt:           "Write comprehensive unit tests for this function:\n\nfunc Divide(a, b float64) (float64, error) {\n    if b == 0 { return 0, fmt.Errorf(\"division by zero\") }\n    return a / b, nil\n}",
				ExpectedBehavior: "Should test normal division, division by zero, negative numbers, large numbers",
			},
			{
				ID: "test-write-2", Category: "test_write", MinScore: 5,
				Prompt:           "Write table-driven tests for a function ParseAge(s string) (int, error) that parses a string age, rejecting negatives and non-numbers.",
				ExpectedBehavior: "Should use table-driven pattern with subtests, cover valid, invalid, edge cases",
			},
			{
				ID: "pr-review-1", Category: "pr_review", MinScore: 5,
				Prompt:           "Review this code change and provide feedback:\n\n-func GetUser(id string) (*User, error) {\n+func GetUser(id string) *User {\n     u, err := db.Find(id)\n-    if err != nil { return nil, err }\n-    return u, nil\n+    if err != nil { return nil }\n+    return u\n }",
				ExpectedBehavior: "Should flag loss of error information, suggest keeping error return",
			},
			{
				ID: "code-gen-1", Category: "code_gen", MinScore: 5,
				Prompt:           "Write a Go function that validates an email address. It should check for @ symbol, domain with dot, no spaces, and minimum lengths.",
				ExpectedBehavior: "Should implement basic email validation with clear error messages",
			},
			{
				ID: "code-gen-2", Category: "code_gen", MinScore: 5,
				Prompt:           "Implement a thread-safe LRU cache in Go with Get, Put, and a configurable maximum size.",
				ExpectedBehavior: "Should use doubly-linked list + map, mutex for thread safety, evict least recently used",
			},
			{
				ID: "reasoning-1", Category: "reasoning", MinScore: 5,
				Prompt:           "Explain the trade-offs between using channels vs mutexes for synchronization in Go. Give concrete examples of when each is more appropriate.",
				ExpectedBehavior: "Should discuss communication vs shared state, blocking, complexity, performance",
			},
			{
				ID: "reasoning-2", Category: "reasoning", MinScore: 5,
				Prompt:           "A REST API is responding slowly under load. The p99 latency is 5 seconds. List the most likely causes in order of probability and how you would diagnose each.",
				ExpectedBehavior: "Should mention database queries, connection pooling, memory, GC, N+1 queries, caching",
			},
		},
	}
}

// RunBenchmark runs the full benchmark suite against a model.
func RunBenchmark(ctx context.Context, evaluator *Evaluator, model string, suite BenchmarkSuite) (BenchmarkReport, error) {
	if evaluator == nil {
		return BenchmarkReport{}, fmt.Errorf("evaluator is required")
	}

	report := BenchmarkReport{
		Suite:     suite.Name,
		Model:     model,
		Timestamp: time.Now().UTC(),
	}

	start := time.Now()
	totalScore := 0.0
	passed := 0

	for _, tc := range suite.Cases {
		select {
		case <-ctx.Done():
			return report, ctx.Err()
		default:
		}

		caseStart := time.Now()
		result, _, err := evaluator.EvaluateWithGeneration(ctx, model, tc.Prompt)

		br := BenchmarkResult{
			CaseID:   tc.ID,
			Category: tc.Category,
			Duration: time.Since(caseStart).Milliseconds(),
		}

		if err != nil {
			br.Score = 0
			br.Pass = false
			br.Criteria = map[string]float64{}
		} else {
			br.Score = result.Score
			br.Criteria = result.Criteria
			br.Pass = result.Score >= tc.MinScore
		}

		totalScore += br.Score
		if br.Pass {
			passed++
		}
		report.Results = append(report.Results, br)
	}

	n := len(suite.Cases)
	if n > 0 {
		report.AvgScore = totalScore / float64(n)
		report.PassRate = float64(passed) / float64(n)
	}
	report.TotalTime = time.Since(start).Milliseconds()

	return report, nil
}
