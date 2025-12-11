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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ConditionTypeSubnetsReady        = "SubnetsReady"
	ConditionTypeSecurityGroupsReady = "SecurityGroupsReady"
	ConditionTypeValidationSucceeded = "ValidationSucceeded"
)

// YandexNodeClassSpec is the specification for a YandexNodeClass
type YandexNodeClassSpec struct {
	// Platform is the platform of the nodes
	// Default is "standard-v3"
	// +kubebuilder:validation:Enum:=standard-v1;standard-v2;standard-v3
	// +kubebuilder:default=standard-v3
	// +optional
	Platform string `json:"platform"`

	// CanBePreemptible determines if the nodes can be preemptible
	// By default, nodes are not preemptible
	// +kubebuilder:default=false
	// +optional
	CanBePreemptible *bool `json:"can_be_preemptible,omitempty"`

	// CoreFractions is the list of core fractions to use for the nodes
	// If not specified, the default core fraction of 100% will be used
	// +optional
	CoreFractions []CoreFraction `json:"core_fractions,omitempty"`

	// SubnetSelectorTerms is a list of subnet selector terms. The terms are ORed.
	// +kubebuilder:validation:XValidation:message="subnetSelectorTerms cannot be empty",rule="self.size() != 0"
	// +kubebuilder:validation:XValidation:message="expected at least one, got none, ['labels', 'id']",rule="self.all(x, has(x.labels) || has(x.id))"
	// +kubebuilder:validation:XValidation:message="'id' is mutually exclusive, cannot be set with a combination of other fields in a subnet selector term",rule="!self.all(x, has(x.id) && has(x.labels))"
	// +kubebuilder:validation:MaxItems:=30
	// +required
	SubnetSelectorTerms []SubnetSelectorTerm `json:"subnetSelectorTerms" hash:"ignore"`

	// DiskType is the type of disk to create
	// Valid values are:
	// - "network-hdd"
	// - "network-ssd" (default)
	// - "network-ssd-nonreplicated"
	// - "network-ssd-io-m3"
	// +optional
	// +kubebuilder:validation:Enum=network-hdd;network-ssd;network-ssd-nonreplicated;network-ssd-io-m3
	// +kubebuilder:default=network-ssd
	DiskType string `json:"diskType,omitempty"`

	// DiskSize is the size of the booted disk
	// +optional
	// +kubebuilder:default="30Gi"
	DiskSize resource.Quantity `json:"diskSize,omitempty"`

	// Labels to apply to the VMs
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// NodeLabels is additional labels on node
	// +optional
	NodeLabels map[string]string `json:"nodeLabels,omitempty"`

	// SecurityGroups to apply to the VMs
	// +optional
	SecurityGroups []string `json:"securityGroups,omitempty"`
}

// CoreFraction is a string representation of a core fraction
// +kubebuilder:validation:Enum=5;20;50;100
type CoreFraction string

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

// SubnetSelectorTerm defines selection logic for a subnet used by Karpenter to launch nodes.
// If multiple fields are used for selection, the requirements are ANDed.
type SubnetSelectorTerm struct {
	// Tags is a map of key/value tags used to select subnets
	// Specifying '*' for a value selects all values for a given tag key.
	// +kubebuilder:validation:XValidation:message="empty label keys or values aren't supported",rule="self.all(k, k != '' && self[k] != '')"
	// +kubebuilder:validation:MaxProperties:=20
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// ID is the subnet id in Yandex Cloud
	// +optional
	ID string `json:"id,omitempty"`
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

// MetadataOptions contains parameters for specifying VM metadata
type MetadataOptions struct {
	// UserData is base64-encoded user-data to be made available to the instance
	// +optional
	UserData string `json:"userData,omitempty"`
}

// YandexNodeClassStatus defines the observed state of YandexNodeClass
type YandexNodeClassStatus struct {
	// Subnets contains the current subnet values that are available to the
	// cluster under the subnet selectors.
	// +optional
	Subnets []Subnet `json:"subnets,omitempty"`

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

// Subnet contains resolved Subnet selector values utilized for node launch
type Subnet struct {
	// ID of the subnet
	// +required
	ID string `json:"id"`
	// The associated availability zone ID
	// +optional
	ZoneID string `json:"zoneID,omitempty"`
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
