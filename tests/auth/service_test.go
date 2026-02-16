package auth_test

import (
	"testing"

	"ccgateway/internal/auth"
)

func TestRegisterHashesPasswordAndLoginVerifies(t *testing.T) {
	svc := auth.NewInMemoryService()

	user, err := svc.Register("alice", "secret-pass", auth.RoleUser)
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if user.Password == "secret-pass" {
		t.Fatalf("password must be stored as hash")
	}

	loggedIn, err := svc.Login("alice", "secret-pass")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if loggedIn.Username != "alice" {
		t.Fatalf("unexpected user after login: %+v", loggedIn)
	}

	if _, err := svc.Login("alice", "wrong-pass"); err == nil {
		t.Fatalf("expected login failure with wrong password")
	}
}

func TestGetAndListReturnCopies(t *testing.T) {
	svc := auth.NewInMemoryService()
	created, err := svc.Register("copy-user", "secret-pass", auth.RoleUser)
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	got, err := svc.Get(created.ID)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	got.Username = "mutated-directly"
	got.Group = "vip"

	again, err := svc.Get(created.ID)
	if err != nil {
		t.Fatalf("get after mutation failed: %v", err)
	}
	if again.Username != "copy-user" {
		t.Fatalf("expected stored username unchanged, got %q", again.Username)
	}
	if again.Group == "vip" {
		t.Fatalf("expected stored group unchanged without update")
	}

	users := svc.List()
	if len(users) != 1 {
		t.Fatalf("expected 1 user in list, got %d", len(users))
	}
	users[0].Username = "mutated-from-list"
	last, err := svc.Get(created.ID)
	if err != nil {
		t.Fatalf("get after list mutation failed: %v", err)
	}
	if last.Username != "copy-user" {
		t.Fatalf("expected list return value to be copy, got %q", last.Username)
	}
}

func TestLinkGitHubAndWeChatEnforceUniqueness(t *testing.T) {
	svc := auth.NewInMemoryService()
	u1, err := svc.RegisterWithEmail("gh-u1", "u1@example.test", "secret-pass", auth.RoleUser)
	if err != nil {
		t.Fatalf("register u1 failed: %v", err)
	}
	u2, err := svc.RegisterWithEmail("gh-u2", "u2@example.test", "secret-pass", auth.RoleUser)
	if err != nil {
		t.Fatalf("register u2 failed: %v", err)
	}

	if err := svc.LinkGitHub(u1.ID, "gh-001"); err != nil {
		t.Fatalf("link github for u1 failed: %v", err)
	}
	if err := svc.LinkGitHub(u2.ID, "gh-001"); err == nil {
		t.Fatalf("expected github uniqueness conflict")
	}
	if err := svc.LinkWeChat(u1.ID, "wc-001"); err != nil {
		t.Fatalf("link wechat for u1 failed: %v", err)
	}
	if err := svc.LinkWeChat(u2.ID, "wc-001"); err == nil {
		t.Fatalf("expected wechat uniqueness conflict")
	}
}
