/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NodeInstanceSpec defines the desired state of NodeInstance
type NodeInstanceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// foo is an example field of NodeInstance. Edit nodeinstance_types.go to remove/update
	// +optional
	Ip            string `json:"ip"`
	NodeGroupName string `json:"nodeGroupName"`
}

type NodeInstancePhase string

const (
	NodeInstancePhasePendingSync = NodeInstancePhase("PendingSync")
	NodeInstancePhaseSyncSuccess = NodeInstancePhase("SyncSuccess")
	NodeInstancePhaseSyncFailed  = NodeInstancePhase("SyncFailed")
)

// NodeInstanceStatus defines the observed state of NodeInstance.
type NodeInstanceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the NodeInstance resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	Phase      string             `json:"phase,omitempty"`
	//上次同步得时间
	LastSynceTime string `json:"lastSynceTime"`
	//有那些项目发生了变化
	ManagerDiffDetail string `json:"managerDiffDetail"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=ni
// +kubebuilder:printcolumn:name="STATUS",type="string",JSONPath=".status.phase",description="The version of Thanos Ruler"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type NodeInstance struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of NodeInstance
	// +required
	Spec NodeInstanceSpec `json:"spec"`

	// status defines the observed state of NodeInstance
	// +optional
	Status NodeInstanceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// NodeInstanceList contains a list of NodeInstance
type NodeInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []NodeInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeInstance{}, &NodeInstanceList{})
}
