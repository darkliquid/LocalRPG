package gui

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed docs/*.md
var embeddedDocsFS embed.FS

// ErrDocNotFound is returned when an article ID does not match any embedded guide.
var ErrDocNotFound = errors.New("document not found")

type docFrontmatter struct {
	ID          string `yaml:"id"`
	Title       string `yaml:"title"`
	Category    string `yaml:"category"`
	Order       int    `yaml:"order"`
	Description string `yaml:"description"`
}

// parseDocFile reads frontmatter and body from markdown bytes.
func parseDocFile(data []byte) (*docFrontmatter, string, error) {
	content := string(data)
	normalized := strings.ReplaceAll(content, "\r\n", "\n")

	if !strings.HasPrefix(normalized, "---\n") {
		return nil, "", fmt.Errorf("missing frontmatter delimiter")
	}

	endIdx := strings.Index(normalized[4:], "\n---\n")
	if endIdx == -1 {
		return nil, "", fmt.Errorf("unclosed frontmatter delimiter")
	}

	fmRaw := normalized[4 : 4+endIdx]
	bodyRaw := strings.TrimSpace(normalized[4+endIdx+5:])

	var fm docFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return nil, "", fmt.Errorf("parse frontmatter yaml: %w", err)
	}

	return &fm, bodyRaw, nil
}

// GetDocsList returns metadata for all embedded documentation articles sorted by category and order.
func (s *Service) GetDocsList(_ context.Context) ([]DocArticleSummaryDTO, error) {
	entries, err := fs.ReadDir(embeddedDocsFS, "docs")
	if err != nil {
		return nil, fmt.Errorf("read embedded docs: %w", err)
	}

	var summaries []DocArticleSummaryDTO
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		data, err := embeddedDocsFS.ReadFile("docs/" + entry.Name())
		if err != nil {
			continue
		}

		fm, _, err := parseDocFile(data)
		if err != nil {
			continue
		}

		id := fm.ID
		if id == "" {
			id = strings.TrimSuffix(entry.Name(), ".md")
		}

		summaries = append(summaries, DocArticleSummaryDTO{
			ID:          id,
			Title:       fm.Title,
			Category:    fm.Category,
			Order:       fm.Order,
			Description: fm.Description,
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Order != summaries[j].Order {
			return summaries[i].Order < summaries[j].Order
		}
		if summaries[i].Category != summaries[j].Category {
			return summaries[i].Category < summaries[j].Category
		}
		return summaries[i].ID < summaries[j].ID
	})

	return summaries, nil
}

// GetDocArticle retrieves an article by its ID.
func (s *Service) GetDocArticle(_ context.Context, id string) (*DocArticleDTO, error) {
	cleanID := strings.TrimSpace(id)
	cleanID = strings.TrimPrefix(cleanID, "/")
	cleanID = strings.TrimSuffix(cleanID, ".md")

	entries, err := fs.ReadDir(embeddedDocsFS, "docs")
	if err != nil {
		return nil, fmt.Errorf("read embedded docs: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		data, err := embeddedDocsFS.ReadFile("docs/" + entry.Name())
		if err != nil {
			continue
		}

		fm, body, err := parseDocFile(data)
		if err != nil {
			continue
		}

		docID := fm.ID
		if docID == "" {
			docID = strings.TrimSuffix(entry.Name(), ".md")
		}

		if docID == cleanID || strings.TrimSuffix(entry.Name(), ".md") == cleanID {
			return &DocArticleDTO{
				DocArticleSummaryDTO: DocArticleSummaryDTO{
					ID:          docID,
					Title:       fm.Title,
					Category:    fm.Category,
					Order:       fm.Order,
					Description: fm.Description,
				},
				Content: body,
			}, nil
		}
	}

	return nil, ErrDocNotFound
}
