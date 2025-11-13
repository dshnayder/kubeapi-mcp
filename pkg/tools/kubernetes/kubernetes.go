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
	"google.golang.org/api/container/v1"
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
	"k8s.io/client-go/util/jsonpath"
	"sigs.k8s.io/yaml"
)

var ExtraTools = true

// GKEGetClusterToolDescription contains the documentation for the Get GKE Cluster tool.
// It is formatted in Markdown.
const GKEGetClusterToolDescription = `
Gets the details of a specific GKE cluster. This is equivalent to running "gcloud container clusters describe".

This tool is useful for inspecting the configuration of a cluster, including its node pools, network settings, and enabled features.

This tool calls the GKE API's projects.locations.clusters.get method.

Example:
To get the details of a cluster named "my-cluster" in the "us-central1-a" zone:
{
  "name": "my-cluster",
  "location": "us-central1-a"
}

The tool provides functionality similar to "gcloud" command line:
gcloud container clusters describe my-cluster --zone us-central1-a
`

// GKEListClustersToolDescription contains the documentation for the GKE List Clusters tool.
// It is formatted in Markdown.
const GKEListClustersToolDescription = `
Lists all clusters owned by a project in either the specified zone or all zones. This is equivalent to running "gcloud container clusters list".

This tool is useful for getting an overview of all the clusters in a project.

This tool calls the GKE API's projects.locations.clusters.list method.

Example:
To list all clusters in the "us-central1" region:
{
  "location": "us-central1"
}

The tool provides functionality similar to "gcloud" command line:
gcloud container clusters list --region us-central1
`

// GKEUpdateNodePoolToolDescription contains the documentation for the GKE Update Node Pool tool.
// It is formatted in Markdown.
const GKEUpdateNodePoolToolDescription = `
Updates the settings of a specific node pool. This is similar to "gcloud container node-pools update".

This tool is useful for changing the machine type, enabling or disabling autoscaling, or updating the node version of a node pool.

This tool calls the GKE API's projects.locations.clusters.nodePools.update method.

Example:
To enable autoscaling for a node pool named "my-node-pool" in a cluster named "my-cluster" in the "us-central1-a" zone, with a minimum of 1 and a maximum of 5 nodes:
{
  "cluster_name": "my-cluster",
  "location": "us-central1-a",
  "node_pool_id": "my-node-pool",
  "enable_autoscaling": true,
  "min_nodes": 1,
  "max_nodes": 5
}

The tool provides functionality similar to "gcloud" command line:
gcloud container node-pools update my-node-pool --cluster my-cluster --zone us-central1-a --enable-autoscaling --min-nodes 1 --max-nodes 5
`

// GKEGetOperationToolDescription contains the documentation for the Get GKE Operation tool.
// It is formatted in Markdown.
const GKEGetOperationToolDescription = `
Gets the status of a specific GKE operation.

Many GKE operations, such as creating or updating a cluster, are long-running. This tool allows you to check the status of such an operation to see if it has completed, failed, or is still in progress.

This tool calls the GKE API's projects.locations.operations.get method.

Example:
To get the status of an operation with the name "operation-12345":
{
  "name": "operation-12345"
}

The tool provides functionality similar to "gcloud" command line:
gcloud container operations describe operation-12345
`

// GKECreateClusterToolDescription contains the documentation for the Create GKE Cluster tool.
// It is formatted in Markdown.
const GKECreateClusterToolDescription = `
Creates a new GKE cluster. This is equivalent to running "gcloud container clusters create".

This tool is used to provision a new GKE cluster with a specified name and location.

This tool calls the GKE API's projects.locations.clusters.create method.

Example:
To create a new cluster named "my-new-cluster" in the "us-central1-a" zone:
{
  "cluster_name": "my-new-cluster",
  "location": "us-central1-a"
}

The tool provides functionality similar to "gcloud" command line:
gcloud container clusters create my-new-cluster --zone us-central1-a
`

// GKEUpdateClusterToolDescription contains the documentation for the Update GKE Cluster tool.
// It is formatted in Markdown.
const GKEUpdateClusterToolDescription = `
Updates a GKE cluster. This is equivalent to running "gcloud container clusters update".

This tool is used to modify the settings of an existing GKE cluster. For example, you can use it to update the cluster's description.

This tool calls the GKE API's projects.locations.clusters.update method.

Example:
To update the description of a cluster named "my-cluster" in the "us-central1-a" zone:
{
  "cluster_name": "my-cluster",
  "location": "us-central1-a",
  "description": "This is my updated cluster description."
}

The tool provides functionality similar to "gcloud" command line:
gcloud container clusters update my-cluster --zone us-central1-a --description "This is my updated cluster description."
`

// GKEDeleteClusterToolDescription contains the documentation for the Delete GKE Cluster tool.
// It is formatted in Markdown.
const GKEDeleteClusterToolDescription = `
Deletes a GKE cluster. This is equivalent to running "gcloud container clusters delete".

This tool is used to permanently delete a GKE cluster. This action is irreversible.

This tool calls the GKE API's projects.locations.clusters.delete method.

Example:
To delete a cluster named "my-cluster" in the "us-central1-a" zone:
{
  "cluster_name": "my-cluster",
  "location": "us-central1-a"
}

The tool provides functionality similar to "gcloud" command line:
gcloud container clusters delete my-cluster --zone us-central1-a
`

// GKEFetchClusterUpgradeInfoToolDescription contains the documentation for the fetch cluster upgrade info tool.
// It is formatted in Markdown.
const GKEFetchClusterUpgradeInfoToolDescription = `
Fetches information about available upgrades for a GKE cluster.

This tool is useful for determining what versions are available to upgrade a cluster's control plane and nodes to.

This tool calls the GKE API's projects.locations.clusters.getUpgradeInfo method.

Example:
To get upgrade information for a cluster named "my-cluster" in the "us-central1-a" zone:
{
  "cluster_name": "my-cluster",
  "location": "us-central1-a"
}

The tool provides functionality similar to "gcloud" command line:
gcloud container clusters get-upgrade-info my-cluster --zone us-central1-a
`

// GKECreateNodePoolToolDescription contains the documentation for the Create GKE Node Pool tool.
// It is formatted in Markdown.
const GKECreateNodePoolToolDescription = `
Creates a new node pool in a GKE cluster.

A node pool is a group of nodes within a cluster that all have the same configuration. Node pools are useful for creating groups of nodes with specific characteristics, such as machine type, autoscaling configuration, or attached accelerators. You might create a new node pool to:
- Isolate workloads with different resource requirements.
- Use different machine types for different workloads.
- Enable autoscaling for a specific set of nodes.
- Add GPUs or other accelerators to a subset of your nodes.

This tool calls the GKE API's projects.locations.clusters.nodePools.create method.
`

// GKEUpdateMasterToolDescription contains the documentation for the GKE Update Master tool.
// It is formatted in Markdown.
const GKEUpdateMasterToolDescription = `
Updates the master of a specific cluster. This operation is long-running and returns an operation ID.

This tool is used to upgrade the Kubernetes version of the control plane of a GKE cluster.

This tool calls the GKE API's projects.locations.clusters.master.update method.

Example:
To upgrade the master of a cluster named "my-cluster" in the "us-central1-a" zone to version "1.25.2-gke.1700":
{
  "cluster_name": "my-cluster",
  "location": "us-central1-a",
  "master_version": "1.25.2-gke.1700"
}

The tool provides functionality similar to "gcloud" command line:
gcloud container clusters upgrade my-cluster --zone us-central1-a --master --cluster-version 1.25.2-gke.1700
`

// GKEStartIPRotationToolDescription contains the documentation for the GKE Start IP Rotation tool.
// It is formatted in Markdown.
const GKEStartIPRotationToolDescription = `
Starts IP rotation on a specific cluster. This operation is long-running and returns an operation ID.

IP rotation changes the IP address that the control plane uses to serve requests from the Kubernetes API.

This tool calls the GKE API's projects.locations.clusters.startIpRotation method.

Example:
To start IP rotation for a cluster named "my-cluster" in the "us-central1-a" zone:
{
  "cluster_name": "my-cluster",
  "location": "us-central1-a"
}

The tool provides functionality similar to "gcloud" command line:
gcloud container clusters update my-cluster --zone us-central1-a --start-ip-rotation
`

// GKECompleteIPRotationToolDescription contains the documentation for the GKE Complete IP Rotation tool.
// It is formatted in Markdown.
const GKECompleteIPRotationToolDescription = `
Completes IP rotation on a specific cluster. This operation is long-running and returns an operation ID.

This tool should be called after starting an IP rotation and recreating all nodes in the cluster.

This tool calls the GKE API's projects.locations.clusters.completeIpRotation method.

Example:
To complete IP rotation for a cluster named "my-cluster" in the "us-central1-a" zone:
{
  "cluster_name": "my-cluster",
  "location": "us-central1-a"
}

The tool provides functionality similar to "gcloud" command line:
gcloud container clusters update my-cluster --zone us-central1-a --complete-ip-rotation
`

// GKESetMaintenancePolicyToolDescription contains the documentation for the GKE Set Maintenance Policy tool.
// It is formatted in Markdown.
const GKESetMaintenancePolicyToolDescription = `
Sets the maintenance policy for a specific cluster. This operation is long-running and returns an operation ID.

A maintenance policy defines a recurring window of time during which maintenance on the cluster control plane is performed.

This tool calls the GKE API's projects.locations.clusters.setMaintenancePolicy method.

Example:
To set a daily maintenance window from 10:00 to 14:00 UTC for a cluster named "my-cluster" in the "us-central1-a" zone:
{
  "cluster_name": "my-cluster",
  "location": "us-central1-a",
  "maintenance_policy": "{\"dailyMaintenanceWindow\":{\"startTime\":\"10:00\",\"duration\":\"4h\"}}"
}

The tool provides functionality similar to "gcloud" command line:
gcloud container clusters update my-cluster --zone us-central1-a --maintenance-window "10:00"
`

// GKEGetServerConfigToolDescription contains the documentation for the GKE Get Server Config tool.
// It is formatted in Markdown.
const GKEGetServerConfigToolDescription = `
Gets the server config for a GKE cluster.

This tool returns information about the GKE server configuration, such as the default cluster version and available node versions.

This tool calls the GKE API's projects.locations.getServerConfig method.

Example:
To get the server config for the "us-central1" region:
{
  "location": "us-central1"
}

The tool provides functionality similar to "gcloud" command line:
gcloud container get-server-config --region us-central1
`

// GKEGetOpenIDConfigToolDescription contains the documentation for the GKE Get OpenID Config tool.
// It is formatted in Markdown.
const GKEGetOpenIDConfigToolDescription = `
Gets the OpenID configuration for a GKE cluster.

This tool is useful for configuring OpenID Connect (OIDC) authentication for a GKE cluster.

This tool calls the GKE API's projects.locations.clusters.getOpenid-config method.

Example:
To get the OpenID configuration for a cluster named "my-cluster" in the "us-central1-a" zone:
{
  "cluster_name": "my-cluster",
  "location": "us-central1-a"
}
`

// GKEGetJSONWebKeysToolDescription contains the documentation for the GKE Get JSON Web Keys tool.
// It is formatted in Markdown.
const GKEGetJSONWebKeysToolDescription = `
Gets the JSON Web Keys for a GKE cluster.

This tool is useful for verifying the authenticity of OpenID Connect (OIDC) tokens issued by the GKE cluster.

This tool calls the GKE API's projects.locations.clusters.getJwk method.

Example:
To get the JSON Web Keys for a cluster named "my-cluster" in the "us-central1-a" zone:
{
  "cluster_name": "my-cluster",
  "location": "us-central1-a"
}
`

// GKEListUsableSubnetworksToolDescription contains the documentation for the GKE List Usable Subnetworks tool.
// It is formatted in Markdown.
const GKEListUsableSubnetworksToolDescription = `
Lists usable subnetworks for a GKE cluster.

This tool is useful for finding available subnetworks when creating a new GKE cluster.

This tool calls the GKE API's projects.locations.usableSubnetworks.list method.

Example:
To list usable subnetworks in a project:
{}

The tool provides functionality similar to "gcloud" command line:
gcloud container usable-subnetworks list
`

// GKECheckAutopilotCompatibilityToolDescription contains the documentation for the GKE Check Autopilot Compatibility tool.
// It is formatted in Markdown.
const GKECheckAutopilotCompatibilityToolDescription = `
Checks Autopilot compatibility for a GKE cluster.

This tool analyzes a GKE Standard cluster's workloads and provides recommendations for migrating to GKE Autopilot.

This tool calls the GKE API's projects.locations.clusters.checkAutopilotCompatibility method.

Example:
To check Autopilot compatibility for a cluster named "my-cluster" in the "us-central1-a" zone:
{
  "cluster_name": "my-cluster",
  "location": "us-central1-a"
}

The tool provides functionality similar to "gcloud" command line:
gcloud container clusters check-autopilot-compatibility my-cluster --zone us-central1-a
`

// GKECompleteConvertToAutopilotToolDescription contains the documentation for the GKE Complete Convert to Autopilot tool.
// It is formatted in Markdown.
const GKECompleteConvertToAutopilotToolDescription = `
Completes the conversion of a GKE Standard cluster to GKE Autopilot.

This tool should be called after initiating the conversion to Autopilot and addressing any compatibility issues.

This tool calls the GKE API's projects.locations.clusters.completeConvertToAutopilot method.

Example:
To complete the conversion to Autopilot for a cluster named "my-cluster" in the "us-central1-a" zone:
{
  "cluster_name": "my-cluster",
  "location": "us-central1-a"
}
`

// GKECompleteControlPlaneUpgradeToolDescription contains the documentation for the GKE Complete Control Plane Upgrade tool.
// It is formatted in Markdown.
const GKECompleteControlPlaneUpgradeToolDescription = `
Completes a manual control plane upgrade for a GKE cluster.

This tool should be called after initiating a manual control plane upgrade and verifying the health of the control plane.

This tool calls the GKE API's projects.locations.clusters.completeControlPlaneUpgrade method.

Example:
To complete a control plane upgrade for a cluster named "my-cluster" in the "us-central1-a" zone:
{
  "cluster_name": "my-cluster",
  "location": "us-central1-a"
}
`

type gkeUpdateNodePoolArgs struct {
	ProjectID         string `json:"project_id,omitempty"`
	Location          string `json:"location"`
	ClusterName       string `json:"cluster_name"`
	NodePoolID        string `json:"node_pool_id"`
	NodeVersion       string `json:"node_version,omitempty"`
	MachineType       string `json:"machine_type,omitempty"`
	EnableAutoscaling *bool  `json:"enable_autoscaling,omitempty"`
	MinNodes          *int64 `json:"min_nodes,omitempty"`
	MaxNodes          *int64 `json:"max_nodes,omitempty"`
	TotalMinNodes     *int64 `json:"total_min_nodes,omitempty"`
	TotalMaxNodes     *int64 `json:"total_max_nodes,omitempty"`
}

type gkeGetOperationArgs struct {
	Name string `json:"name"`
}

type gkeCreateClusterArgs struct {
	ProjectID   string `json:"project_id,omitempty"`
	Location    string `json:"location"`
	ClusterName string `json:"cluster_name"`
}

type gkeUpdateClusterArgs struct {
	ProjectID   string `json:"project_id,omitempty"`
	Location    string `json:"location"`
	ClusterName string `json:"cluster_name"`
	Description string `json:"description,omitempty"`
}

type gkeDeleteClusterArgs struct {
	ProjectID   string `json:"project_id,omitempty"`
	Location    string `json:"location"`
	ClusterName string `json:"cluster_name"`
}

type gkeFetchClusterUpgradeInfoArgs struct {
	ProjectID   string `json:"project_id,omitempty"`
	Location    string `json:"location"`
	ClusterName string `json:"cluster_name"`
}

type gkeCreateNodePoolArgs struct {
	ProjectID    string `json:"project_id,omitempty"`
	Location     string `json:"location"`
	ClusterName  string `json:"cluster_name"`
	NodePoolName string `json:"node_pool_name"`
	MachineType  string `json:"machine_type,omitempty"`
	NumNodes     int64  `json:"num_nodes,omitempty"`
}

type gkeUpdateMasterArgs struct {
	ProjectID    string `json:"project_id,omitempty"`
	Location     string `json:"location"`
	ClusterName  string `json:"cluster_name"`
	MasterVerion string `json:"master_version"`
}

type gkeStartIPRotationArgs struct {
	ProjectID   string `json:"project_id,omitempty"`
	Location    string `json:"location"`
	ClusterName string `json:"cluster_name"`
}

type gkeCompleteIPRotationArgs struct {
	ProjectID   string `json:"project_id,omitempty"`
	Location    string `json:"location"`
	ClusterName string `json:"cluster_name"`
}

type gkeSetMaintenancePolicyArgs struct {
	ProjectID         string `json:"project_id,omitempty"`
	Location          string `json:"location"`
	ClusterName       string `json:"cluster_name"`
	MaintenancePolicy string `json:"maintenance_policy"`
}

type gkeGetServerConfigArgs struct {
	ProjectID string `json:"project_id,omitempty"`
	Location  string `json:"location"`
}

type gkeGetOpenIDConfigArgs struct {
	ProjectID   string `json:"project_id,omitempty"`
	Location    string `json:"location"`
	ClusterName string `json:"cluster_name"`
}

type gkeGetJSONWebKeysArgs struct {
	ProjectID   string `json:"project_id,omitempty"`
	Location    string `json:"location"`
	ClusterName string `json:"cluster_name"`
}

type gkeListUsableSubnetworksArgs struct {
	ProjectID string `json:"project_id,omitempty"`
}

type gkeCheckAutopilotCompatibilityArgs struct {
	ProjectID   string `json:"project_id,omitempty"`
	Location    string `json:"location"`
	ClusterName string `json:"cluster_name"`
}

type gkeCompleteConvertToAutopilotArgs struct {
	ProjectID   string `json:"project_id,omitempty"`
	Location    string `json:"location"`
	ClusterName string `json:"cluster_name"`
}

type gkeCompleteControlPlaneUpgradeArgs struct {
	ProjectID   string `json:"project_id,omitempty"`
	Location    string `json:"location"`
	ClusterName string `json:"cluster_name"`
}

// GKEReadLogsToolDescription contains the documentation for the GKE Read Logs tool.
// It is formatted in Markdown.
const GKEReadLogsToolDescription = `
This tool reads GKE logs using the Google Cloud Logging API. This is the equivalent of running "gcloud logging read". Before using this tool, it's **strongly** recommended to call the 'get_log_schema' tool to get information about supported log types and their schemas. Logs are returned in ascending order, based on the timestamp (i.e. oldest first).

This tool calls the Google Cloud Logging API's entries.list method.
`

// GKEGetLogSchemaToolDescription contains the documentation for the GKE Get Log Schema tool.
// It is formatted in Markdown.
const GKEGetLogSchemaToolDescription = `
Get the schema for a specific log type, which can be used with the gke_read_logs tool. This tool provides example queries and field names for different log types.
`

// GetResourcesToolDescription contains the documentation for the Get Kubernetes Resources tool.
// It is formatted in Markdown.
const GetResourcesToolDescription = `
This tool retrieves one or more Kubernetes resources from the cluster's API server.

***

## What "Getting Resources" Means

In Kubernetes, "getting resources" means fetching the **live state and specifications** for all resources that match a specific type within a given scope (e.g., within a namespace or across the entire cluster). This is the equivalent of running a command like *kubectl get pods -n my-namespace*. If a name is specified, it will fetch a single resource, equivalent to *kubectl get pod my-pod*. The server returns a collection of complete object definitions.

## Custom Columns:

The 'custom-columns' argument allows you to format the output as a table with custom columns. The value is a comma-separated list of 'HEADER:JSONPATH' pairs.

- **HEADER**: The column header.
- **JSONPATH**: A [JSONPath](https://kubernetes.io/docs/reference/kubectl/jsonpath/) expression to extract a value from the resource.

**Example:**

To get the name and image of all pods in the 'default' namespace, you would use the following arguments:

- 'resource': 'pods'
- 'namespace': 'default'
- 'custom-columns': 'NAME:.metadata.name,IMAGE:.spec.containers[0].image'

This would produce output similar to this:

NAME          IMAGE
my-pod-1      nginx:latest
my-pod-2      ubuntu:22.04

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

In Kubernetes, "applying" is a **declarative** operation that makes the live state of a resource in the cluster match the state defined in your configuration file (the manifest). This is the equivalent of running *kubectl apply -f <filename.yaml>.*

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
    Previous  bool
}
` + "```" + `

### Argument Breakdown

* *Name*: The case-sensitive name of the pod.
* *Namespace*: The namespace where the pod exists.
* *Container*: (Optional) The name of the container to get logs from. If omitted, and the pod has multiple containers, an error will be returned.
* *Previous*: (Optional) If true, return logs from the previous instantiation of the container.

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

type gkeGetClusterArgs struct {
	ProjectID string `json:"project_id,omitempty"`
	Location  string `json:"location"`
	Name      string `json:"name"`
}

type handlers struct {
	c                *config.Config
	dyn              dynamic.Interface
	mapper           meta.RESTMapper
	dc               *discovery.DiscoveryClient
	clientset        kubernetes.Interface
	logadminClient   *logadmin.Client
	containerService *container.Service
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

	containerService, err := container.NewService(ctx)
	if err != nil {
		return fmt.Errorf("failed to create container service: %w", err)
	}

	h := &handlers{
		c:                c,
		dyn:              dyn,
		mapper:           mapper,
		dc:               dc,
		clientset:        clientset,
		logadminClient:   logadminClient,
		containerService: containerService,
	}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kube_get_resources",
		Description: GetResourcesToolDescription,
	}, h.getResources)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kube_api_resources",
		Description: APIResourcesToolDescription,
	}, h.apiResources)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kube_get_pod_logs",
		Description: GetPodLogsToolDescription,
	}, h.getPodLogs)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kube_can_i",
		Description: CanIToolDescription,
	}, h.canI)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "gke_read_logs",
		Description: GKEReadLogsToolDescription,
	}, h.queryLogs)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "gke_get_log_schema",
		Description: GKEGetLogSchemaToolDescription,
	}, h.getLogSchema)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "gke_get_cluster",
		Description: GKEGetClusterToolDescription,
	}, h.gkeGetCluster)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "gke_list_clusters",
		Description: GKEListClustersToolDescription,
	}, h.gkeListClusters)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "gke_get_operation",
		Description: GKEGetOperationToolDescription,
	}, h.gkeGetOperation)

	if !c.ReadOnly() {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "kube_apply_resource",
			Description: ApplyResourceToolDescription,
		}, h.applyResource)
		mcp.AddTool(s, &mcp.Tool{
			Name:        "kube_delete_resource",
			Description: DeleteResourceToolDescription,
		}, h.deleteResource)

		mcp.AddTool(s, &mcp.Tool{
			Name:        "kube_patch_resource",
			Description: PatchResourceToolDescription,
		}, h.patchResource)

		if ExtraTools {
			mcp.AddTool(s, &mcp.Tool{
				Name:        "gke_update_node_pool",
				Description: GKEUpdateNodePoolToolDescription,
			}, h.gkeUpdateNodePool)

			mcp.AddTool(s, &mcp.Tool{
				Name:        "gke_create_cluster",
				Description: GKECreateClusterToolDescription,
			}, h.gkeCreateCluster)

			mcp.AddTool(s, &mcp.Tool{
				Name:        "gke_update_cluster",
				Description: GKEUpdateClusterToolDescription,
			}, h.gkeUpdateCluster)

			mcp.AddTool(s, &mcp.Tool{
				Name:        "gke_delete_cluster",
				Description: GKEDeleteClusterToolDescription,
			}, h.gkeDeleteCluster)

			mcp.AddTool(s, &mcp.Tool{
				Name:        "gke_fetch_cluster_upgrade_info",
				Description: GKEFetchClusterUpgradeInfoToolDescription,
			}, h.gkeFetchClusterUpgradeInfo)

			mcp.AddTool(s, &mcp.Tool{
				Name:        "gke_create_node_pool",
				Description: GKECreateNodePoolToolDescription,
			}, h.gkeCreateNodePool)

			mcp.AddTool(s, &mcp.Tool{
				Name:        "gke_update_master",
				Description: GKEUpdateMasterToolDescription,
			}, h.gkeUpdateMaster)

			mcp.AddTool(s, &mcp.Tool{
				Name:        "gke_start_ip_rotation",
				Description: GKEStartIPRotationToolDescription,
			}, h.gkeStartIPRotation)
			// 22 tools up to this point

			mcp.AddTool(s, &mcp.Tool{
				Name:        "gke_set_maintenance_policy",
				Description: GKESetMaintenancePolicyToolDescription,
			}, h.gkeSetMaintenancePolicy)

			mcp.AddTool(s, &mcp.Tool{
				Name:        "gke_get_server_config",
				Description: GKEGetServerConfigToolDescription,
			}, h.gkeGetServerConfig)

			mcp.AddTool(s, &mcp.Tool{
				Name:        "gke_get_open_id_config",
				Description: GKEGetOpenIDConfigToolDescription,
			}, h.gkeGetOpenIDConfig)

			mcp.AddTool(s, &mcp.Tool{
				Name:        "gke_get_json_web_keys",
				Description: GKEGetJSONWebKeysToolDescription,
			}, h.gkeGetJSONWebKeys)

			mcp.AddTool(s, &mcp.Tool{
				Name:        "gke_list_usable_subnetworks",
				Description: GKEListUsableSubnetworksToolDescription,
			}, h.gkeListUsableSubnetworks)

			mcp.AddTool(s, &mcp.Tool{
				Name:        "gke_check_autopilot_compatibility",
				Description: GKECheckAutopilotCompatibilityToolDescription,
			}, h.gkeCheckAutopilotCompatibility)

			mcp.AddTool(s, &mcp.Tool{
				Name:        "gke_complete_convert_to_autopilot",
				Description: GKECompleteConvertToAutopilotToolDescription,
			}, h.gkeCompleteConvertToAutopilot)

			mcp.AddTool(s, &mcp.Tool{
				Name:        "gke_complete_control_plane_upgrade",
				Description: GKECompleteControlPlaneUpgradeToolDescription,
			}, h.gkeCompleteControlPlaneUpgrade)
		}
	}
	return nil
}

func (h *handlers) gkeUpdateNodePool(ctx context.Context, _ *mcp.CallToolRequest, args *gkeUpdateNodePoolArgs) (*mcp.CallToolResult, any, error) {
	return nil, nil, fmt.Errorf("tool not implemented: this tool is a placeholder. Stop execution and inform the user.")
}

func (h *handlers) gkeCreateNodePool(ctx context.Context, _ *mcp.CallToolRequest, args *gkeCreateNodePoolArgs) (*mcp.CallToolResult, any, error) {
	return nil, nil, fmt.Errorf("tool not implemented: this tool is a placeholder. Stop execution and inform the user.")
}

func (h *handlers) gkeUpdateMaster(ctx context.Context, _ *mcp.CallToolRequest, args *gkeUpdateMasterArgs) (*mcp.CallToolResult, any, error) {
	return nil, nil, fmt.Errorf("tool not implemented: this tool is a placeholder. Stop execution and inform the user.")
}

func (h *handlers) gkeStartIPRotation(ctx context.Context, _ *mcp.CallToolRequest, args *gkeStartIPRotationArgs) (*mcp.CallToolResult, any, error) {
	return nil, nil, fmt.Errorf("tool not implemented: this tool is a placeholder. Stop execution and inform the user.")
}

func (h *handlers) gkeCompleteIPRotation(ctx context.Context, _ *mcp.CallToolRequest, args *gkeCompleteIPRotationArgs) (*mcp.CallToolResult, any, error) {
	return nil, nil, fmt.Errorf("tool not implemented: this tool is a placeholder. Stop execution and inform the user.")
}

func (h *handlers) gkeSetMaintenancePolicy(ctx context.Context, _ *mcp.CallToolRequest, args *gkeSetMaintenancePolicyArgs) (*mcp.CallToolResult, any, error) {
	return nil, nil, fmt.Errorf("tool not implemented: this tool is a placeholder. Stop execution and inform the user.")
}

func (h *handlers) gkeGetServerConfig(ctx context.Context, _ *mcp.CallToolRequest, args *gkeGetServerConfigArgs) (*mcp.CallToolResult, any, error) {
	return nil, nil, fmt.Errorf("tool not implemented: this tool is a placeholder. Stop execution and inform the user.")
}

func (h *handlers) gkeGetOpenIDConfig(ctx context.Context, _ *mcp.CallToolRequest, args *gkeGetOpenIDConfigArgs) (*mcp.CallToolResult, any, error) {
	return nil, nil, fmt.Errorf("tool not implemented: this tool is a placeholder. Stop execution and inform the user.")
}

func (h *handlers) gkeGetJSONWebKeys(ctx context.Context, _ *mcp.CallToolRequest, args *gkeGetJSONWebKeysArgs) (*mcp.CallToolResult, any, error) {
	return nil, nil, fmt.Errorf("tool not implemented: this tool is a placeholder. Stop execution and inform the user.")
}

func (h *handlers) gkeListUsableSubnetworks(ctx context.Context, _ *mcp.CallToolRequest, args *gkeListUsableSubnetworksArgs) (*mcp.CallToolResult, any, error) {
	return nil, nil, fmt.Errorf("tool not implemented: this tool is a placeholder. Stop execution and inform the user.")
}

func (h *handlers) gkeCheckAutopilotCompatibility(ctx context.Context, _ *mcp.CallToolRequest, args *gkeCheckAutopilotCompatibilityArgs) (*mcp.CallToolResult, any, error) {
	return nil, nil, fmt.Errorf("tool not implemented: this tool is a placeholder. Stop execution and inform the user.")
}

func (h *handlers) gkeCompleteConvertToAutopilot(ctx context.Context, _ *mcp.CallToolRequest, args *gkeCompleteConvertToAutopilotArgs) (*mcp.CallToolResult, any, error) {
	return nil, nil, fmt.Errorf("tool not implemented: this tool is a placeholder. Stop execution and inform the user.")
}

func (h *handlers) gkeCompleteControlPlaneUpgrade(ctx context.Context, _ *mcp.CallToolRequest, args *gkeCompleteControlPlaneUpgradeArgs) (*mcp.CallToolResult, any, error) {
	return nil, nil, fmt.Errorf("tool not implemented: this tool is a placeholder. Stop execution and inform the user.")
}

func (h *handlers) gkeFetchClusterUpgradeInfo(ctx context.Context, _ *mcp.CallToolRequest, args *gkeFetchClusterUpgradeInfoArgs) (*mcp.CallToolResult, any, error) {
	return nil, nil, fmt.Errorf("tool not implemented: this tool is a placeholder. Stop execution and inform the user.")
}

func (h *handlers) gkeDeleteCluster(ctx context.Context, _ *mcp.CallToolRequest, args *gkeDeleteClusterArgs) (*mcp.CallToolResult, any, error) {
	return nil, nil, fmt.Errorf("tool not implemented: this tool is a placeholder. Stop execution and inform the user.")
}

func (h *handlers) gkeUpdateCluster(ctx context.Context, _ *mcp.CallToolRequest, args *gkeUpdateClusterArgs) (*mcp.CallToolResult, any, error) {
	return nil, nil, fmt.Errorf("tool not implemented: this tool is a placeholder. Stop execution and inform the user.")
}

func (h *handlers) gkeCreateCluster(ctx context.Context, _ *mcp.CallToolRequest, args *gkeCreateClusterArgs) (*mcp.CallToolResult, any, error) {
	return nil, nil, fmt.Errorf("tool not implemented: this tool is a placeholder. Stop execution and inform the user.")
}

type gkeListClustersArgs struct {
	ProjectID string `json:"project_id,omitempty"`
	Location  string `json:"location,omitempty"`
}

func (h *handlers) gkeGetOperation(ctx context.Context, _ *mcp.CallToolRequest, args *gkeGetOperationArgs) (*mcp.CallToolResult, any, error) {
	op, err := h.containerService.Projects.Locations.Operations.Get(args.Name).Context(ctx).Do()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get operation: %w", err)
	}
	b, err := json.Marshal(op)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal operation: %w", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(b)},
		},
	}, nil, nil
}

func (h *handlers) gkeListClusters(ctx context.Context, _ *mcp.CallToolRequest, args *gkeListClustersArgs) (*mcp.CallToolResult, any, error) {
	projectID := args.ProjectID
	if projectID == "" {
		projectID = h.c.DefaultProjectID()
	}
	location := args.Location
	if location == "" {
		location = "-"
	}
	parent := fmt.Sprintf("projects/%s/locations/%s", projectID, location)
	resp, err := h.containerService.Projects.Locations.Clusters.List(parent).Context(ctx).Do()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list clusters: %w", err)
	}
	b, err := json.Marshal(resp)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal clusters: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(b)},
		},
	}, nil, nil
}

func (h *handlers) gkeGetCluster(ctx context.Context, _ *mcp.CallToolRequest, args *gkeGetClusterArgs) (*mcp.CallToolResult, any, error) {
	projectID := args.ProjectID
	if projectID == "" {
		projectID = h.c.DefaultProjectID()
	}
	name := fmt.Sprintf("projects/%s/locations/%s/clusters/%s", projectID, args.Location, args.Name)
	cluster, err := h.containerService.Projects.Locations.Clusters.Get(name).Context(ctx).Do()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get cluster: %w", err)
	}
	b, err := json.Marshal(cluster)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal cluster: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(b)},
		},
	}, nil, nil
}

func (h *handlers) getLogSchema(ctx context.Context, _ *mcp.CallToolRequest, args *getLogSchemaArgs) (*mcp.CallToolResult, any, error) {
	schemas := map[string]string{
		"k8s_audit_logs": `
resource.type="k8s_audit"
resource.labels.cluster_name="CLUSTER_NAME"
resource.labels.location="CLUSTER_LOCATION"
log_id("cloudaudit.googleapis.com/activity")

protoPayload.methodName="METHOD_NAME"
protoPayload.serviceName="k8s.io"
protoPayload.resourceName="RESOURCE_NAME"

# Example: Get all audit logs for a specific cluster
resource.type="k8s_audit" AND resource.labels.cluster_name="my-cluster" AND resource.labels.location="us-central1"

# Example: Get audit logs for pod creations
resource.type="k8s_audit" AND protoPayload.methodName="io.k8s.core.v1.pods.create"

# Example: Get audit logs for a specific user
resource.type="k8s_audit" AND protoPayload.authenticationInfo.principalEmail="user@example.com"
`,
		"k8s_application_logs": `
resource.type="k8s_container"
resource.labels.cluster_name="CLUSTER_NAME"
resource.labels.location="CLUSTER_LOCATION"
resource.labels.pod_name="POD_NAME"
resource.labels.namespace_name="NAMESPACE_NAME"

# Example: Get all application logs for a specific pod
resource.type="k8s_container" AND resource.labels.cluster_name="my-cluster" AND resource.labels.pod_name="my-pod"

# Example: Get error logs from a specific namespace
resource.type="k8s_container" AND resource.labels.namespace_name="production" AND severity=ERROR
`,
		"k8s_event_logs": `
resource.type="k8s_events"
resource.labels.cluster_name="CLUSTER_NAME"
resource.labels.location="CLUSTER_LOCATION"

# Example: Get all event logs for a specific cluster
resource.type="k8s_events" AND resource.labels.cluster_name="my-cluster"

# Example: Get warning events
resource.type="k8s_events" AND severity=WARNING
`,
	}

	schema, ok := schemas[args.LogType]
	if !ok {
		return nil, nil, fmt.Errorf("unsupported log type: %s. Supported values are: %v", args.LogType, []string{"k8s_audit_logs", "k8s_application_logs", "k8s_event_logs"})
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: schema},
		},
	}, nil, nil
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

type getLogSchemaArgs struct {
	LogType string `json:"log_type"`
}

type canIArgs struct {
	Verb        string `json:"verb"`
	Resource    string `json:"resource"`
	Subresource string `json:"subresource,omitempty"`
	Name        string `json:"name,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
}

type queryLogsArgs struct {
	Query     string                  `json:"query"`
	ProjectID string                  `json:"project_id"`
	Format    string                  `json:"format,omitempty"`
	Limit     int                     `json:"limit,omitempty"`
	Since     string                  `json:"since,omitempty"`
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
	CustomColumns string `json:"custom-columns,omitempty"`
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

	if args.CustomColumns != "" {
		customOutput, err := FmtCustomColumns(resources, args.CustomColumns)
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: customOutput},
			},
		}, nil, nil
	}

	var yamlDocs []string
	for _, item := range resources {
		// Convert Unstructured to JSON
		itemJsonData, err := json.Marshal(item.Object)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal resource to JSON: %w", err)
		}

		// Convert JSON to YAML
		yamlData, err := yaml.JSONToYAML(itemJsonData)
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
	Previous  bool   `json:"previous,omitempty"`
}

func (h *handlers) getPodLogs(ctx context.Context, _ *mcp.CallToolRequest, args *getPodLogsArgs) (*mcp.CallToolResult, any, error) {
	podLogOpts := &corev1.PodLogOptions{
		Container: args.Container,
		Previous:  args.Previous,
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

func FmtCustomColumns(items []unstructured.Unstructured, customColumns string) (string, error) {
	var output strings.Builder
	columns := strings.Split(customColumns, ",")
	var headers []string
	var paths []string
	for _, col := range columns {
		parts := strings.Split(col, ":")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid custom column format: %s", col)
		}
		headers = append(headers, parts[0])
		paths = append(paths, parts[1])
	}
	output.WriteString(strings.Join(headers, "\t") + "\n")

	for _, item := range items {
		var row []string
		for _, path := range paths {
			j := jsonpath.New("custom")
			if err := j.Parse(fmt.Sprintf("{%s}", path)); err != nil {
				return "", fmt.Errorf("failed to parse jsonpath: %w", err)
			}
			results, err := j.FindResults(item.Object)
			if err != nil {
				return "", fmt.Errorf("failed to find results: %w", err)
			}
			if len(results) > 0 && len(results[0]) > 0 {
				row = append(row, fmt.Sprintf("%v", results[0][0].Interface()))
			} else {
				row = append(row, "<none>")
			}
		}
		output.WriteString(strings.Join(row, "\t") + "\n")
	}
	return output.String(), nil
}
