package tenant_test

import (
	. "ccgateway/internal/tenant"
	"testing"
)

func TestManager_CreateAndGet(t *testing.T) {
	m := NewManager()
	tenant, err := m.Create("t1", "Acme", "key-123", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if tenant.Name != "Acme" {
		t.Fatalf("expected Acme, got %s", tenant.Name)
	}
	got, ok := m.Get("t1")
	if !ok {
		t.Fatal("tenant not found")
	}
	if got.APIKey != "key-123" {
		t.Fatalf("expected key-123, got %s", got.APIKey)
	}
}

func TestManager_DuplicateID(t *testing.T) {
	m := NewManager()
	_, _ = m.Create("t1", "Acme", "key-1", 0, 0)
	_, err := m.Create("t1", "Acme2", "key-2", 0, 0)
	if err == nil {
		t.Fatal("expected duplicate ID error")
	}
}

func TestManager_DuplicateKey(t *testing.T) {
	m := NewManager()
	_, _ = m.Create("t1", "Acme", "key-1", 0, 0)
	_, err := m.Create("t2", "Beta", "key-1", 0, 0)
	if err == nil {
		t.Fatal("expected duplicate key error")
	}
}

func TestManager_Authenticate(t *testing.T) {
	m := NewManager()
	_, _ = m.Create("t1", "Acme", "key-123", 0, 0)
	tenant, err := m.Authenticate("key-123")
	if err != nil {
		t.Fatal(err)
	}
	if tenant.ID != "t1" {
		t.Fatalf("expected t1, got %s", tenant.ID)
	}
}

func TestManager_AuthenticateInvalidKey(t *testing.T) {
	m := NewManager()
	_, err := m.Authenticate("invalid")
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestManager_Deactivate(t *testing.T) {
	m := NewManager()
	_, _ = m.Create("t1", "Acme", "key-123", 0, 0)
	_ = m.Deactivate("t1")
	_, err := m.Authenticate("key-123")
	if err == nil {
		t.Fatal("expected error for deactivated tenant")
	}
	_ = m.Activate("t1")
	_, err = m.Authenticate("key-123")
	if err != nil {
		t.Fatalf("should work after reactivation: %v", err)
	}
}

func TestManager_RateLimit(t *testing.T) {
	m := NewManager()
	_, _ = m.Create("t1", "Acme", "key-123", 2, 0) // 2 RPM
	_, _ = m.Authenticate("key-123")
	_, _ = m.Authenticate("key-123")
	_, err := m.Authenticate("key-123")
	if err == nil {
		t.Fatal("expected rate limit error")
	}
}

func TestManager_DeleteAndList(t *testing.T) {
	m := NewManager()
	_, _ = m.Create("t1", "Acme", "key-1", 0, 0)
	_, _ = m.Create("t2", "Beta", "key-2", 0, 0)
	if len(m.List()) != 2 {
		t.Fatalf("expected 2 tenants")
	}
	_ = m.Delete("t1")
	if len(m.List()) != 1 {
		t.Fatalf("expected 1 tenant after delete")
	}
}

func TestManager_RecordTokens(t *testing.T) {
	m := NewManager()
	_, _ = m.Create("t1", "Acme", "key-123", 0, 0)
	m.RecordTokens("t1", 1000, 0.05)
	got, _ := m.Get("t1")
	if got.Usage.Tokens != 1000 {
		t.Fatalf("expected 1000 tokens, got %d", got.Usage.Tokens)
	}
	if got.Usage.CostUSD != 0.05 {
		t.Fatalf("expected 0.05 cost, got %f", got.Usage.CostUSD)
	}
}
