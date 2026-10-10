package gateway

type OpenAIChatCompletionsRequest struct {
	Model     string              `json:"model"`
	Messages  []OpenAIChatMessage `json:"messages"`
	MaxTokens int                 `json:"max_tokens,omitempty"`
	// Groq and newer OpenAI-compatible clients may send max_completion_tokens.
	MaxCompletionTokens int              `json:"max_completion_tokens,omitempty"`
	Stream              bool             `json:"stream,omitempty"`
	StreamOptions       map[string]any   `json:"stream_options,omitempty"`
	Tools               []OpenAIChatTool `json:"tools,omitempty"`
	ToolChoice          any              `json:"tool_choice,omitempty"`
	Temperature         *float64         `json:"temperature,omitempty"`
	TopP                *float64         `json:"top_p,omitempty"`
	ResponseFormat      any              `json:"response_format,omitempty"`
	Metadata            map[string]any   `json:"metadata,omitempty"`
}

type OpenAIChatMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content"`
	Name       string           `json:"name,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
}

type OpenAIChatTool struct {
	Type              string                        `json:"type"`
	Function          OpenAIChatToolDetails         `json:"function"`
	Name              string                        `json:"name,omitempty"`
	Description       string                        `json:"description,omitempty"`
	Strict            bool                          `json:"strict,omitempty"`
	Parameters        map[string]any                `json:"parameters,omitempty"`
	Tools             []OpenAIResponseNamespaceTool `json:"tools,omitempty"`
	ExternalWebAccess bool                          `json:"external_web_access,omitempty"`
}

type OpenAIChatToolDetails struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type OpenAIResponseNamespaceTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Strict      bool           `json:"strict,omitempty"`
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

type OpenAIModelsResponse struct {
	Object string        `json:"object"`
	Data   []OpenAIModel `json:"data"`
}

type OpenAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type OpenAIResponsesRequest struct {
	Model              string           `json:"model"`
	Input              any              `json:"input"`
	Instructions       any              `json:"instructions,omitempty"`
	MaxOutputTokens    int              `json:"max_output_tokens,omitempty"`
	Stream             bool             `json:"stream,omitempty"`
	StreamOptions      map[string]any   `json:"stream_options,omitempty"`
	Tools              []OpenAIChatTool `json:"tools,omitempty"`
	ToolChoice         any              `json:"tool_choice,omitempty"`
	Temperature        *float64         `json:"temperature,omitempty"`
	TopP               *float64         `json:"top_p,omitempty"`
	Metadata           map[string]any   `json:"metadata,omitempty"`
	PreviousResponseID string           `json:"previous_response_id,omitempty"`
	Reasoning          map[string]any   `json:"reasoning,omitempty"`
	Text               map[string]any   `json:"text,omitempty"`
	ParallelToolCalls  *bool            `json:"parallel_tool_calls,omitempty"`
	Store              *bool            `json:"store,omitempty"`
	Include            []string         `json:"include,omitempty"`
	Truncation         string           `json:"truncation,omitempty"`
	ServiceTier        string           `json:"service_tier,omitempty"`
	PromptCacheKey     string           `json:"prompt_cache_key,omitempty"`
	SafetyIdentifier   string           `json:"safety_identifier,omitempty"`
	TopLogprobs        *int             `json:"top_logprobs,omitempty"`
	Background         *bool            `json:"background,omitempty"`
	Conversation       any              `json:"conversation,omitempty"`
	MaxToolCalls       *int             `json:"max_tool_calls,omitempty"`
	ClientMetadata     map[string]any   `json:"client_metadata,omitempty"`
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
