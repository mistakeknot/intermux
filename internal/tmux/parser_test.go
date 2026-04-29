package tmux

import (
	"reflect"
	"testing"
)

func TestParsePaneContentExtractsLowercaseAndMixedCaseBeadIDs(t *testing.T) {
	content := `bd show sylveste-kgfi.1 --long
○ MediumSetting-ni9 · Interlacer mashup: Alone in Kyoto with Hindustani classical instruments
Working bead: dtla-vjp.5 before public screenshot cleanup
$`

	got := ParsePaneContent(content, "ghostty-sylveste-claude")
	want := []string{"sylveste-kgfi.1", "MediumSetting-ni9", "dtla-vjp.5"}
	if !reflect.DeepEqual(got.ActiveBeads, want) {
		t.Fatalf("ActiveBeads = %#v, want %#v", got.ActiveBeads, want)
	}
}

func TestParsePaneContentDoesNotTreatGenericHyphenatedCommandsAsBeads(t *testing.T) {
	content := `git push origin main
no tracker context here, just a hyphenated phrase like hot-fix and git-push
bd search hot-fix
issue git-push
✓ git-push completed
$`

	got := ParsePaneContent(content, "ghostty-sylveste-claude")
	if len(got.ActiveBeads) != 0 {
		t.Fatalf("ActiveBeads = %#v, want none", got.ActiveBeads)
	}
}

func TestParsePaneContentExtractsBdCommandPositionIDsWithoutTitleFalsePositives(t *testing.T) {
	content := `bd update sylveste-kgfi.1 --title hot-fix
bd dep sylveste-kgfi.1 --blocks sylveste-kgfi.2
$`

	got := ParsePaneContent(content, "ghostty-sylveste-claude")
	want := []string{"sylveste-kgfi.1", "sylveste-kgfi.2"}
	if !reflect.DeepEqual(got.ActiveBeads, want) {
		t.Fatalf("ActiveBeads = %#v, want %#v", got.ActiveBeads, want)
	}
}
