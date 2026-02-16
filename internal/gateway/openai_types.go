package gateway

type OpenAIChatCompletionsRequest struct {
	Model         string              `json:"model"`
	Messages      []OpenAIChatMessage `json:"messages"`
	MaxTokens     int                 `json:"max_tokens,omitempty"`
	Stream        bool                `json:"stream,omitempty"`
	StreamOptions map[string]any      `json:"stream_options,omitempty"`
	Tools         []OpenAIChatTool    `json:"tools,omitempty"`
	ToolChoice    any                 `json:"tool_choice,omitempty"`
	Temperature   *float64            `json:"temperature,omitempty"`
	TopP          *float64            `json:"top_p,omitempty"`
	Metadata      map[string]any      `json:"metadata,omitempty"`
}

type OpenAIChatMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content"`
	Name       string           `json:"name,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
}

type OpenAIChatTool struct {
	Type     string                `json:"type"`
	Function OpenAIChatToolDetails `json:"function"`
}

type OpenAIChatToolDetails struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type OpenAIChatCompletionsResponse struct {
	ID      string                       `json:"id"`
	Object  string                       `json:"object"`
	Created int64                        `json:"created"`
	Model   string                       `json:"model"`
	Choices []OpenAIChatCompletionChoice `json:"choices"`
	Usage   OpenAIUsage                  `json:"usage"`
}

type OpenAIChatCompletionChoice struct {
	Index        int                       `json:"index"`
	Message      OpenAIChatResponseMessage `json:"message"`
	FinishReason string                    `json:"finish_reason"`
}

type OpenAIChatResponseMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content,omitempty"`
	ToolCalls []OpenAIToolCall `json:"tool_calls,omitempty"`
}

type OpenAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function OpenAIToolFunction `json:"function"`
}

type OpenAIToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type OpenAIResponsesRequest struct {
	Model           string           `json:"model"`
	Input           any              `json:"input"`
	MaxOutputTokens int              `json:"max_output_tokens,omitempty"`
	Stream          bool             `json:"stream,omitempty"`
	StreamOptions   map[string]any   `json:"stream_options,omitempty"`
	Tools           []OpenAIChatTool `json:"tools,omitempty"`
	ToolChoice      any              `json:"tool_choice,omitempty"`
	Temperature     *float64         `json:"temperature,omitempty"`
	TopP            *float64         `json:"top_p,omitempty"`
	Metadata        map[string]any   `json:"metadata,omitempty"`
}

type OpenAIResponsesResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Status  string                 `json:"status"`
	Output  []OpenAIResponseOutput `json:"output"`
	Usage   OpenAIUsage            `json:"usage"`
}

type OpenAIResponseOutput struct {
	Type    string                  `json:"type"`
	ID      string                  `json:"id,omitempty"`
	Role    string                  `json:"role,omitempty"`
	Content []OpenAIResponseContent `json:"content,omitempty"`
	Name    string                  `json:"name,omitempty"`
	CallID  string                  `json:"call_id,omitempty"`
	Args    string                  `json:"arguments,omitempty"`
}

type OpenAIResponseContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
