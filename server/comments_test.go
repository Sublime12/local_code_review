package server

import (
	"strings"
	"testing"

	"github.com/Sublime12/local_code_review/git"
)

func TestCommentStoreAndPromptGeneration(t *testing.T) {
	store := NewCommentStore()

	comment := Comment{
		FilePath: "file_path/a/b/c.js",
		Line:     89,
		Text:     "you should do this per example instead of doing this",
		Snippet: []git.CodeLine{
			{LineNumber: 87, Content: "content 87"},
			{LineNumber: 88, Content: "content 88"},
			{LineNumber: 89, Content: "content 89", IsTarget: true},
			{LineNumber: 90, Content: "content 90"},
			{LineNumber: 91, Content: "content 91"},
		},
	}

	store.AddOrUpdate(&comment)

	list := store.List()
	if len(list) != 1 {
		t.Fatalf("Expected 1 comment, got %d", len(list))
	}

	prompt := store.GeneratePrompt()

	expectedFragments := []string{
		"review: you should do this per example instead of doing this",
		"file_path/a/b/c.js:89",
		"87 content 87",
		"88 content 88",
		"89 content 89",
		"90 content 90",
		"91 content 91",
	}

	for _, frag := range expectedFragments {
		if !strings.Contains(prompt, frag) {
			t.Errorf("Prompt missing expected fragment: %q\nPrompt output:\n%s", frag, prompt)
		}
	}
}
