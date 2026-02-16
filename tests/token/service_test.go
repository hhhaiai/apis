package token_test

import (
	"strings"
	"testing"

	"ccgateway/internal/token"
)

func TestGenerateUsesRandomOpaqueTokenValue(t *testing.T) {
	svc := token.NewInMemoryService()

	t1, err := svc.Generate("userA", 100)
	if err != nil {
		t.Fatalf("generate token1 failed: %v", err)
	}
	t2, err := svc.Generate("userA", 100)
	if err != nil {
		t.Fatalf("generate token2 failed: %v", err)
	}

	if t1.Value == t2.Value {
		t.Fatalf("generated token values must be unique")
	}
	if !strings.HasPrefix(t1.Value, "sk-") {
		t.Fatalf("unexpected token prefix: %q", t1.Value)
	}
	if strings.Contains(t1.Value, "userA") {
		t.Fatalf("token value must not expose user id: %q", t1.Value)
	}
}

func TestDeductQuotaForLimitedAndUnlimitedTokens(t *testing.T) {
	svc := token.NewInMemoryService()

	limited, err := svc.Generate("limited", 10)
	if err != nil {
		t.Fatalf("generate limited token failed: %v", err)
	}
	if err := svc.DeductQuota(limited.Value, 7); err != nil {
		t.Fatalf("deduct quota failed: %v", err)
	}
	gotLimited, err := svc.Validate(limited.Value)
	if err != nil {
		t.Fatalf("validate limited token failed: %v", err)
	}
	if gotLimited.Quota != 3 {
		t.Fatalf("expected remaining quota 3, got %d", gotLimited.Quota)
	}

	if err := svc.DeductQuota(limited.Value, 5); err != token.ErrQuotaExceeded {
		t.Fatalf("expected ErrQuotaExceeded, got %v", err)
	}

	unlimited, err := svc.Generate("unlimited", 0)
	if err != nil {
		t.Fatalf("generate unlimited token failed: %v", err)
	}
	if err := svc.DeductQuota(unlimited.Value, 10000); err != nil {
		t.Fatalf("deduct quota for unlimited token failed: %v", err)
	}
	gotUnlimited, err := svc.Validate(unlimited.Value)
	if err != nil {
		t.Fatalf("validate unlimited token failed: %v", err)
	}
	if !gotUnlimited.UnlimitedQuota {
		t.Fatalf("expected unlimited token flag")
	}
}

func TestUpdateNormalizesStatusAndUnlimitedQuota(t *testing.T) {
	svc := token.NewInMemoryService()

	tk, err := svc.Generate("userA", 10)
	if err != nil {
		t.Fatalf("generate token failed: %v", err)
	}

	tk.Status = 0
	if err := svc.Update(tk); err != nil {
		t.Fatalf("update token status failed: %v", err)
	}
	if _, err := svc.Validate(tk.Value); err != token.ErrTokenDisabled {
		t.Fatalf("expected disabled token after status=0 update, got %v", err)
	}

	updated, err := svc.Get(tk.Value)
	if err != nil {
		t.Fatalf("get token failed: %v", err)
	}
	updated.Status = token.StatusEnabled
	updated.Quota = 0
	updated.UnlimitedQuota = false
	if err := svc.Update(updated); err != nil {
		t.Fatalf("update token quota failed: %v", err)
	}

	got, err := svc.Validate(tk.Value)
	if err != nil {
		t.Fatalf("validate token after unlimited update failed: %v", err)
	}
	if !got.UnlimitedQuota {
		t.Fatalf("expected quota=0 update to become unlimited")
	}
}
