package channel

import (
	"strings"
	"time"
)

const (
	StatusUnknown          = 0
	StatusEnabled         = 1
	StatusManuallyDisabled = 2
	StatusAutoDisabled    = 3
)

// Channel represents an upstream API channel
type Channel struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Type         string    `json:"type"` // "openai", "anthropic", "custom", etc.
	Key          string    `json:"-"`    // Secret - never expose in JSON

	BaseURL     *string   `json:"base_url,omitempty"`
	Models      string    `json:"models"` // Comma-separated list of supported models

	Status      int       `json:"status"`
	Weight      uint      `json:"weight"` // For load balancing

	Group       string    `json:"group"` // "default", "vip", "enterprise"

	Priority    int64     `json:"priority"` // Higher = more preferred

	ResponseTime int      `json:"response_time_ms"` // Last response time in ms
	TestTime     int64    `json:"test_time"`      // Last test timestamp
	Balance     float64   `json:"balance"`        // Account balance (for quota tracking)

	ModelMapping *string  `json:"model_mapping,omitempty"` // Custom model name mapping

	UsedQuota   int64     `json:"used_quota"` // Total used quota

	Config      string    `json:"config,omitempty"` // Additional config as JSON

	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// IsEnabled checks if channel is available
func (c *Channel) IsEnabled() bool {
	return c.Status == StatusEnabled
}

// CanHandleModel checks if channel can handle specific model
func (c *Channel) CanHandleModel(model string) bool {
	if c.Models == "" {
		return true // No restriction
	}
	models := splitAndTrim(c.Models, ",")
	for _, m := range models {
		if m == model {
			return true
		}
	}
	return false
}

// GetWeight returns effective weight (0 = disabled)
func (c *Channel) GetWeight() uint {
	if !c.IsEnabled() {
		return 0
	}
	return c.Weight
}

func splitAndTrim(s, sep string) []string {
	var result []string
	for _, part := range strings.Split(s, sep) {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
