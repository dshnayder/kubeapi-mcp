// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package udt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dmitryshnayder/kubeapi-mcp/pkg/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	udtGetListToolDescription = `
	Returns a list of available playbooks and their associated keywords.
	Universal Debug Tree (UDT) are used to troubleshoot and debug issues with GKE clusters.
	Call this tool to get the names of all available playbooks and when to use them based on the keywords.
	`
	udtGetPlaybookToolDescription = `
	Returns the content of a playbook given its name.
	Universal Debug Tree (UDT) are used to troubleshoot and debug issues with GKE clusters.
	The AI agent should follow this playbook when investigating the issue.
	`
)

type udtGetListArgs struct{}

type udtGetPlaybookArgs struct {
	Name string `json:"name"`
}

type playbookInfo struct {
	Name     string   `json:"name"`
	Keywords []string `json:"keywords"`
	Summary  string   `json:"summary"`
	Title    string   `json:"title"`
}

type handlers struct {
	playbooks   []playbookInfo
	playbookDir string
}

func Install(ctx context.Context, s *mcp.Server, c *config.Config) error {
	udtPath := c.UDTPath()
	if udtPath == "" {
		return nil
	}

	h := &handlers{
		playbookDir: udtPath,
	}
	if err := h.scanPlaybooks(); err != nil {
		return fmt.Errorf("failed to scan playbooks: %w", err)
	}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "udt_get_list",
		Description: udtGetListToolDescription,
	}, h.getList)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "udt_get_playbook",
		Description: udtGetPlaybookToolDescription,
	}, h.getPlaybook)

	return nil
}

func (h *handlers) scanPlaybooks() error {
	files, err := os.ReadDir(h.playbookDir)
	if err != nil {
		return err
	}

	reKeywords := regexp.MustCompile(`keywords:\s*"([^"]*)"`)

	for _, file := range files {
		if file.Type().IsRegular() && strings.HasSuffix(file.Name(), ".md") {
			name := strings.TrimSuffix(file.Name(), ".md")
			content, err := os.ReadFile(filepath.Join(h.playbookDir, file.Name()))
			if err != nil {
				// Log error or handle it, for now, skip the file
				continue
			}

			var keywords []string
			keywordMatches := reKeywords.FindAllStringSubmatch(string(content), -1)
			for _, match := range keywordMatches {
				if len(match) > 1 {
					keywords = append(keywords, strings.TrimSpace(match[1]))
				}
			}

			var summary string
			lines := strings.Split(string(content), "\n")
			inSummary := false
			var summaryLines []string
			var title string
			for _, line := range lines {
				trimmedLine := strings.TrimSpace(line)

				if title == "" && strings.HasPrefix(trimmedLine, "# ") {
					title = strings.TrimSpace(strings.TrimPrefix(trimmedLine, "# "))
				}

				if strings.HasPrefix(trimmedLine, "SUMMARY:") {
					inSummary = true
					summaryPart := strings.TrimSpace(strings.TrimPrefix(trimmedLine, "SUMMARY:"))
					if summaryPart != "" {
						summaryLines = append(summaryLines, summaryPart)
					}
					continue
				}

				if inSummary {
					if trimmedLine == "" {
						break // End of summary
					}
					summaryLines = append(summaryLines, trimmedLine)
				}
			}
			summary = strings.Join(summaryLines, " ")

			if len(keywords) > 0 {
				h.playbooks = append(h.playbooks, playbookInfo{
					Name:     name,
					Keywords: keywords,
					Summary:  summary,
					Title:    title,
				})
			}
		}
	}
	return nil
}

func (h *handlers) getList(ctx context.Context, _ *mcp.CallToolRequest, args *udtGetListArgs) (*mcp.CallToolResult, any, error) {
	b, err := json.Marshal(h.playbooks)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal playbooks: %w", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(b)},
		},
	}, nil, nil
}

func (h *handlers) getPlaybook(ctx context.Context, _ *mcp.CallToolRequest, args *udtGetPlaybookArgs) (*mcp.CallToolResult, any, error) {
	cleanName := filepath.Base(args.Name)
	filePath := filepath.Join(h.playbookDir, cleanName+".md")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("playbook %q not found", cleanName)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read playbook %q: %w", cleanName, err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(content)},
		},
	}, nil, nil
}
