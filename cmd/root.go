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

package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime/debug"

	"github.com/dmitryshnayder/kubeapi-mcp/pkg/config"
	"github.com/dmitryshnayder/kubeapi-mcp/pkg/install"
	"github.com/dmitryshnayder/kubeapi-mcp/pkg/prompts"
	"github.com/dmitryshnayder/kubeapi-mcp/pkg/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

const (
	geminiInstructionsURI = "mcp://kubeapi/pkg/install/GEMINI.md"
)

var (
	version = "(unknown)"

	// command flags
	serverMode string
	serverPort int

	// rootCmd represents the base command when called without any subcommands
	rootCmd = &cobra.Command{
		Use:   "kubeapi-mcp",
		Short: "An MCP Server for Kubernetes",
		Run:   runRootCmd,
	}

	installCmd = &cobra.Command{
		Use:   "install",
		Short: "Install the KubeAPI MCP Server into your AI tool settings.",
	}

	installGeminiCLICmd = &cobra.Command{
		Use:   "gemini-cli",
		Short: "Install the KubeAPI MCP Server into your Gemini CLI settings.",
		Run:   runInstallGeminiCLICmd,
	}

	installCursorCmd = &cobra.Command{
		Use:   "cursor",
		Short: "Install the KubeAPI MCP Server into your Cursor settings.",
		Run:   runInstallCursorCmd,
	}

	installClaudeDesktopCmd = &cobra.Command{
		Use:   "claude-desktop",
		Short: "Install the KubeAPI MCP Server into your Claude Desktop settings.",
		Run:   runInstallClaudeDesktopCmd,
	}

	installClaudeCodeCmd = &cobra.Command{
		Use:   "claude-code",
		Short: "Install the KubeAPI MCP Server into your Claude Code CLI settings.",
		Run:   runInstallClaudeCodeCmd,
	}

	installDeveloper   bool
	installProjectOnly bool
)

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	if bi, ok := debug.ReadBuildInfo(); ok {
		version = bi.Main.Version
	} else {
		log.Printf("Failed to read build info to get version.")
	}

	rootCmd.Flags().StringVar(&serverMode, "server-mode", "stdio", "transport to use for the server: stdio (default) or http")
	rootCmd.Flags().IntVar(&serverPort, "server-port", 8080, "server port to use when server-mode is http; defaults to 8080")
	rootCmd.AddCommand(installCmd)

	installCmd.AddCommand(installGeminiCLICmd)
	installCmd.AddCommand(installCursorCmd)
	installCmd.AddCommand(installClaudeDesktopCmd)
	installCmd.AddCommand(installClaudeCodeCmd)

	installGeminiCLICmd.Flags().BoolVarP(&installDeveloper, "developer", "d", false, "Install the MCP Server in developer mode for Gemini CLI")
	installGeminiCLICmd.Flags().BoolVarP(&installProjectOnly, "project-only", "p", false, "Install the MCP Server only for the current project. Please run this in the root directory of your project")

	installCursorCmd.Flags().BoolVarP(&installProjectOnly, "project-only", "p", false, "Install the MCP Server only for the current project. Please run this in the root directory of your project")
	installClaudeCodeCmd.Flags().BoolVarP(&installProjectOnly, "project-only", "p", false, "Install the MCP Server only for the current project. Please run this in the root directory of your project")
}

type startOptions struct {
	serverMode string
	serverPort int
}

func runRootCmd(cmd *cobra.Command, args []string) {
	opts := startOptions{
		serverMode: serverMode,
		serverPort: serverPort,
	}
	startMCPServer(cmd.Context(), opts)
}

func startMCPServer(ctx context.Context, opts startOptions) {
	c := config.New(version)

	instructions := ""

	s := mcp.NewServer(
		&mcp.Implementation{
			Name:    "KubeAPI MCP Server",
			Version: version,
		},
		&mcp.ServerOptions{
			Instructions: instructions,
			HasTools:     true,
			HasResources: true,
		},
	)

	resource := &mcp.Resource{
		URI:         geminiInstructionsURI,
		Name:        "GEMINI.md",
		Description: "Instructions for how to use the KubeAPI MCP server",
		MIMEType:    "text/markdown",
	}

	s.AddResource(resource, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				&mcp.ResourceContents{
					URI:      geminiInstructionsURI,
					MIMEType: "text/markdown",
					Text:     string(install.GeminiMarkdown),
				},
			},
		}, nil
	})

	if err := prompts.Install(ctx, s, c); err != nil {
		log.Fatalf("Failed to install prompts: %v\n", err)
	}

	if err := tools.Install(ctx, s, c); err != nil {
		log.Fatalf("Failed to install tools: %v\n", err)
	}

	// start server in the right mode
	log.Printf("Starting KubeAPI MCP Server (%s) in mode '%s'", version, opts.serverMode)
	var err error
	endpoint := fmt.Sprintf(":%d", opts.serverPort)

	switch opts.serverMode {
	case "stdio":
		tr := &mcp.LoggingTransport{Transport: &mcp.StdioTransport{}, Writer: log.Writer()}
		err = s.Run(ctx, tr)
	case "http":
		handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
			return s
		}, nil)
		log.Printf("Listening for HTTP connections on port: %d", opts.serverPort)
		err = http.ListenAndServe(endpoint, handler)
	default:
		log.Printf("Unknown mode '%s', defaulting to 'stdio'", opts.serverMode)
		tr := &mcp.LoggingTransport{Transport: &mcp.StdioTransport{}, Writer: log.Writer()}
		err = s.Run(ctx, tr)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			log.Printf("Server shutting down.")
		} else {
			log.Printf("Server error: %v\n", err)
		}
	}
}

func installOptions() (*install.InstallOptions, error) {
	return install.NewInstallOptions(
		version,
		installProjectOnly,
		installDeveloper,
	)
}

func runInstallGeminiCLICmd(cmd *cobra.Command, args []string) {
	opts, err := installOptions()
	if err != nil {
		log.Fatalf("Failed to get install options: %v", err)
	}

	if err := install.GeminiCLIExtension(opts); err != nil {
		log.Fatalf("Failed to install for gemini-cli: %v", err)
	}
	fmt.Println("Successfully installed KubeAPI MCP server as a gemini-cli extension.")
}

func runInstallCursorCmd(cmd *cobra.Command, args []string) {
	opts, err := installOptions()
	if err != nil {
		log.Fatalf("Failed to get install options: %v", err)
	}

	if err := install.CursorMCPExtension(opts); err != nil {
		log.Fatalf("Failed to install for cursor: %v", err)
	}
	fmt.Println("Successfully installed KubeAPI MCP server as a cursor MCP server.")
}

func runInstallClaudeDesktopCmd(cmd *cobra.Command, args []string) {
	opts, err := installOptions()
	if err != nil {
		log.Fatalf("Failed to get install options: %v", err)
	}

	if err := install.ClaudeDesktopExtension(opts); err != nil {
		log.Fatalf("Failed to install for Claude Desktop: %v", err)
	}
	fmt.Println("Successfully installed KubeAPI MCP server in Claude Desktop configuration.")
}

func runInstallClaudeCodeCmd(cmd *cobra.Command, args []string) {
	opts, err := installOptions()
	if err != nil {
		log.Fatalf("Failed to get install options: %v", err)
	}

	if err := install.ClaudeCodeExtension(opts); err != nil {
		log.Fatalf("Failed to install for Claude Code: %v", err)
	}

	fmt.Println("Successfully installed KubeAPI MCP server for Claude Code.")
}
