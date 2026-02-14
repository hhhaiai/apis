package gateway

import (
	"fmt"
	"strings"

	"ccgateway/internal/orchestrator"
)

func collectResponseText(resp orchestrator.Response) string {
	parts := []string{}
	for _, block := range resp.Blocks {
		if strings.ToLower(strings.TrimSpace(block.Type)) != "text" {
			continue
		}
		text := strings.TrimSpace(block.Text)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n")
}

func appendStreamText(builder *strings.Builder, ev orchestrator.StreamEvent) {
	if builder == nil {
		return
	}
	if delta := strings.TrimSpace(ev.DeltaText); delta != "" {
		builder.WriteString(delta)
		return
	}
	if strings.ToLower(strings.TrimSpace(ev.Block.Type)) == "text" {
		if text := strings.TrimSpace(ev.Block.Text); text != "" {
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(text)
		}
	}
}

func buildRunRecordText(path, mode string, status int, stream bool, outputText, errText string) string {
	parts := []string{
		strings.TrimSpace(path),
		fmt.Sprintf("status=%d", status),
	}
	if mode = strings.TrimSpace(mode); mode != "" {
		parts = append(parts, "mode="+mode)
	}
	if stream {
		parts = append(parts, "stream=true")
	}
	if output := normalizeSpaces(outputText); output != "" {
		parts = append(parts, fmt.Sprintf(`output="%s"`, truncateText(output, 260)))
	}
	if errText = normalizeSpaces(errText); errText != "" {
		parts = append(parts, fmt.Sprintf(`error="%s"`, truncateText(errText, 160)))
	}
	return strings.Join(parts, " | ")
}

func compactOutputForEvent(outputText string) string {
	return truncateText(normalizeSpaces(outputText), 800)
}
