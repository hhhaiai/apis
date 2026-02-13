package gateway

type MessagesRequest struct {
	Model       string           `json:"model"`
	MaxTokens   int              `json:"max_tokens"`
	Messages    []MessageParam   `json:"messages"`
	System      any              `json:"system,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
	Temperature *float64         `json:"temperature,omitempty"`
	TopP        *float64         `json:"top_p,omitempty"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	ToolChoice  any              `json:"tool_choice,omitempty"`
	Metadata    map[string]any   `json:"metadata,omitempty"`
}

type MessageParam struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

type MessageResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Model        string         `json:"model"`
	Content      []ContentBlock `json:"content"`
	StopReason   string         `json:"stop_reason,omitempty"`
	StopSequence *string        `json:"stop_sequence"`
	Usage        UsageResponse  `json:"usage"`
}

type ContentBlock struct {
	Type  string         `json:"type"`
	Text  string         `json:"text,omitempty"`
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`
}

type UsageResponse struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type CountTokensRequest struct {
	Model    string         `json:"model"`
	Messages []MessageParam `json:"messages"`
	System   any            `json:"system,omitempty"`
}

type CountTokensResponse struct {
	InputTokens int `json:"input_tokens"`
}

type ErrorEnvelope struct {
	Type  string        `json:"type"`
	Error ErrorResponse `json:"error"`
}

type ErrorResponse struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}
