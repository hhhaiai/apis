package agentteam

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ccgateway/internal/subagent"
)

type subagentRunner interface {
	Spawn(ctx context.Context, cfg subagent.SpawnConfig) (subagent.Agent, error)
	Wait(ctx context.Context, id string, pollInterval time.Duration) (subagent.Agent, error)
}

// NewSubagentTaskFunc bridges team task execution to subagent lifecycle.
func NewSubagentTaskFunc(runner subagentRunner) TaskFunc {
	if runner == nil {
		return nil
	}
	return func(ctx context.Context, agent Agent, task Task) (string, error) {
		taskText := renderTaskText(task)
		if taskText == "" {
			taskText = "team task"
		}
		spawned, err := runner.Spawn(ctx, subagent.SpawnConfig{
			ParentID:    parentIDForSubagent(agent, task),
			Model:       strings.TrimSpace(agent.Model),
			Task:        taskText,
			Permissions: extractPermissions(agent.Meta),
		})
		if err != nil {
			return "", err
		}

		done, err := runner.Wait(ctx, spawned.ID, 20*time.Millisecond)
		if err != nil {
			return "", err
		}
		result := strings.TrimSpace(done.Result)
		if result == "" {
			result = "completed"
		}
		return fmt.Sprintf("%s (subagent=%s)", result, done.ID), nil
	}
}

func renderTaskText(task Task) string {
	title := strings.TrimSpace(task.Title)
	desc := strings.TrimSpace(task.Description)
	switch {
	case title == "":
		return desc
	case desc == "":
		return title
	default:
		return title + "\n\n" + desc
	}
}

func extractPermissions(meta map[string]any) []string {
	if len(meta) == 0 {
		return nil
	}
	raw, ok := meta["permissions"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return cleanStrings(v)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			text, ok := item.(string)
			if !ok {
				continue
			}
			text = strings.TrimSpace(text)
			if text != "" {
				out = append(out, text)
			}
		}
		return cleanStrings(out)
	default:
		return nil
	}
}

func parentIDForSubagent(agent Agent, task Task) string {
	if len(task.Meta) > 0 {
		if teamID, ok := task.Meta["team_id"].(string); ok {
			teamID = strings.TrimSpace(teamID)
			if teamID != "" {
				return teamID
			}
		}
	}
	return strings.TrimSpace(agent.ID)
}
