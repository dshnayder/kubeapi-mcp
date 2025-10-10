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
		Description: "Get a Kubernetes resource.",
	}, h.getResource)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kubernetes_list_resources",
		Description: "List Kubernetes resources.",
	}, h.listResources)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kubernetes_apply_resource",
		Description: "Apply a Kubernetes resource.",
	}, h.applyResource)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kubernetes_delete_resource",
		Description: "Delete a Kubernetes resource.",
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
