package tmux

import (
	"testing"

	"github.com/mistakeknot/intermux/internal/activity"
)

func TestParsePaneContentDerivesStatusFromTheLastLine(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    activity.AgentStatus
	}{
		{
			name:    "spinner glyph plus status word on the last line is active",
			content: "✻ Thinking…",
			want:    activity.StatusActive,
		},
		{
			name:    "a bare shell prompt on the last line is idle",
			content: "$ ",
			want:    activity.StatusIdle,
		},
		{
			name: "a finished status banner earlier in the window does not outlive its prompt",
			content: "$ pytest\n" +
				"Running 12 tests\n" +
				"..........\n" +
				"12 passed in 1.2s\n" +
				"$ ",
			want: activity.StatusIdle,
		},
		{
			name:    "spinner-prefixed status word is active",
			content: "⠋ Running tests…",
			want:    activity.StatusActive,
		},
		{
			name:    "an empty pane is unknown",
			content: "",
			want:    activity.StatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParsePaneContent(tt.content, "ghostty-sylveste-claude")
			if got.Status != tt.want {
				t.Errorf("Status = %q, want %q", got.Status, tt.want)
			}
		})
	}
}
