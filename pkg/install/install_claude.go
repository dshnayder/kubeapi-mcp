// pkg/install/claude.go
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
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ClaudeDesktopExtension installs the KubeAPI MCP Server into Claude Desktop settings
func ClaudeDesktopExtension(opts *InstallOptions) error {
	configPath, err := getClaudeDesktopConfigPath()
	if err != nil {
		return fmt.Errorf("could not determine Claude Desktop config path: %w", err)
	}

	// Ensure the directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("could not create Claude Desktop config directory: %w", err)
	}

	// Read existing configuration if it exists
	config := make(map[string]interface{})
	if data, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("could not parse existing Claude Desktop config: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("could not read Claude Desktop config: %w", err)
	}

	// Add or update the kubeapi-mcp server configuration
	mcpServers, ok := config["mcpServers"].(map[string]interface{})
	if !ok {
		// Handle the case where mcpServers does not exist or is not a map
		mcpServers = make(map[string]interface{})
		config["mcpServers"] = mcpServers
	}

	mcpServers["kubeapi-mcp"] = map[string]interface{}{
		"command": opts.exePath,
	}

	// Write the updated config back
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal Claude Desktop config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("could not write Claude Desktop config: %w", err)
	}

	return nil
}

// getClaudeDesktopConfigPath returns the platform-specific path to Claude Desktop's config file
func getClaudeDesktopConfigPath() (string, error) {
	var configDir string

	switch runtime.GOOS {
	case "darwin": // macOS
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		configDir = filepath.Join(homeDir, "Library", "Application Support", "Claude")
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("APPDATA environment variable not set")
		}
		configDir = filepath.Join(appData, "Claude")
	case "linux":
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		configDir = filepath.Join(homeDir, ".config", "Claude")
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	return filepath.Join(configDir, "claude_desktop_config.json"), nil
}

// ClaudeCodeExtension installs the KubeAPI MCP Server for Claude Code CLI
func ClaudeCodeExtension(opts *InstallOptions) error {
	installDir := opts.installDir
	claudeMDPath := filepath.Join(installDir, "CLAUDE.md")

	// Check if CLAUDE.md exists to determine the warning message
	_, err := os.Stat(claudeMDPath)
	exists := err == nil
	isNew := os.IsNotExist(err)

	// Ask for user confirmation to create/edit CLAUDE.md
	if exists {
		fmt.Println("Warning: CLAUDE.md already exists. The KubeAPI MCP usage instructions will be appended.")
	} else if isNew {
		fmt.Println("Note: CLAUDE.md does not exist. A new one will be created and the KubeAPI MCP usage instructions will be added.")
	} else {
		return fmt.Errorf("failed to check file status: %w", err)
	}

	fmt.Print("Would you like to proceed? (yes/no): ")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read user input: %w", err)
	}

	if strings.ToLower(strings.TrimSpace(response)) != "yes" {
		fmt.Println("Installation canceled.")
		return nil
	}

	// Create the KUBEAPI_MCP_USAGE_GUIDE.md file
	usageGuideMDPath := filepath.Join(installDir, "KUBEAPI_MCP_USAGE_GUIDE.md")
	if err := os.WriteFile(usageGuideMDPath, []byte(GeminiMarkdown), 0644); err != nil {
		return fmt.Errorf("could not create KUBEAPI_MCP_USAGE_GUIDE.md: %w", err)
	}
	fmt.Println("Created KUBEAPI_MCP_USAGE_GUIDE.md.")

	// Add the reference line with the actual path to CLAUDE.md
	claudeLine := fmt.Sprintf("\n# KubeAPI-MCP Server Instructions\n - @%s", usageGuideMDPath)

	file, err := os.OpenFile(claudeMDPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("could not open or create CLAUDE.md: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(claudeLine); err != nil {
		return fmt.Errorf("could not append to CLAUDE.md: %w", err)
	}
	fmt.Println("Added a reference to KUBEAPI_MCP_USAGE_GUIDE.md in CLAUDE.md.")

	// Execute the command to add the MCP server
	command := "claude"
	args := []string{
		"mcp",
		"add",
		"kubeapi-mcp",
		opts.exePath,
	}

	cmdToRun := exec.Command(command, args...)
	cmdToRun.Stdout = os.Stdout
	cmdToRun.Stderr = os.Stderr

	if err := cmdToRun.Run(); err != nil {
		return fmt.Errorf("failed to run command 'claude mcp add': %w", err)
	}

	return nil
}
