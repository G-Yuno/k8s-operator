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

// NodeGroupSpec defines the desired state of NodeGroup
type NodeGroupSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of NodeGroup. Edit nodegroup_types.go to remove/update
	// 机器ip列表
	Ips                 []string            `json:"ips"`
	PackageManagers     []PackageManager    `json:"packageManagers,omitempty"`
	KernelArgsManagers  []KernelArgsManager `json:"kernelArgsManagers,omitempty"`
	FileManagers        []FileManager       `json:"fileManagers,omitempty"`
	PostRestartServices []string            `json:"postRestartServices,omitempty"`
}

// 软件包管理
type PackageManager struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// 内核参数
type KernelArgsManager struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// 配置文件
type FileManager struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Mode    string `json:"mode"`
}

//池子刚创建出来 等待同步到节点
//池子已创建出来 部分同步到节点，还没同步完成
//池子已创建出来 全部同步到了节点
//池子正在删除	等待节点删除

type NodeGroupPhase string

const (
	NodeGroupPhasePendingSync = NodeGroupPhase("PendingSync")
	NodeGroupPhaseSynced      = NodeGroupPhase("Synced")
	NodeGroupPhaseDeleting    = NodeGroupPhase("Deleting")
)

// NodeGroupStatus defines the observed state of NodeGroup.
type NodeGroupStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the NodeGroup resource.
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
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	InstanceNum int `json:"instanceNum"`

	Phase string `json:"phase,omitempty"`
	// 作为一个池子得知道当前有多少个节点已同步，多少个节点同步失败
	SyncSuccessNodeNum        int    `json:"syncSuccessNodeNum"`
	SyncFailedNodeNum         int    `json:"syncFailedNodeNum"`
	PendingSyncNodeNum        int    `json:"pendingSyncNodeNum"`
	AnsiblePlayBookYamlString string `json:"ansiblePlayBookYamlString"`
	// 上一次和这一次模板的hash值
	LastContentHash string `json:"lastContentHash"`
	ThisContentHash string `json:"thisContentHash"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=np
// +kubebuilder:printcolumn:name="STATUS",type="string",JSONPath=".status.phase",description="The version of Thanos Ruler"
// +kubebuilder:printcolumn:name="instanceNum",type="integer",JSONPath=".status.instanceNum",description="The number of instance"
// +kubebuilder:printcolumn:name="syncSuccessNodeNum",type="integer",JSONPath=".status.syncSuccessNodeNum",description="The number of instance"
// +kubebuilder:printcolumn:name="syncFailedNodeNum",type="integer",JSONPath=".status.syncFailedNodeNum",description="The number of instance"
// +kubebuilder:printcolumn:name="pendingSyncNodeNum",type="integer",JSONPath=".status.pendingSyncNodeNum",description="The number of instance"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// NodeGroup is the Schema for the nodegroups API
type NodeGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeGroupSpec   `json:"spec,omitempty"`
	Status NodeGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodeGroupList contains a list of NodeGroup
type NodeGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []NodeGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeGroup{}, &NodeGroupList{})
}
