# GKE MCP Extension for Gemini CLI

This document provides instructions for an AI agent on how to use the available tools to manage Google Kubernetes Engine (GKE) resources.

## Guiding Principles

- **Prefer Native Tools:** Always prefer to use the tools provided by this extension (e.g., `kubernetes_get_resources`) instead of shelling out to `gcloud` or `kubectl` for the same functionality. This ensures better-structured data and more reliable execution.
- **Clarify Ambiguity:** Do not guess or assume values for required parameters like cluster names or locations. If the user's request is ambiguous, ask clarifying questions to confirm the exact resource they intend to interact with.
- **Use Defaults:** If a `project_id` is not specified by the user, you can use the default value configured in the environment.

## Authentication

Some MCP tools required [Application Default Credentials](https://cloud.google.com/docs/authentication/application-default-credentials). If they return an "Unauthenticated" error, tell the user to run `gcloud auth application-default login` and try again. This is an interactive command and must be run manually outside the AI.

## Kubernetes Tools

### kubernetes_get_resources

This tool retrieves one or more Kubernetes resources from the cluster's API server. It is the equivalent of running `kubectl get`.

**When to use:**
- When the user asks to "get", "list", "show", or "describe" a Kubernetes resource.
- To check the status or configuration of a resource.

### kubernetes_api_resources

This tool lists the API resources available in the cluster. This is the equivalent of running `kubectl api-resources`.

**When to use:**
- When the user wants to know what resources are available in the cluster.
- To find the short name or API group for a resource.

### kubernetes_get_pod_logs

This tool retrieves logs from a specific pod in the cluster. This is the equivalent of running `kubectl logs`.

**When to use:**
- When the user asks for the logs of a pod.
- To troubleshoot a pod that is not behaving as expected.

### kubernetes_can_i

This tool checks if the current user can perform a specific action on a Kubernetes resource. This is the equivalent of running `kubectl auth can-i`.

**When to use:**
- When a user is having permission issues.
- To check if a user has the necessary permissions to perform an action before attempting it.

### kubernetes_query_logs

This tool queries Google Cloud Platform logs using Logging Query Language (LQL).

**When to use:**
- When the user wants to search for specific log entries.
- To troubleshoot issues that may be reflected in the logs.

### kubernetes_apply_resource

This tool applies a configuration to a resource from a YAML manifest. If the resource doesn't exist, it will be created. If it already exists, it will be updated. This is the equivalent of running `kubectl apply -f`.

**When to use:**
- When a user wants to create or update a resource from a YAML manifest.

### kubernetes_delete_resource

This tool deletes a specific Kubernetes resource from the cluster. This is the equivalent of running `kubectl delete`.

**When to use:**
- When a user wants to delete a resource.

### kubernetes_patch_resource

This tool patches a specific Kubernetes resource from the cluster. This is the equivalent of running `kubectl patch`.

**When to use:**
- When a user wants to perform a partial update of a resource.