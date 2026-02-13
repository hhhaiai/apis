package orchestrator

import "context"

type Service interface {
	Complete(ctx context.Context, req Request) (Response, error)
	Stream(ctx context.Context, req Request) (<-chan StreamEvent, <-chan error)
}

type Request struct {
	RunID     string
	Model     string
	MaxTokens int
	System    any
	Messages  []Message
	Tools     []Tool
	Metadata  map[string]any
	Headers   map[string]string
}

type Message struct {
	Role    string
	Content any
}

type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type Response struct {
	Model      string
	Blocks     []AssistantBlock
	StopReason string
	Usage      Usage
	Trace      Trace
}

type AssistantBlock struct {
	Type  string
	Text  string
	ID    string
	Name  string
	Input map[string]any
}

type Usage struct {
	InputTokens  int
	OutputTokens int
}

type Trace struct {
	Provider         string
	Model            string
	FallbackUsed     bool
	ReflectionPasses int
	SelectedBy       string
	CandidateCount   int
	JudgeEnabled     bool
}

type StreamEvent struct {
	Type        string
	Index       int
	Block       AssistantBlock
	DeltaText   string
	DeltaJSON   string
	StopReason  string
	Usage       Usage
	RawEvent    string
	RawData     []byte
	PassThrough bool
}
