package server

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Sublime12/local_code_review/git"
)

// Comment represents an inline review comment
type Comment struct {
	ID        string         `json:"id"`
	FilePath  string         `json:"filePath"`
	Line      int            `json:"line"`
	Text      string         `json:"text"`
	Snippet   []git.CodeLine `json:"snippet"`
	CreatedAt time.Time      `json:"createdAt"`
}

// CommentStore manages review comments in-memory
type CommentStore struct {
	mu       sync.RWMutex
	comments map[string]Comment
}

// NewCommentStore initializes an empty comment store
func NewCommentStore() *CommentStore {
	return &CommentStore{
		comments: make(map[string]Comment),
	}
}

// AddOrUpdate inserts or updates a comment
func (s *CommentStore) AddOrUpdate(c *Comment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.ID == "" {
		c.ID = fmt.Sprintf("%s:%d:%d", c.FilePath, c.Line, time.Now().UnixNano())
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	s.comments[c.ID] = *c
}

// Delete removes a comment by ID
func (s *CommentStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.comments, id)
}

// Clear removes all comments
func (s *CommentStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.comments = make(map[string]Comment)
}

func (s *CommentStore) sortedComments() []Comment {
	list := make([]Comment, 0, len(s.comments))
	for _, c := range s.comments {
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool {
		if !list[i].CreatedAt.Equal(list[j].CreatedAt) {
			return list[i].CreatedAt.Before(list[j].CreatedAt)
		}
		return list[i].ID < list[j].ID
	})
	return list
}

// List returns all active comments
func (s *CommentStore) List() []Comment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sortedComments()
}

// GeneratePrompt formats the review comments into the prompt for AI agents
func (s *CommentStore) GeneratePrompt() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.comments) == 0 {
		return "No review comments provided."
	}

	var sb strings.Builder
	sb.WriteString("Please address the following code review comments:\n\n")

	for _, c := range s.sortedComments() {
		if c.FilePath == "" {
			sb.WriteString(fmt.Sprintf("review (general): %s\n\n", c.Text))
			continue
		}

		sb.WriteString(fmt.Sprintf("review: %s\n", c.Text))
		if c.Line <= 0 {
			sb.WriteString(fmt.Sprintf("%s\n\n", c.FilePath))
			continue
		}

		sb.WriteString(fmt.Sprintf("%s:%d\n", c.FilePath, c.Line))
		if len(c.Snippet) > 0 {
			for _, line := range c.Snippet {
				sb.WriteString(fmt.Sprintf("%d %s\n", line.LineNumber, line.Content))
			}
		}
		sb.WriteString("\n")
	}

	return strings.TrimSpace(sb.String())
}

// SaveToFile saves the formatted review comments to a markdown file
func (s *CommentStore) SaveToFile(repoRoot, filename string) error {
	if filename == "" {
		filename = ".review.md"
	}
	fullPath := filepath.Join(repoRoot, filename)
	content := s.GeneratePrompt() + "\n"
	return os.WriteFile(fullPath, []byte(content), 0644)
}
