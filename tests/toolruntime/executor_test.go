package toolruntime_test

import (
	. "ccgateway/internal/toolruntime"
	"context"
	"testing"
)

func TestDefaultExecutorGetWeather(t *testing.T) {
	ex := NewDefaultExecutor()
	out, err := ex.Execute(context.Background(), Call{
		Name: "get_weather",
		Input: map[string]any{
			"city": "Hangzhou",
		},
	})
	if err != nil {
		t.Fatalf("execute get_weather: %v", err)
	}
	content, ok := out.Content.(map[string]any)
	if !ok {
		t.Fatalf("expected map content, got %T", out.Content)
	}
	if content["city"] != "Hangzhou" {
		t.Fatalf("unexpected city in result: %#v", content["city"])
	}
}

func TestDefaultExecutorUnknownTool(t *testing.T) {
	ex := NewDefaultExecutor()
	_, err := ex.Execute(context.Background(), Call{Name: "unknown_tool"})
	if err == nil {
		t.Fatalf("expected unknown tool error")
	}
}

func TestDefaultExecutorWebSearchRequiresQuery(t *testing.T) {
	ex := NewDefaultExecutor()
	if _, err := ex.Execute(context.Background(), Call{Name: "web_search"}); err == nil {
		t.Fatalf("expected web_search input validation error")
	}
}
