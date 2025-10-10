// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package install

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func GeminiCLIExtension(opts *InstallOptions) error {

	contextFilename := "GEMINI.md"
	// In developer mode, we use the GEMINI.md file directly from the repo.
	if opts.developerMode {
		if strings.HasPrefix(opts.exePath, os.TempDir()) {
			return fmt.Errorf("cannot install in developer mode using `go run`. Try again using `go build` and `./kubeapi-mcp`")
		}
		log.Printf("version: %s", opts.version)
		contextFilename = filepath.Join(filepath.Dir(opts.exePath), "pkg", "install", "GEMINI.md")
		if _, err := os.ReadFile(contextFilename); err != nil {
			return fmt.Errorf("could not read context file from %s: %w", contextFilename, err)
		}
	}

	extensionDir := filepath.Join(opts.installDir, ".gemini", "extensions", "kubeapi-mcp")
	if err := os.MkdirAll(extensionDir, 0755); err != nil {
		return fmt.Errorf("could not create extension directory: %w", err)
	}

	// Create the manifest file as described in https://github.com/google-gemini/gemini-cli/blob/main/docs/extension.md.
	manifest := map[string]interface{}{
		"name":            "kubeapi-mcp",
		"version":         opts.version,
		"description":     "Enable MCP-compatible AI agents to interact with Kubernetes.",
		"contextFileName": contextFilename,
		"mcpServers": map[string]interface{}{
			"kubeapi": map[string]interface{}{
				"command": opts.exePath,
			},
		},
	}

	manifestPath := filepath.Join(extensionDir, "gemini-extension.json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal manifest.json: %w", err)
	}

	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		return fmt.Errorf("could not write manifest.json: %w", err)
	}

	// In developer mode we don't need to create the GEMINI.md file.
	if !opts.developerMode {
		geminiMdPath := filepath.Join(extensionDir, "GEMINI.md")
		if err := os.WriteFile(geminiMdPath, GeminiMarkdown, 0644); err != nil {
			return fmt.Errorf("could not write GEMINI.md: %w", err)
		}
	}

	return nil
}
