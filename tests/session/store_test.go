package session_test

import (
	. "ccgateway/internal/session"
	"testing"
)

func TestStoreCreateGetList(t *testing.T) {
	st := NewStore()

	first, err := st.Create(CreateInput{
		ID:    "sess_a",
		Title: "first",
		Metadata: map[string]any{
			"team": "alpha",
		},
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := st.Create(CreateInput{
		ID:    "sess_b",
		Title: "second",
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	if first.Type != "session" {
		t.Fatalf("expected type=session, got %q", first.Type)
	}
	got, ok := st.Get("sess_a")
	if !ok {
		t.Fatalf("expected session found")
	}
	if got.Title != "first" {
		t.Fatalf("unexpected title: %q", got.Title)
	}
	got.Metadata["team"] = "mutated"
	gotAgain, ok := st.Get("sess_a")
	if !ok {
		t.Fatalf("expected session found on second get")
	}
	if gotAgain.Metadata["team"] != "alpha" {
		t.Fatalf("expected metadata clone isolation, got %#v", gotAgain.Metadata["team"])
	}

	list := st.List(10)
	if len(list) != 2 {
		t.Fatalf("expected 2 sessions in list, got %d", len(list))
	}
	if list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("expected reverse chronological order, got %#v", []string{list[0].ID, list[1].ID})
	}
}

func TestStoreForkInheritAndOverride(t *testing.T) {
	st := NewStore()
	parent, err := st.Create(CreateInput{
		ID:    "sess_parent",
		Title: "parent",
		Metadata: map[string]any{
			"mode": "plan",
		},
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	child, err := st.Fork(parent.ID, CreateInput{})
	if err != nil {
		t.Fatalf("fork child: %v", err)
	}
	if child.ParentID != parent.ID {
		t.Fatalf("expected parent_id=%s, got %s", parent.ID, child.ParentID)
	}
	if child.Title != parent.Title {
		t.Fatalf("expected inherited title %q, got %q", parent.Title, child.Title)
	}
	if child.Metadata["mode"] != "plan" {
		t.Fatalf("expected inherited metadata, got %#v", child.Metadata)
	}

	override, err := st.Fork(parent.ID, CreateInput{
		Title: "child-override",
		Metadata: map[string]any{
			"mode": "chat",
		},
	})
	if err != nil {
		t.Fatalf("fork override: %v", err)
	}
	if override.Title != "child-override" {
		t.Fatalf("expected override title, got %q", override.Title)
	}
	if override.Metadata["mode"] != "chat" {
		t.Fatalf("expected override metadata, got %#v", override.Metadata)
	}
}

func TestStoreCreateRejectDuplicateID(t *testing.T) {
	st := NewStore()
	if _, err := st.Create(CreateInput{ID: "sess_dup"}); err != nil {
		t.Fatalf("create first duplicate candidate: %v", err)
	}
	if _, err := st.Create(CreateInput{ID: "sess_dup"}); err == nil {
		t.Fatalf("expected duplicate id error")
	}
}
