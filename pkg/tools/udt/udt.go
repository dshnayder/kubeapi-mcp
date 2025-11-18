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
	udtListPlaybooksToolDescription = `
This tool scans a predefined directory for Markdown playbook files, extracts their names, associated keywords, a summary, and a title. The keywords are extracted from lines starting with 'keywords:', the summary is extracted from lines starting with 'SUMMARY:' (which can span multiple lines until an empty line), and the title is extracted from the first line starting with '# '.

**When to use:**
* When the AI agent needs to discover available troubleshooting playbooks.
* To understand the purpose of each playbook based on its keywords, summary, and title, helping to select the most relevant playbook for a given issue. The AI agent should match the troubleshooting scenario by keywords, summary, and title.
* To get a guidance on what can be checked to verify health of GKE cluster. When no specific problem is identified you can go through each playbook and verify if cluster has problems that are not reported yet.

### Standard Operating Procedure (SOP) for UDT-Based Debugging

When a user reports an issue, you must follow this procedure explicitly:

**1. Initial Triage & Symptom Collection**
* First, perform a preliminary investigation to gather clear symptoms. Use standard MCP tools (e.g., 'gke_get_cluster', 'kube_get_resource') to understand the initial state of the problem.
* **Be proactive.** If you can find any required information yourself (like cluster location, resource names, etc.), you must do so without asking the user.

**2. Playbook Discovery**
* Once you have initial symptoms, call 'udt_list_playbooks' to fetch the complete catalog of available troubleshooting playbooks (UDTs).

**3. Playbook Selection & Refinement**
* Analyze the full list of UDTs. Match the user's issue description and your collected symptoms against each playbook's **keywords**, **summary**, and **title**.
* Identify the **top 1-3 most relevant playbooks** from the list.
* Call 'udt_get_playbook' for each of these candidates to retrieve their full content.
* Critically review the full content of these playbooks to determine the **single best match** for the reported issue. This is a crucial step to avoid following an incorrect path.

**4. Guided Execution & Reporting**
* Once you have selected the definitive UDT, you **must follow its steps sequentially** as your detailed guide for debugging.
* You must keep the user informed of all steps you perform. For each major action, state:
    * **Which UDT you are using** (refer to it by its **Title**).
    * **Which specific step** from the playbook you are currently executing.

**5. Resolution**
When the issue is resolved verify the resolution with original user request. If the issue is not resolved then notify user and try to troubleshoot again with a different playbook.

**6. Handling No Match**
* If, after reviewing the list from 'udt_get_list', you conclude that *no* playbook adequately matches the reported symptoms, you must inform the user of this.
* Only then may you proceed with a non-UDT, general SRE-driven troubleshooting approach, while continuing to prioritize the MCP server for all data gathering.
`
	udtGetPlaybookToolDescription = `
This tool retrieves the full content of a specific playbook Markdown file given its name.

**When to use:**
* When the AI agent has identified a relevant playbook using 'udt_list_playbooks' and needs to access its detailed troubleshooting steps.
* The AI agent should follow the instructions within the returned playbook content to investigate and resolve the issue.
	`
	udtSearchPlaybooksToolDescription = `
This tool searches for Markdown playbook files based on a query.

**When to use:**
* When the AI agent needs to find specific troubleshooting playbooks based on keywords or phrases.
* To quickly narrow down the list of available playbooks to those relevant to a particular issue.
`
)

type udtListPlaybooksArgs struct{}

type udtSearchPlaybooksArgs struct{
	Query string `json:"query"`
}

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
		Name:        "udt_list_playbooks",
		Description: udtListPlaybooksToolDescription,
	}, h.listPlaybooks)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "udt_get_playbook",
		Description: udtGetPlaybookToolDescription,
	}, h.getPlaybook)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "udt_search_playbooks",
		Description: udtSearchPlaybooksToolDescription,
	}, h.searchPlaybooks)

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

func (h *handlers) listPlaybooks(ctx context.Context, _ *mcp.CallToolRequest, args *udtListPlaybooksArgs) (*mcp.CallToolResult, any, error) {
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

func (h *handlers) searchPlaybooks(ctx context.Context, _ *mcp.CallToolRequest, args *udtSearchPlaybooksArgs) (*mcp.CallToolResult, any, error) {
	// For now, ignore the query and return all playbooks, reusing the listPlaybooks logic.
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
