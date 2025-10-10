# KubeAPI MCP Server and Gemini CLI Extension

Enable MCP-compatible AI agents to interact with Kubernetes.

<img src="https://raw.githubusercontent.com/dmitryshnayder/kubeapi-mcp/main/assets/kubeapi-mcp-gemini-cli-demo.gif" alt="A demonstration of using the KubeAPI MCP server with the Gemini CLI" width="600">

## Installation

Choose a way to install the MCP Server and then connect your AI to it.

### Use as a Gemini CLI Extension

1. Install [Gemini CLI](https://github.com/google-gemini/gemini-cli?tab=readme-ov-file#-installation).

2. Install the extension

```sh
gemini extensions install https://github.com/dmitryshnayder/kubeapi-mcp.git
```

### Use in MCP Clients / Other AIs

#### Quick Install (Linux & macOS only)

```sh
curl -sSL https://raw.githubusercontent.com/dmitryshnayder/kubeapi-mcp/main/install.sh | bash
```

#### Manual Install

If you haven't already installed Go, follow [these instructions](https://go.dev/doc/install).

Once Go is installed, run the following command to install kubeapi-mcp:

```sh
go install github.com/dmitryshnayder/kubeapi-mcp@latest
```

The `kubeapi-mcp` binary will be installed in the directory specified by the `GOBIN` environment variable. If `GOBIN` is not set, it defaults to `$GOPATH/bin` and, if `GOPATH` is also not set, it falls back to `$HOME/go/bin`.

You can find the exact location by running `go env GOBIN`. If the command returns an empty value, run `go env GOPATH` to find the installation directory.

For additional help, refer to the troubleshoot section: [kubeapi-mcp: command not found](TROUBLESHOOTING.md#kubeapi-mcp-command-not-found-on-macos-or-linux).

### Add the MCP Server to your AI

For detailed instructions on how to connect the KubeAPI MCP Server to various AI clients, including cursor and claude desktop, please refer to our dedicated [installation guide](docs/installation_guide/).

## MCP Tools

- `kubernetes_get_resource`: Get a Kubernetes resource.
- `kubernetes_list_resources`: List Kubernetes resources.
- `kubernetes_apply_resource`: Apply a Kubernetes resource.
- `kubernetes_delete_resource`: Delete a Kubernetes resource.

## MCP Context

In addition to the tools above, a lot of value is provided through the bundled context instructions.

- **Cost**: The provided instructions allows the AI to answer many questions related to Kubernetes costs, including queries related to clusters, namespaces, and Kubernetes workloads.

- **Kubernetes Known Issues**: The provided instructions allows the AI to fetch the latest Kubernetes Known issues and check whether the cluster is affected by one of these known issues.

## Supported MCP Transports

By default, `kubeapi-mcp` uses the [stdio]("https://modelcontextprotocol.io/specification/2025-06-18/basic/transports#stdio") transport. Additionally, the [Streamable HTTP](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports#streamable-http) transport is supported as well.

You can set the transport mode using the following options:

`--server-mode`: transport to use for the server: stdio (default) or http

`--server-port`: server port to use when server-mode is http or sse; defaults to 8080

```sh
kubeapi-mcp --server-mode http --server-port 8080
```

> [!WARNING]
> When using the `Streamable HTTP` transport, the server listens on all network interfaces (e.g., `0.0.0.0`), which can expose it to any network your machine is connected to.
> Please ensure you have a firewall ad/or other security measures in place to restrict access if the server is not intended to be public.

### Connecting Gemini CLI to the HTTP Server

To connect Gemini CLI to the `kubeapi-mcp` HTTP server, you need to configure the CLI to point to the correct endpoint. You can do this by updating your `~/.gemini/settings.json` file. For a basic setup without authentication, the file should look like this:

```json
{
  "mcpServers": {
    "kubeapi": {
      "httpUrl": "http://127.0.0.1:8080/mcp"
    }
  }
}
```

This configuration tells Gemini CLI how to reach the kubeapi-mcp server running on your local machine at port 8080.

## Development

To compile the binary and update the `gemini-cli` extension with your local changes, follow these steps:

1. Remove the global kubeapi-mcp configuration

   ```sh
   rm -rf ~/.gemini/extensions/kubeapi-mcp
   ```

1. Build the binary from the root of the project:

   ```sh
   go build -o kubeapi-mcp .
   ```

1. Run the installation command to update the extension manifest:

   ```sh
   ./kubeapi-mcp install gemini-cli --developer
   ```

   This will make `gemini-cli` use your locally compiled binary.
