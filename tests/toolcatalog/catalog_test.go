package toolcatalog_test

import (
	. "ccgateway/internal/toolcatalog"
	"testing"
)

func TestCatalogCheckAllowed(t *testing.T) {
	c := NewCatalog([]ToolSpec{
		{Name: "get_weather", Status: StatusSupported},
		{Name: "web_search", Status: StatusExperimental},
		{Name: "sql_exec", Status: StatusUnsupported},
	})

	if err := c.CheckAllowed("get_weather", false, false); err != nil {
		t.Fatalf("supported tool should pass: %v", err)
	}
	if err := c.CheckAllowed("web_search", false, true); err == nil {
		t.Fatalf("experimental tool should fail when disabled")
	}
	if err := c.CheckAllowed("web_search", true, true); err != nil {
		t.Fatalf("experimental tool should pass when enabled: %v", err)
	}
	if err := c.CheckAllowed("sql_exec", true, true); err == nil {
		t.Fatalf("unsupported tool should fail")
	}
	if err := c.CheckAllowed("unknown_tool", false, false); err == nil {
		t.Fatalf("unknown tool should fail when unknown disabled")
	}
	if err := c.CheckAllowed("unknown_tool", false, true); err != nil {
		t.Fatalf("unknown tool should pass when unknown enabled: %v", err)
	}
}
