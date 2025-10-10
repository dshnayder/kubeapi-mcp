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
	"context"
	"encoding/json"
	"fmt"

	"github.com/dmitryshnayder/kubeapi-mcp/pkg/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

// GetResourceToolDescription contains the documentation for the Get Kubernetes Resource tool.
// It is formatted in Markdown.
const GetResourceToolDescription = `
This tool retrieves a specific Kubernetes resource from the cluster's API server.

***

## What "Getting a Resource" Means

In Kubernetes, "getting a resource" means fetching its **live state and specification** from the API server. This is the equivalent of running the *kubectl get* command. The server returns the complete object definition, which includes its desired state (the *spec*) and its current, observed state (the *status*). This allows you to inspect the configuration and current health of any resource within the cluster.

## Resource Format: YAML

The resource definition is returned in **YAML (YAML Ain't Markup Language)** format. YAML is a human-readable data serialization standard that Kubernetes uses to define all its objects. It uses indentation to represent data structure, making it easy to read and manage complex configurations.

For example, a simple Pod definition in YAML looks like this:

` + "```" + `yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-pod
  namespace: default
spec:
  containers:
  - name: my-container
    image: nginx
` + "```" + `

***

## How to Specify a Resource

To retrieve a specific resource, you must provide its coordinates within the Kubernetes API. The *getResourceArgs* structure defines the necessary arguments to uniquely identify any resource.

` + "```" + `go
// The actual struct includes JSON tags. They are omitted here for clarity.
// Refer to the source code for the complete definition.
getResourceArgs struct {
    Group     string
    Version   string
    Resource  string
    Name      string
    Namespace string
}
` + "```" + `

### Argument Breakdown

* *Group*: The API group the resource belongs to. API groups help organize resources. For example, Deployments are in the *apps* group, and Jobs are in the *batch* group. Core resources, like Pods and Services, belong to the core API group, which is specified as an empty string (*""*).
* *Version*: The version of the API group, such as *v1* or *v1beta1*. This ensures API compatibility as Kubernetes evolves.
* *Resource*: The **plural, lowercase name** for the resource type (e.g., *pods*, *deployments*, *services*, *configmaps*).
* *Name*: The case-sensitive name of the specific resource instance you want to retrieve (e.g., *my-app-deployment*, *nginx-pod-123*).
* *Namespace*: The namespace where the resource exists. This is an optional field because some resources, like *Nodes* or *PersistentVolumes*, are **cluster-scoped** and do not belong to any namespace. For namespaced resources like *Pods* or *Deployments*, if you omit this, it will typically default to the *default* namespace.

### Example

To get a *Deployment* named *frontend-app* from the *production* namespace, you would structure the arguments like this:

* *Group*: *"apps"*
* *Version*: *"v1"*
* *Resource*: *"deployments"*
* *Name*: *"frontend-app"*
* *Namespace*: *"production"*
`

// ListResourcesToolDescription contains the documentation for the List Kubernetes Resources tool.
// It is formatted in Markdown.
const ListResourcesToolDescription = `
This tool retrieves a list of Kubernetes resources of a specific type from the cluster's API server.

***

## What "Listing Resources" Means

In Kubernetes, "listing resources" means fetching the **live state and specifications** for all resources that match a specific type within a given scope (e.g., within a namespace or across the entire cluster). This is the equivalent of running a command like *kubectl get pods -n my-namespace*. The server returns a collection of complete object definitions.
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

## How to Specify a Resource Type to List

To list resources, you must specify the type you are interested in. The *listResourcesArgs* structure defines the necessary arguments.

` + "```" + `go
// The actual struct includes JSON tags. They are omitted here for clarity.
// Refer to the source code for the complete definition.
listResourcesArgs struct {
    Group     string
    Version   string
    Resource  string
    Namespace string
}
` + "```" + `

### Argument Breakdown

* *Group*: The API group the resource belongs to. For example, Deployments are in the *apps* group. Core resources like Pods belong to the core API group, specified as an empty string (*""*).
* *Version*: The version of the API group, such as *v1* or *v1beta1*.
* *Resource*: The **plural, lowercase name** for the resource type (e.g., *pods*, *deployments*, *services*).
* *Namespace*: (Optional) The namespace from which to list resources.
    * If you provide a namespace, the tool will only list resources from that specific namespace.
    * If this field is **omitted** for a namespaced resource type (like *Pods*), it will list resources from **all namespaces**.
    * For cluster-scoped resources (like *Nodes*), this field should be omitted.

### Example

To list all *Services* in the *kube-system* namespace, you would structure the arguments like this:

* *Group*: *""*
* *Version*: *"v1"*
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
    Group     string
    Version   string
    Resource  string
    Name      string
    Namespace string
}
` + "```" + `

### Argument Breakdown

* *Group*: The API group the resource belongs to (e.g., *apps* for Deployments). The core API group is specified as an empty string (*""*).
* *Version*: The version of the API group, such as *v1* or *v1beta1*.
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

* *Group*: *""*
* *Version*: *"v1"*
* *Resource*: *"secrets"*
* *Name*: *"api-keys"*
* *Namespace*: *"production"*
`

type handlers struct {
	c      *config.Config
	dyn    dynamic.Interface
	mapper meta.RESTMapper
}

func Install(ctx context.Context, s *mcp.Server, c *config.Config) error {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	restConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig: %w", err)
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

	h := &handlers{
		c:      c,
		dyn:    dyn,
		mapper: mapper,
	}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kubernetes_get_resource",
		Description: GetResourceToolDescription,
	}, h.getResource)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kubernetes_list_resources",
		Description: ListResourcesToolDescription,
	}, h.listResources)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kubernetes_apply_resource",
		Description: ApplyResourceToolDescription,
	}, h.applyResource)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kubernetes_delete_resource",
		Description: DeleteResourceToolDescription,
	}, h.deleteResource)

	return nil
}

type getResourceArgs struct {
	Group     string `json:"group"`
	Version   string `json:"version"`
	Resource  string `json:"resource"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

func (h *handlers) getResource(ctx context.Context, _ *mcp.CallToolRequest, args *getResourceArgs) (*mcp.CallToolResult, any, error) {
	gvr := schema.GroupVersionResource{Group: args.Group, Version: args.Version, Resource: args.Resource}
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
	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, nil, nil
}

type listResourcesArgs struct {
	Group     string `json:"group"`
	Version   string `json:"version"`
	Resource  string `json:"resource"`
	Namespace string `json:"namespace,omitempty"`
}

func (h *handlers) listResources(ctx context.Context, _ *mcp.CallToolRequest, args *listResourcesArgs) (*mcp.CallToolResult, any, error) {
	gvr := schema.GroupVersionResource{Group: args.Group, Version: args.Version, Resource: args.Resource}
	var list *unstructured.UnstructuredList
	var err error
	if args.Namespace != "" {
		list, err = h.dyn.Resource(gvr).Namespace(args.Namespace).List(ctx, metav1.ListOptions{})
	} else {
		list, err = h.dyn.Resource(gvr).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, nil, err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, nil, nil
}

type applyResourceArgs struct {
	Manifest string `json:"manifest"`
}

func (h *handlers) applyResource(ctx context.Context, _ *mcp.CallToolRequest, args *applyResourceArgs) (*mcp.CallToolResult, any, error) {
	var obj unstructured.Unstructured
	if err := json.Unmarshal([]byte(args.Manifest), &obj); err != nil {
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
		appliedObj, err = h.dyn.Resource(gvr).Namespace(namespace).Apply(ctx, name, &obj, metav1.ApplyOptions{FieldManager: "kubeapi-mcp"})
	} else {
		appliedObj, err = h.dyn.Resource(gvr).Apply(ctx, name, &obj, metav1.ApplyOptions{FieldManager: "kubeapi-mcp"})
	}

	if err != nil {
		return nil, nil, err
	}
	data, err := json.MarshalIndent(appliedObj, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, nil, nil
}

type deleteResourceArgs struct {
	Group     string `json:"group"`
	Version   string `json:"version"`
	Resource  string `json:"resource"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

func (h *handlers) deleteResource(ctx context.Context, _ *mcp.CallToolRequest, args *deleteResourceArgs) (*mcp.CallToolResult, any, error) {
	gvr := schema.GroupVersionResource{Group: args.Group, Version: args.Version, Resource: args.Resource}
	var err error
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
