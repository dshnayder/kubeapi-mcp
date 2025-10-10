# Installing the KubeAPI MCP Server in Cursor

This guide provides detailed steps on how to install and configure the KubeAPI MCP Server for use with the Cursor IDE. This allows you to leverage Cursor's AI agent to interact with your Kubernetes clusters using natural language prompts.

## Prerequisites and Installation of the `kubeapi-mcp` Binary

The KubeAPI MCP Server is a command-line tool. You must have the binary installed on your system before configuring it in Cursor.

Please follow the [installation instructions in the main readme](../../README.md#install-the-mcp-server) to install the `kubeapi-mcp` binary.

## Installing `kubeapi-mcp` for Cursor via Command Line

The `kubeapi-mcp` tool provides a convenient command-line interface to automatically configure Cursor with the KubeAPI MCP Server. This method handles the configuration files and rule creation automatically.

### Basic Installation

```bash
# Install kubeapi-mcp globally for Cursor (creates ~/.cursor/mcp.json)
kubeapi-mcp install cursor
```

Or

```bash
# Install kubeapi-mcp project-only for Cursor (creates ./.cursor/mcp.json)
# Please run this in the root directory of your project
kubeapi-mcp install cursor --project-only
# or use the short form
kubeapi-mcp install cursor -p
```

### Installation Options

#### Global vs Project-Specific Installation

- **Global installation** (default): Creates configuration in your home directory (`~/.cursor/`)
  - Available across all projects
  - Configuration persists across system restarts
  - Use when you want the KubeAPI MCP Server available everywhere

- **Project-only installation**: Creates configuration in the current project directory (`./.cursor/`)
  - Only available in the current project
  - Configuration is version-controlled with your project
  - Use when you want project-specific KubeAPI MCP Server configuration

#### Command Line Flags

| Flag             | Short | Description                          | Example                     |
| ---------------- | ----- | ------------------------------------ | --------------------------- |
| `--project-only` | `-p`  | Install only for the current project | `kubeapi-mcp install cursor -p` |
| (no flag)        | -     | Install globally (default)           | `kubeapi-mcp install cursor`    |

### What the Installation Command Does

When you run `kubeapi-mcp install cursor`, it automatically:

1. **Creates the MCP configuration**: Generates the appropriate `mcp.json` file with the KubeAPI MCP Server configuration
2. **Sets up the rules directory**: Creates the `.cursor/rules/` directory structure
3. **Creates the Kubernetes rule**: Generates `kubeapi-mcp.mdc` with the necessary context and instructions
4. **Handles file paths**: Automatically determines the correct paths for global vs project-specific installation

## Install `kubeapi-mcp` for Cursor Manually

Cursor uses a JSON configuration file to manage its MCP servers. You must define your server in this file.

- **For global use:** Edit the global configuration file at `~/.cursor/mcp.json`.
- **For project-specific use:** Create a `.cursor/mcp.json` file in your project's root directory.

Add the following configuration snippet to your `mcp.json` file. If the file already exists, merge this into the `mcpServers` object.

```json
{
  "mcpServers": {
    "kubeapi-mcp": {
      "command": "kubeapi-mcp",
      "type": "stdio"
    }
  }
}
```

## Adapting Context from `GEMINI.md`

A key challenge in this integration is that the `kubeapi-mcp` tool relies on a `gemini.md` file for its system prompts. To avoid rewriting the core logic, we will adapt this file's functionality for Cursor by using the **"Rules"** system.

### Steps to Implement the Rule

1. **Create the Rule File**: Create a new file named `kubeapi-mcp.mdc` in your project's `.cursor/rules/` directory.

2. **Add Metadata**: Add the following metadata block to the top of the `kubeapi-mcp.mdc` file.

   ```markdown
   ---
   name: Kubernetes MCP Instructions
   description: Provides guidance for using the kubeapi-mcp tool.
   ---
   ```

3. **Copy Content**: Copy the entire content of the [`kubeapi-mcp`'s `GEMINI.md`](../../pkg/install/GEMINI.md) file into your `kubeapi-mcp.mdc` file, placing it directly below the metadata block.

This rule will be configured to be **Agent Requested** by default, allowing the AI to dynamically include the Kubernetes context in its prompts only when it's relevant to your conversation.

## Verification and Usage

### How to Verify the Connection

1. Restart Cursor after modifying the configuration.
2. Open **Settings** (`Ctrl + ,` or `Cmd + ,`).
3. Navigate to **Features \> MCP**. A green dot next to the "kubeapi-mcp" entry indicates a successful connection.

### Sample Usage in Cursor

Once connected, you can use natural language prompts in the Cursor chat to interact with your Kubernetes environment. For example:

- **Prompt:** "List all the Kubernetes clusters I have in the `us-central1` region."
- **Expected Behavior:** Cursor's AI will propose using the `list_clusters` tool. After your approval, it will execute the command and display the results in a readable format.
