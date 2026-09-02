package harness

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestModelOutputPreviewSeparatesAndTruncatesReasoningAndAnswer(t *testing.T) {
	message := &schema.AgenticMessage{ContentBlocks: []*schema.ContentBlock{
		schema.NewContentBlock(&schema.Reasoning{Text: "思考"}),
		schema.NewContentBlock(&schema.AssistantGenText{Text: "回答"}),
	}}
	reasoning, answer := modelOutputPreview(message)
	if reasoning != "思考" || answer != "回答" {
		t.Fatalf("reasoning=%q answer=%q", reasoning, answer)
	}
	message.ContentBlocks[0].Reasoning.Text = strings.Repeat("思", maxToolLogPreviewRunes+1)
	reasoning, _ = modelOutputPreview(message)
	if !strings.Contains(reasoning, "truncated; original_runes=") {
		t.Fatalf("preview=%q", reasoning)
	}
}
