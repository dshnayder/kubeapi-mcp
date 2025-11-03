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

package kubernetes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/template"
	"time"

	"cloud.google.com/go/logging/logadmin"
	"github.com/dmitryshnayder/kubeapi-mcp/pkg/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/api/iterator"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

// GetResourcesToolDescription contains the documentation for the Get Kubernetes Resources tool.
// It is formatted in Markdown.
const GetResourcesToolDescription = `
This tool retrieves one or more Kubernetes resources from the cluster's API server.

***

## What "Getting Resources" Means

In Kubernetes, "getting resources" means fetching the **live state and specifications** for all resources that match a specific type within a given scope (e.g., within a namespace or across the entire cluster). This is the equivalent of running a command like *kubectl get pods -n my-namespace*. If a name is specified, it will fetch a single resource, equivalent to *kubectl get pod my-pod*. The server returns a collection of complete object definitions.
## Response Format: A List of YAML Documents

The tool returns a list of resources, with each resource formatted as a complete **YAML** document. The list of YAML documents are concatenated together, separated by the standard YAML document separator (*---*).

For example, a response containing two Pods would look like this:

` + "```" + `yaml
apiVersion: v1
kind: Pod
metadata:
  name: pod-1
  namespace: default
spec:
  # ... pod spec ...
---
apiVersion: v1
kind: Pod
metadata:
  name: pod-2
  namespace: default
spec:
  # ... pod spec ...
` + "```" + `

***

## How to Specify Resources to Get

To get resources, you must specify the type you are interested in. The *getResourcesArgs* structure defines the necessary arguments.

` + "```" + `go
// The actual struct includes JSON tags. They are omitted here for clarity.
// Refer to the source code for the complete definition.
getResourcesArgs struct {
    Resource      string
    Name          string
    Namespace     string
    LabelSelector string
}
` + "```" + `

### Argument Breakdown

* *Resource*: The **plural, lowercase name** for the resource type (e.g., *pods*, *deployments*, *services*).
* *Name*: (Optional) The case-sensitive name of the specific resource instance you want to retrieve (e.g., *my-app-deployment*, *nginx-pod-123*). If omitted, all resources of the specified type will be returned.
* *Namespace*: (Optional) The namespace from which to list resources.
    * If you provide a namespace, the tool will only list resources from that specific namespace.
    * If this field is **omitted** for a namespaced resource type (like *Pods*), it will list resources from **all namespaces**.
    * For cluster-scoped resources (like *Nodes*), this field should be omitted.
* *LabelSelector*: (Optional) A Kubernetes label selector to filter the resources.

### Example

To list all *Services* in the *kube-system* namespace, you would structure the arguments like this:

* *Resource*: *"services"*
* *Namespace*: *"kube-system"*
`

// ApplyResourceToolDescription contains the documentation for the Apply Kubernetes Resource tool.
// It is formatted in Markdown.
const ApplyResourceToolDescription = `
This tool applies a configuration to a resource from a YAML manifest. If the resource doesn't exist, it will be created. If it already exists, it will be updated.

***

## What "Applying a Resource" Means

In Kubernetes, "applying" is a **declarative** operation that makes the live state of a resource in the cluster match the state defined in your configuration file (the manifest). This is the equivalent of running *kubectl apply -f <filename.yaml>*.

The Kubernetes API server receives the manifest and calculates the difference between your desired state and the current configuration of the resource. It then applies only the necessary changes to update the resource. This is the recommended way to manage Kubernetes objects.

***

## Manifest Format: YAML

The tool expects a single argument, *manifest*, which is a string containing a complete resource definition in **YAML** format. A valid manifest must include these top-level fields:

* **apiVersion**: The version of the Kubernetes API to use (e.g., *v1*, *apps/v1*).
* **kind**: The type of resource you want to create (e.g., *Pod*, *Deployment*, *ConfigMap*).
* **metadata**: Data that helps uniquely identify the object, including a *name* and optionally a *namespace*.
* **spec** or **data**: The desired state for the resource (e.g., container images for a *Deployment*, or key-value pairs for a *ConfigMap*).

***

## How to Use the Tool

To apply a resource, you provide the full YAML manifest as a single string.

` + "```" + `go
// The actual struct includes JSON tags. They are omitted here for clarity.
// Refer to the source code for the complete definition.
applyResourceArgs struct {
    Manifest string
}
` + "```" + `

### Response Format

The tool's response is the full YAML of the object **after** it has been applied to the cluster. This returned manifest will include server-populated fields like the *status* block and fields within *metadata* (*uid*, *resourceVersion*, etc.), confirming the result of the operation.

### Example

To create or update a *ConfigMap* named *my-config* in the *default* namespace, you would provide the following string as the *manifest* argument:

` + "```" + `yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-config
  namespace: default
data:
  database.url: "postgres.example.com"
  ui.theme: "dark"
` + "```" + `
`

// DeleteResourceToolDescription contains the documentation for the Delete Kubernetes Resource tool.
// It is formatted in Markdown.
const DeleteResourceToolDescription = `
This tool deletes a specific Kubernetes resource from the cluster.

***

## What "Deleting a Resource" Means

In Kubernetes, "deleting a resource" means sending a request to the API server to remove an object from the cluster. This is the equivalent of running the *kubectl delete* command.

When the API server receives the request, it initiates a **graceful termination** process. For a resource like a Pod, this involves signaling the containers to shut down, giving them time to finish their work before being forcefully terminated. Once the object is terminated, its definition is removed from etcd, the cluster's backing store. ⚠️ **This action is irreversible.**

***

## How to Specify a Resource to Delete

To delete a specific resource, you must provide its coordinates within the Kubernetes API. The *deleteResourceArgs* structure defines the necessary arguments to uniquely identify any resource for deletion.

` + "```" + `go
// The actual struct includes JSON tags. They are omitted here for clarity.
// Refer to the source code for the complete definition.
deleteResourceArgs struct {
    Resource  string
    Name      string
    Namespace string
}
` + "```" + `

### Argument Breakdown

* *Resource*: The **plural, lowercase name** for the resource type (e.g., *pods*, *deployments*, *secrets*).
* *Name*: The case-sensitive name of the specific resource instance you want to delete.
* *Namespace*: The namespace where the resource exists. This field must be provided for namespaced resources. For cluster-scoped resources like *Nodes*, it should be omitted.

### Response Format

Upon successful deletion, the tool returns a simple confirmation message in the following format:

` + "```" + `
Resource <resource-type>/<resource-name> deleted.
` + "```" + `

### Example

To delete a *Secret* named *api-keys* from the *production* namespace, you would structure the arguments like this:

* *Resource*: *"secrets"*
* *Name*: *"api-keys"*
* *Namespace*: *"production"*
`

// APIResourcesToolDescription contains the documentation for the List API Resources tool.
// It is formatted in Markdown.
const APIResourcesToolDescription = `
This tool lists the API resources available in the cluster. This is the equivalent of running *kubectl api-resources*.

***

## What "Listing API Resources" Means

In Kubernetes, "listing API resources" means querying the API server to discover all the resource types it supports. This includes core resources like *Pods* and *Services*, as well as Custom Resource Definitions (CRDs) that extend the Kubernetes API.

The tool returns a table that provides the following information for each resource:
* **NAME**: The plural, lowercase name of the resource (e.g., *pods*).
* **SHORTNAMES**: A comma-separated list of short names or aliases (e.g., *po* for *pods*).
* **APIVERSION**: The API group and version (e.g., *v1*, *apps/v1*).
* **NAMESPACED**: A boolean indicating whether the resource is namespaced (*true*) or cluster-scoped (*false*).
* **KIND**: The CamelCase name of the resource kind (e.g., *Pod*).
`

// GetPodLogsToolDescription contains the documentation for the Get Kubernetes Pod Logs tool.
// It is formatted in Markdown.
const GetPodLogsToolDescription = `
This tool retrieves logs from a specific pod in the cluster.

***

## What "Getting Pod Logs" Means

In Kubernetes, "getting pod logs" means fetching the output from the containers running within a pod. This is the equivalent of running a command like *kubectl logs my-pod -n my-namespace*.

The tool returns the raw log output as a string.

***

## How to Specify a Pod to Get Logs From

To get logs from a pod, you must specify the pod's name and namespace. You can also optionally specify a container name if the pod has multiple containers.

` + "```" + `go
// The actual struct includes JSON tags. They are omitted here for clarity.
// Refer to the source code for the complete definition.
getPodLogsArgs struct {
    Name      string
    Namespace string
    Container string
}
` + "```" + `

### Argument Breakdown

* *Name*: The case-sensitive name of the pod.
* *Namespace*: The namespace where the pod exists.
* *Container*: (Optional) The name of the container to get logs from. If omitted, and the pod has multiple containers, an error will be returned.

### Example

To get logs from a pod named *my-app-pod-123* in the *production* namespace, you would structure the arguments like this:

* *Name*: *"my-app-pod-123"*
* *Namespace*: *"production"*
`

// PatchResourceToolDescription contains the documentation for the Patch Kubernetes Resource tool.
// It is formatted in Markdown.
const PatchResourceToolDescription = `
This tool patches a specific Kubernetes resource from the cluster.

***

## What "Patching a Resource" Means

In Kubernetes, "patching a resource" means sending a request to the API server to partially update an object. This is the equivalent of running the *kubectl patch* command.

The tool supports three types of patches:
* **strategic-merge-patch**: The default. Merges the patch with the existing resource, following rules specific to each field.
* **merge-patch**: Merges the patch with the existing resource. For maps, the patch's keys and values are merged with the existing map. For lists, the patch's list replaces the existing list.
* **json-patch**: A JSON Patch is a sequence of operations (add, remove, replace, etc.) that are applied to a JSON document.

***

## How to Specify a Resource to Patch

To patch a specific resource, you must provide its coordinates within the Kubernetes API, the patch itself, and optionally the patch type. The *patchResourceArgs* structure defines the necessary arguments.

` + "```" + `go
// The actual struct includes JSON tags. They are omitted here for clarity.
// Refer to the source code for the complete definition.
patchResourceArgs struct {
    Resource  string
    Name      string
    Namespace string
    Patch     string
    PatchType string
}
` + "```" + `

### Argument Breakdown

* *Resource*: The **plural, lowercase name** for the resource type (e.g., *pods*, *deployments*, *secrets*).
* *Name*: The case-sensitive name of the specific resource instance you want to patch.
* *Namespace*: The namespace where the resource exists. This field must be provided for namespaced resources. For cluster-scoped resources like *Nodes*, it should be omitted.
* *Patch*: A YAML string representing the patch.
* *PatchType*: (Optional) The type of patch to apply. Can be *strategic*, *merge*, or *json*. Defaults to *strategic*.

### Response Format

The tool's response is the full YAML of the object **after** it has been patched.

### Example

To patch a *Deployment* named *my-app* in the *default* namespace to change the number of replicas, you would structure the arguments like this:

* *Resource*: *"deployments"*
* *Name*: *"my-app"*
* *Namespace*: *"default"*
* *Patch*: *'spec: {replicas: 3}'*
`

// CanIToolDescription contains the documentation for the Kubernetes Can I tool.
// It is formatted in Markdown.
const CanIToolDescription = `
This tool checks if the current user can perform a specific action on a Kubernetes resource. This is the equivalent of running *kubectl auth can-i*.

***

## What "Can I" Means

In Kubernetes, "can I" means checking if the current user's permissions (as defined by Roles and RoleBindings) allow them to perform a certain verb (like 'get', 'create', 'delete') on a specific resource (like 'pods', 'deployments') in a given namespace.

The tool returns a simple "yes" or "no" answer.

***

## How to Use the Tool

To use the tool, you must provide the verb and the resource you want to check.

` + "```" + `go
// The actual struct includes JSON tags. They are omitted here for clarity.
// Refer to the source code for the complete definition.
canIArgs struct {
    Verb         string
    Resource     string
    Subresource  string
    Name         string
    Namespace    string
}
` + "```" + `

### Argument Breakdown

* *Verb*: The action you want to check (e.g., 'get', 'list', 'watch', 'create', 'update', 'patch', 'delete').
* *Resource*: The plural, lowercase name for the resource type (e.g., 'pods', 'deployments', 'services').
* *Subresource*: (Optional) The subresource to check (e.g., 'log', 'status').
* *Name*: (Optional) The name of a specific resource instance to check.
* *Namespace*: (Optional) The namespace to check the action in.
`

type handlers struct {
	c              *config.Config
	dyn            dynamic.Interface
	mapper         meta.RESTMapper
	dc             *discovery.DiscoveryClient
	clientset      kubernetes.Interface
	logadminClient *logadmin.Client
}

func Install(ctx context.Context, s *mcp.Server, c *config.Config) error {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	restConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig: %w", err)
	}
	restConfig.Timeout = 30 * time.Second

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	dyn, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	dc, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create discovery client: %w", err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	logadminClient, err := logadmin.NewClient(ctx, c.DefaultProjectID())
	if err != nil {
		return fmt.Errorf("failed to create logadmin client: %w", err)
	}

	h := &handlers{
		c:              c,
		dyn:            dyn,
		mapper:         mapper,
		dc:             dc,
		clientset:      clientset,
		logadminClient: logadminClient,
	}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kubernetes_get_resources",
		Description: GetResourcesToolDescription,
	}, h.getResources)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kubernetes_api_resources",
		Description: APIResourcesToolDescription,
	}, h.apiResources)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kubernetes_get_pod_logs",
		Description: GetPodLogsToolDescription,
	}, h.getPodLogs)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kubernetes_can_i",
		Description: CanIToolDescription,
	}, h.canI)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kubernetes_query_logs",
		Description: QueryLogsToolDescription,
	}, h.queryLogs)

	if !c.ReadOnly() {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "kubernetes_apply_resource",
			Description: ApplyResourceToolDescription,
		}, h.applyResource)

		mcp.AddTool(s, &mcp.Tool{
			Name:        "kubernetes_delete_resource",
			Description: DeleteResourceToolDescription,
		}, h.deleteResource)

		mcp.AddTool(s, &mcp.Tool{
			Name:        "kubernetes_patch_resource",
			Description: PatchResourceToolDescription,
		}, h.patchResource)

	}
	return nil
}

func (h *handlers) canI(ctx context.Context, _ *mcp.CallToolRequest, args *canIArgs) (*mcp.CallToolResult, any, error) {
	sar := &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Verb:        args.Verb,
				Resource:    args.Resource,
				Subresource: args.Subresource,
				Name:        args.Name,
				Namespace:   args.Namespace,
			},
		},
	}

	response, err := h.clientset.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, sar, metav1.CreateOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create self subject access review: %w", err)
	}

	var result string
	if response.Status.Allowed {
		result = "yes"
	} else {
		result = "no"
		if response.Status.Reason != "" {
			result += fmt.Sprintf(" (reason: %s)", response.Status.Reason)
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: result},
		},
	}, nil, nil
}

// QueryLogsToolDescription contains the documentation for the Query Google Cloud Platform Logs tool.
// It is formatted in Markdown.
const QueryLogsToolDescription = `
Query Google Cloud Platform logs using Logging Query Language (LQL). Before using this tool, it's **strongly** recommended to call the 'get_log_schema' tool to get information about supported log types and their schemas. Logs are returned in ascending order, based on the timestamp (i.e. oldest first).

***

## How to Query Logs

To query logs, you must provide an LQL query string and the GCP project ID. You can also specify an output format, a limit on the number of entries, a relative time duration, or an explicit time range.

` + "```" + `go
// The actual struct includes JSON tags. They are omitted here for clarity.
// Refer to the source code for the complete definition.
queryLogsArgs struct {
	Query     string
	ProjectID string
	Format    string
	Limit     int
	Since     string
	TimeRange *queryLogsTimeRangeArgs
}

queryLogsTimeRangeArgs struct {
	StartTime string
	EndTime   string
}
` + "```" + `

### Argument Breakdown

* *Query*: LQL query string to filter and retrieve log entries. Don't specify time ranges in this filter. Use 'time_range' instead.
* *ProjectID*: GCP project ID to query logs from. Required.
* *Format*: Go template string to format each log entry. If empty, the full JSON representation is returned. Note that empty fields are not included in the response. Example: '{{.timestamp}} [{{.severity}}] {{.textPayload}}'. It's strongly recommended to use a template to minimize the size of the response and only include the fields you need. Use the get_schema tool before this tool to get information about supported log types and their schemas.
* *Limit*: Maximum number of log entries to return. Cannot be greater than 100. Consider multiple calls if needed. Defaults to 10.
* *Since*: Only return logs newer than a relative duration like 5s, 2m, or 3h. The only supported units are seconds ('s'), minutes ('m'), and hours ('h').
* *TimeRange*: Time range for log query. If empty, no restrictions are applied.
    * *StartTime*: Start time for log query (RFC3339 format)
    * *EndTime*: End time for log query (RFC3339 format)
`
type canIArgs struct {
	Verb        string `json:"verb"`
	Resource    string `json:"resource"`
	Subresource string `json:"subresource,omitempty"`
	Name        string `json:"name,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
}

type queryLogsArgs struct {
	Query     string `json:"query"`
	ProjectID string `json:"project_id"`
	Format    string `json:"format,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Since     string `json:"since,omitempty"`
	TimeRange *queryLogsTimeRangeArgs `json:"time_range,omitempty"`
}

type queryLogsTimeRangeArgs struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}


type getResourcesArgs struct {
	Resource      string `json:"resource"`
	Name          string `json:"name,omitempty"`
	Namespace     string `json:"namespace,omitempty"`
	LabelSelector string `json:"labelSelector,omitempty"`
}

func (h *handlers) getResources(ctx context.Context, _ *mcp.CallToolRequest, args *getResourcesArgs) (*mcp.CallToolResult, any, error) {
	gvr, err := h.findGVR(args.Resource)
	if err != nil {
		return nil, nil, err
	}
	var resources []unstructured.Unstructured

	if args.Name != "" {
		var obj *unstructured.Unstructured
		var err error
		if args.Namespace != "" {
			obj, err = h.dyn.Resource(gvr).Namespace(args.Namespace).Get(ctx, args.Name, metav1.GetOptions{})
		} else {
			obj, err = h.dyn.Resource(gvr).Get(ctx, args.Name, metav1.GetOptions{})
		}
		if err != nil {
			return nil, nil, err
		}
		resources = append(resources, *obj)
	} else {
		var list *unstructured.UnstructuredList
		var err error
		listOptions := metav1.ListOptions{}
		if args.LabelSelector != "" {
			listOptions.LabelSelector = args.LabelSelector
		}
		if args.Namespace != "" {
			list, err = h.dyn.Resource(gvr).Namespace(args.Namespace).List(ctx, listOptions)
		} else {
			list, err = h.dyn.Resource(gvr).List(ctx, listOptions)
		}
		if err != nil {
			return nil, nil, err
		}
		resources = list.Items
	}

	var yamlDocs []string
	for _, item := range resources {
		// Convert Unstructured to JSON
		jsonData, err := json.Marshal(item.Object)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal resource to JSON: %w", err)
		}

		// Convert JSON to YAML
		yamlData, err := yaml.JSONToYAML(jsonData)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to convert JSON to YAML: %w", err)
		}
		yamlDocs = append(yamlDocs, string(yamlData))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: strings.Join(yamlDocs, "---\n")},
		},
	}, nil, nil
}

type applyResourceArgs struct {
	Manifest string `json:"manifest"`
}

func (h *handlers) applyResource(ctx context.Context, _ *mcp.CallToolRequest, args *applyResourceArgs) (*mcp.CallToolResult, any, error) {
	yamlParts := strings.Split(args.Manifest, "---")
	var appliedYamls []string

	for _, part := range yamlParts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Convert YAML manifest to JSON
		jsonData, err := yaml.YAMLToJSON([]byte(part))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to convert manifest from YAML to JSON: %w", err)
		}

		// Unmarshal JSON into Unstructured object
		var obj unstructured.Unstructured
		if err := obj.UnmarshalJSON(jsonData); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
		}

		gvk := obj.GroupVersionKind()
		mapping, err := h.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get REST mapping: %w", err)
		}
		gvr := mapping.Resource
		namespace := obj.GetNamespace()
		name := obj.GetName()

		var appliedObj *unstructured.Unstructured
		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			appliedObj, err = h.dyn.Resource(gvr).Namespace(namespace).Apply(ctx, name, &obj, metav1.ApplyOptions{FieldManager: "kubeapi-mcp", Force: true})
		} else {
			appliedObj, err = h.dyn.Resource(gvr).Apply(ctx, name, &obj, metav1.ApplyOptions{FieldManager: "kubeapi-mcp", Force: true})
		}

		if err != nil {
			return nil, nil, err
		}

		// Convert Unstructured to JSON for YAML conversion
		appliedJson, err := json.Marshal(appliedObj.Object)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal resource to JSON: %w", err)
		}

		// Convert JSON to YAML
		yamlData, err := yaml.JSONToYAML(appliedJson)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to convert JSON to YAML: %w", err)
		}
		appliedYamls = append(appliedYamls, string(yamlData))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: strings.Join(appliedYamls, "---\n")},
		},
	}, nil, nil
}

type deleteResourceArgs struct {
	Resource  string `json:"resource"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

func (h *handlers) deleteResource(ctx context.Context, _ *mcp.CallToolRequest, args *deleteResourceArgs) (*mcp.CallToolResult, any, error) {
	gvr, err := h.findGVR(args.Resource)
	if err != nil {
		return nil, nil, err
	}
	if args.Namespace != "" {
		err = h.dyn.Resource(gvr).Namespace(args.Namespace).Delete(ctx, args.Name, metav1.DeleteOptions{})
	} else {
		err = h.dyn.Resource(gvr).Delete(ctx, args.Name, metav1.DeleteOptions{})
	}
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Resource %s/%s deleted.", args.Resource, args.Name)},
		},
	}, nil, nil
}

type apiResourcesArgs struct{}

func (h *handlers) apiResources(ctx context.Context, _ *mcp.CallToolRequest, args *apiResourcesArgs) (*mcp.CallToolResult, any, error) {
	_, resourceLists, err := h.dc.ServerGroupsAndResources()
	if err != nil {
		if _, ok := err.(*discovery.ErrGroupDiscoveryFailed); !ok {
			return nil, nil, fmt.Errorf("failed to get server groups and resources: %w", err)
		}
	}

	var output strings.Builder
	output.WriteString("NAME\tSHORTNAMES\tAPIVERSION\tNAMESPACED\tKIND\n")

	for _, list := range resourceLists {
		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil {
			continue
		}
		for _, resource := range list.APIResources {
			output.WriteString(fmt.Sprintf("%s\t%s\t%s\t%t\t%s\n",
				resource.Name,
				strings.Join(resource.ShortNames, ","),
				gv.String(),
				resource.Namespaced,
				resource.Kind,
			))
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: output.String()},
		},
	}, nil, nil
}

type getPodLogsArgs struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Container string `json:"container,omitempty"`
}

func (h *handlers) getPodLogs(ctx context.Context, _ *mcp.CallToolRequest, args *getPodLogsArgs) (*mcp.CallToolResult, any, error) {
	podLogOpts := &corev1.PodLogOptions{
		Container: args.Container,
	}
	req := h.clientset.CoreV1().Pods(args.Namespace).GetLogs(args.Name, podLogOpts)
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get pod logs: %w", err)
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read pod logs: %w", err)
	}
	logs := buf.String()

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: logs},
		},
	}, nil, nil
}

type patchResourceArgs struct {
	Resource  string `json:"resource"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Patch     string `json:"patch"`
	PatchType string `json:"patchType,omitempty"`
}

func (h *handlers) patchResource(ctx context.Context, _ *mcp.CallToolRequest, args *patchResourceArgs) (*mcp.CallToolResult, any, error) {
	gvr, err := h.findGVR(args.Resource)
	if err != nil {
		return nil, nil, err
	}

	patchType := types.StrategicMergePatchType
	switch args.PatchType {
	case "json":
		patchType = types.JSONPatchType
	case "merge":
		patchType = types.MergePatchType
	case "strategic":
		patchType = types.StrategicMergePatchType
	case "":
		// Do nothing, use default
	default:
		return nil, nil, fmt.Errorf("invalid patch type %q", args.PatchType)
	}

	// Convert YAML patch to JSON
	patchBytes, err := yaml.YAMLToJSON([]byte(args.Patch))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert patch from YAML to JSON: %w", err)
	}

	var patchedObj *unstructured.Unstructured
	if args.Namespace != "" {
		patchedObj, err = h.dyn.Resource(gvr).Namespace(args.Namespace).Patch(ctx, args.Name, patchType, patchBytes, metav1.PatchOptions{})
	} else {
		patchedObj, err = h.dyn.Resource(gvr).Patch(ctx, args.Name, patchType, patchBytes, metav1.PatchOptions{})
	}
	if err != nil {
		return nil, nil, err
	}

	// Convert Unstructured to JSON for YAML conversion
	jsonData, err := json.Marshal(patchedObj.Object)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal resource to JSON: %w", err)
	}

	// Convert JSON to YAML
	yamlData, err := yaml.JSONToYAML(jsonData)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert JSON to YAML: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(yamlData)},
		},
	}, nil, nil
}

func (h *handlers) queryLogs(ctx context.Context, _ *mcp.CallToolRequest, args *queryLogsArgs) (*mcp.CallToolResult, any, error) {
	filter := args.Query
	if args.Since != "" {
		d, err := time.ParseDuration(args.Since)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid since duration: %w", err)
		}
		filter += fmt.Sprintf(` timestamp >= "%s"`, time.Now().Add(-d).Format(time.RFC3339))
	}
	if args.TimeRange != nil {
		filter += fmt.Sprintf(` timestamp >= "%s" AND timestamp <= "%s"`, args.TimeRange.StartTime, args.TimeRange.EndTime)
	}

	it := h.logadminClient.Entries(ctx, logadmin.Filter(filter))
	var result strings.Builder
	limit := 10
	if args.Limit > 0 {
		limit = args.Limit
	}

	tmpl, err := template.New("log").Parse(args.Format)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid format template: %w", err)
	}

	for i := 0; i < limit; i++ {
		entry, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get next log entry: %w", err)
		}

		if args.Format != "" {
			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, entry); err != nil {
				return nil, nil, fmt.Errorf("failed to execute template: %w", err)
			}
			result.WriteString(buf.String())
		} else {
			b, err := json.Marshal(entry)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to marshal log entry: %w", err)
			}
			result.Write(b)
		}
		result.WriteString("\n")
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: result.String()},
		},
	}, nil, nil
}

func (h *handlers) findGVR(resourceKind string) (schema.GroupVersionResource, error) {
	lists, err := h.dc.ServerPreferredResources()
	if err != nil {
		if _, ok := err.(*discovery.ErrGroupDiscoveryFailed); !ok {
			return schema.GroupVersionResource{}, fmt.Errorf("failed to get server preferred resources: %w", err)
		}
	}

	for _, list := range lists {
		for _, resource := range list.APIResources {
			if resource.Kind == resourceKind || resource.Name == resourceKind || resource.SingularName == resourceKind || contains(resource.ShortNames, resourceKind) {
				gv, err := schema.ParseGroupVersion(list.GroupVersion)
				if err != nil {
					return schema.GroupVersionResource{}, fmt.Errorf("failed to parse group version %q: %w", list.GroupVersion, err)
				}
				return gv.WithResource(resource.Name), nil
			}
		}
	}

	return schema.GroupVersionResource{}, fmt.Errorf("resource kind %q not found", resourceKind)
}

func contains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
