/*
Copyright 2025 The Kubernetes Authors.

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

package v1alpha1

import (
	"github.com/awslabs/operatorpkg/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// YandexNodeClass is the Schema for the YandexNodeClass API
// +kubebuilder:object:root=true
// +kubebuilder:object:generate=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
type YandexNodeClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of YandexNodeClass
	Spec YandexNodeClassSpec `json:"spec,omitempty"`

	// Status defines the observed state of YandexNodeClass
	Status YandexNodeClassStatus `json:"status,omitempty"`
}

// PlacementStrategy defines how nodes should be placed across zones
type PlacementStrategy struct {
	// ZoneBalance determines how nodes are distributed across zones
	// Valid values are:
	// - "Balanced" (default) - Nodes are evenly distributed across zones
	// - "AvailabilityFirst" - Prioritize zone availability over even distribution
	// +optional
	// +kubebuilder:validation:Enum=Balanced;AvailabilityFirst
	// +kubebuilder:default=Balanced
	ZoneBalance string `json:"zoneBalance,omitempty"`
}

// YandexNodeClassSpec defines the desired state of YandexNodeClass
type YandexNodeClassSpec struct {
	// CloudID is the Yandex Cloud ID where nodes will be created
	// +required
	CloudID string `json:"cloudID"`

	// FolderID is the Yandex Cloud folder ID where nodes will be created
	// +required
	FolderID string `json:"folderID"`

	// Zone is the availability zone where nodes will be created
	// If not specified, zones will be automatically selected based on placement strategy
	// +optional
	Zone string `json:"zone,omitempty"`

	// NetworkID is the ID of the network for the VM
	// +required
	NetworkID string `json:"networkID"`

	// SubnetID is the ID of the subnet for the VM
	// If not specified, a subnet in the selected zone will be automatically selected
	// +optional
	SubnetID string `json:"subnetID,omitempty"`

	// DiskType is the type of disk to create
	// Valid values are:
	// - "network-hdd" (default)
	// - "network-ssd"
	// - "network-ssd-nonreplicated"
	// +optional
	// +kubebuilder:validation:Enum=network-hdd;network-ssd;network-ssd-nonreplicated
	// +kubebuilder:default=network-hdd
	DiskType string `json:"diskType,omitempty"`

	// DiskSize is the size of the disk in GB
	// +optional
	// +kubebuilder:default=100
	DiskSize int `json:"diskSize,omitempty"`

	// ImageID is the ID of the VM image to use
	// +required
	ImageID string `json:"imageID"`

	// PlacementStrategy defines how nodes should be placed across zones
	// Only used when Zone or Subnet is not specified
	// +optional
	PlacementStrategy *PlacementStrategy `json:"placementStrategy,omitempty"`

	// Labels to apply to the VMs
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// MetadataOptions for the generated launch template of provisioned nodes.
	// +optional
	MetadataOptions *MetadataOptions `json:"metadataOptions,omitempty"`

	// SecurityGroups to apply to the VMs
	// +optional
	SecurityGroups []string `json:"securityGroups,omitempty"`
}

// MetadataOptions contains parameters for specifying VM metadata
type MetadataOptions struct {
	// UserData is base64-encoded user-data to be made available to the instance
	// +optional
	UserData string `json:"userData,omitempty"`
}

// YandexNodeClassStatus defines the observed state of YandexNodeClass
type YandexNodeClassStatus struct {
	// SpecHash is a hash of the YandexNodeClass spec
	// +optional
	SpecHash uint64 `json:"specHash,omitempty"`

	// LastValidationTime is the last time the nodeclass was validated
	// +optional
	LastValidationTime metav1.Time `json:"lastValidationTime,omitempty"`

	// ValidationError contains the error message from the last validation
	// +optional
	ValidationError string `json:"validationError,omitempty"`

	// SelectedInstanceTypes contains the list of instance types that meet the requirements
	// Only populated when using automatic instance type selection
	// +optional
	SelectedInstanceTypes []string `json:"selectedInstanceTypes,omitempty"`

	// Conditions contains signals for health and readiness
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// StatusConditions returns the condition set for the status.Object interface
func (in *YandexNodeClass) StatusConditions() status.ConditionSet {
	return status.NewReadyConditions().For(in)
}

// GetConditions returns the conditions as status.Conditions for the status.Object interface
func (in *YandexNodeClass) GetConditions() []status.Condition {
	conditions := make([]status.Condition, 0, len(in.Status.Conditions))
	for _, c := range in.Status.Conditions {
		conditions = append(conditions, status.Condition{
			Type:               c.Type,
			Status:             c.Status,
			LastTransitionTime: c.LastTransitionTime,
			Reason:             c.Reason,
			Message:            c.Message,
			ObservedGeneration: c.ObservedGeneration,
		})
	}

	return conditions
}

// SetConditions sets the conditions from status.Conditions for the status.Object interface
func (in *YandexNodeClass) SetConditions(conditions []status.Condition) {
	metav1Conditions := make([]metav1.Condition, 0, len(conditions))
	for _, c := range conditions {
		if c.LastTransitionTime.IsZero() {
			continue
		}
		metav1Conditions = append(metav1Conditions, metav1.Condition{
			Type:               c.Type,
			Status:             metav1.ConditionStatus(c.Status),
			LastTransitionTime: c.LastTransitionTime,
			Reason:             c.Reason,
			Message:            c.Message,
			ObservedGeneration: c.ObservedGeneration,
		})
	}

	in.Status.Conditions = metav1Conditions
}

// YandexNodeClassList contains a list of YandexNodeClass
// +kubebuilder:object:root=true
// +kubebuilder:object:generate=true
type YandexNodeClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []YandexNodeClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&YandexNodeClass{}, &YandexNodeClassList{})
}