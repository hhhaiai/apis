package hooks_test

import (
	. "ccgateway/internal/hooks"
	"context"
	"fmt"
	"testing"
)

func TestRegistry_FireOrder(t *testing.T) {
	r := NewRegistry()
	var order []string

	_ = r.Register("low", PreRequest, 1, func(ctx context.Context, data map[string]any) (map[string]any, error) {
		order = append(order, "low")
		return data, nil
	})
	_ = r.Register("high", PreRequest, 10, func(ctx context.Context, data map[string]any) (map[string]any, error) {
		order = append(order, "high")
		return data, nil
	})

	_, err := r.Fire(context.Background(), PreRequest, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "high" || order[1] != "low" {
		t.Fatalf("expected [high, low], got %v", order)
	}
}

func TestRegistry_DataChaining(t *testing.T) {
	r := NewRegistry()

	_ = r.Register("add_field", PreRequest, 1, func(ctx context.Context, data map[string]any) (map[string]any, error) {
		data["added"] = true
		return data, nil
	})
	_ = r.Register("modify_field", PreRequest, 0, func(ctx context.Context, data map[string]any) (map[string]any, error) {
		if _, ok := data["added"]; ok {
			data["modified"] = true
		}
		return data, nil
	})

	result, err := r.Fire(context.Background(), PreRequest, map[string]any{"original": true})
	if err != nil {
		t.Fatal(err)
	}
	if result["added"] != true || result["modified"] != true {
		t.Fatalf("chaining failed: %v", result)
	}
}

func TestRegistry_ErrorStopsExecution(t *testing.T) {
	r := NewRegistry()
	executed := false

	_ = r.Register("fail", PreRequest, 10, func(ctx context.Context, data map[string]any) (map[string]any, error) {
		return nil, fmt.Errorf("deliberate error")
	})
	_ = r.Register("after", PreRequest, 1, func(ctx context.Context, data map[string]any) (map[string]any, error) {
		executed = true
		return data, nil
	})

	_, err := r.Fire(context.Background(), PreRequest, map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}
	if executed {
		t.Fatal("second hook should not have executed after error")
	}
}

func TestRegistry_NoHooks(t *testing.T) {
	r := NewRegistry()
	data := map[string]any{"key": "value"}
	result, err := r.Fire(context.Background(), PreRequest, data)
	if err != nil {
		t.Fatal(err)
	}
	if result["key"] != "value" {
		t.Fatal("data should pass through unchanged")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	r := NewRegistry()
	_ = r.Register("test", PreRequest, 1, func(ctx context.Context, data map[string]any) (map[string]any, error) {
		return data, nil
	})
	if r.Count() != 1 {
		t.Fatal("expected 1 hook")
	}
	r.Unregister("test", PreRequest)
	if r.Count() != 0 {
		t.Fatal("expected 0 hooks after unregister")
	}
}

func TestRegistry_DifferentPoints(t *testing.T) {
	r := NewRegistry()
	_ = r.Register("pre", PreRequest, 1, func(ctx context.Context, data map[string]any) (map[string]any, error) {
		return data, nil
	})
	_ = r.Register("post", PostResponse, 1, func(ctx context.Context, data map[string]any) (map[string]any, error) {
		return data, nil
	})

	list := r.List()
	if len(list[PreRequest]) != 1 || len(list[PostResponse]) != 1 {
		t.Fatalf("expected 1 hook per point, got pre=%d post=%d", len(list[PreRequest]), len(list[PostResponse]))
	}
}
