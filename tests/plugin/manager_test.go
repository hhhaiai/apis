package plugin_test

import (
	. "ccgateway/internal/plugin"
	"testing"
)

func TestManager_InstallAndList(t *testing.T) {
	m := NewManager()
	err := m.Install(Plugin{
		Name:        "test-plugin",
		Description: "A test plugin",
		Skills:      []SkillConfig{{Name: "greet", Template: "Hello {{name}}"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.List()) != 1 {
		t.Fatal("expected 1 plugin")
	}
}

func TestManager_DuplicateInstall(t *testing.T) {
	m := NewManager()
	_ = m.Install(Plugin{Name: "dup"})
	err := m.Install(Plugin{Name: "dup"})
	if err == nil {
		t.Fatal("expected error for duplicate install")
	}
}

func TestManager_Uninstall(t *testing.T) {
	m := NewManager()
	_ = m.Install(Plugin{Name: "removable"})
	if err := m.Uninstall("removable"); err != nil {
		t.Fatal(err)
	}
	if len(m.List()) != 0 {
		t.Fatal("expected 0 plugins after uninstall")
	}
}

func TestManager_EnableDisable(t *testing.T) {
	m := NewManager()
	_ = m.Install(Plugin{Name: "toggleable"})
	p, _ := m.Get("toggleable")
	if !p.Enabled {
		t.Fatal("new plugin should be enabled")
	}
	_ = m.Disable("toggleable")
	p, _ = m.Get("toggleable")
	if p.Enabled {
		t.Fatal("should be disabled")
	}
	_ = m.Enable("toggleable")
	p, _ = m.Get("toggleable")
	if !p.Enabled {
		t.Fatal("should be enabled again")
	}
}

func TestManager_NotFound(t *testing.T) {
	m := NewManager()
	_, ok := m.Get("nonexistent")
	if ok {
		t.Fatal("should not find nonexistent plugin")
	}
	if err := m.Uninstall("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent plugin")
	}
}

func TestManager_EmptyName(t *testing.T) {
	m := NewManager()
	err := m.Install(Plugin{Name: ""})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}
