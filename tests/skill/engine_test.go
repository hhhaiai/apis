package skill_test

import (
	. "ccgateway/internal/skill"
	"testing"
)

func TestEngine_RegisterAndExecute(t *testing.T) {
	e := NewEngine()
	err := e.Register(Skill{
		Name:        "greet",
		Description: "Generate a greeting",
		Parameters:  []Param{{Name: "name", Required: true}},
		Template:    "Hello, {{name}}! Welcome to the platform.",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := e.Execute("greet", map[string]any{"name": "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	if result != "Hello, Alice! Welcome to the platform." {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestEngine_MissingRequired(t *testing.T) {
	e := NewEngine()
	_ = e.Register(Skill{
		Name:       "test",
		Parameters: []Param{{Name: "required_param", Required: true}},
		Template:   "{{required_param}}",
	})

	_, err := e.Execute("test", map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing required param")
	}
}

func TestEngine_DefaultValues(t *testing.T) {
	e := NewEngine()
	_ = e.Register(Skill{
		Name:       "test",
		Parameters: []Param{{Name: "lang", Default: "Go"}},
		Template:   "Write code in {{lang}}",
	})

	result, err := e.Execute("test", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if result != "Write code in Go" {
		t.Fatalf("expected default value, got %q", result)
	}
}

func TestEngine_List(t *testing.T) {
	e := NewEngine()
	_ = e.Register(Skill{Name: "a", Template: "t"})
	_ = e.Register(Skill{Name: "b", Template: "t"})
	if len(e.List()) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(e.List()))
	}
}

func TestEngine_Delete(t *testing.T) {
	e := NewEngine()
	_ = e.Register(Skill{Name: "a", Template: "t"})
	if err := e.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if len(e.List()) != 0 {
		t.Fatal("expected 0 skills after delete")
	}
}

func TestParseSkillMD(t *testing.T) {
	content := `---
name: review_code
description: Review code for issues
version: 2.0
parameters:
  - name: language
    required: true
    description: Programming language
  - name: style
    default: concise
---
Review the following {{language}} code. Be {{style}}.`

	s, err := ParseSkillMD(content)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "review_code" {
		t.Fatalf("expected name review_code, got %q", s.Name)
	}
	if s.Version != "2.0" {
		t.Fatalf("expected version 2.0, got %q", s.Version)
	}
	if len(s.Parameters) != 2 {
		t.Fatalf("expected 2 params, got %d", len(s.Parameters))
	}
	if !s.Parameters[0].Required {
		t.Fatal("first param should be required")
	}
	if s.Parameters[1].Default != "concise" {
		t.Fatalf("second param default should be concise, got %q", s.Parameters[1].Default)
	}
}

func TestParseSkillMD_MissingName(t *testing.T) {
	content := `---
description: No name
---
template body`

	_, err := ParseSkillMD(content)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}
