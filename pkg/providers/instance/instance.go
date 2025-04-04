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

package instance

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tufitko/karpenter-provider-yandex/pkg/apis/v1alpha1"
	"github.com/tufitko/karpenter-provider-yandex/pkg/providers/cloudcapacity"

	"sigs.k8s.io/controller-runtime/pkg/log"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
)

// Provider is the Yandex Cloud instance provider
type Provider struct {
	// Mock implementation, will be replaced with real SDK in the future
	instances         map[string]*corev1.Node
	cloudcapacityProvider *cloudcapacity.Provider
}

// NewProvider creates a new Yandex Cloud instance provider
func NewProvider(cloudcapacityProvider *cloudcapacity.Provider) (*Provider, error) {
	return &Provider{
		instances:         make(map[string]*corev1.Node),
		cloudcapacityProvider: cloudcapacityProvider,
	}, nil
}

// Create creates a new instance
func (p *Provider) Create(ctx context.Context, nodeClaim *karpv1.NodeClaim, nodeClass *v1alpha1.YandexNodeClass, instanceTypes []*cloudprovider.InstanceType) (*corev1.Node, error) {
	instanceTypes = orderInstanceTypesByPrice(instanceTypes, scheduling.NewNodeSelectorRequirementsWithMinValues(nodeClaim.Spec.Requirements...))
	instanceType := instanceTypes[0]

	logger := log.FromContext(ctx).WithName("YandexInstanceProvider")
	logger.Info("Creating instance", "nodeClaim", nodeClaim.Name, "instanceType", instanceType.Name)

	// Determine zone
	zone := nodeClass.Spec.Zone
	if zone == "" {
		requestedZones := scheduling.NewNodeSelectorRequirementsWithMinValues(nodeClaim.Spec.Requirements...).Get(corev1.LabelTopologyZone)
		zone = requestedZones.Any()
		if len(requestedZones.Values()) == 0 || zone == "" {
			zones := p.cloudcapacityProvider.GetAvailableZones(instanceType.Capacity)
			if len(zones) == 0 {
				return nil, cloudprovider.NewInsufficientCapacityError(fmt.Errorf("no capacity zone available"))
			}
			zone = zones[0]
		}
	}

	// Generate a mock provider ID for Yandex Cloud
	providerID := fmt.Sprintf("yandex://mock-folder/%s/instances/%s", nodeClass.Spec.FolderID, nodeClaim.Name)

	// Create a mock node
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeClaim.Name,
			Labels: map[string]string{
				corev1.LabelTopologyRegion:           "ru-central1",
				corev1.LabelTopologyZone:             zone,
				corev1.LabelInstanceTypeStable:       instanceType.Name,
				karpv1.CapacityTypeLabelKey:          karpv1.CapacityTypeOnDemand,
				v1alpha1.LabelInstanceFamily:         strings.Split(instanceType.Name, ".")[0],
				v1alpha1.LabelInstanceCPUPlatform:    "intel-cascade-lake",
			},
			Annotations: map[string]string{
				v1alpha1.AnnotationImageID:    nodeClass.Spec.ImageID,
				v1alpha1.AnnotationFolderID:   nodeClass.Spec.FolderID,
				v1alpha1.AnnotationInstanceID: fmt.Sprintf("mock-instance-%s", nodeClaim.Name),
			},
			CreationTimestamp: metav1.Now(),
		},
		Spec: corev1.NodeSpec{
			ProviderID: providerID,
			Taints:     []corev1.Taint{karpv1.UnregisteredNoExecuteTaint},
		},
		Status: corev1.NodeStatus{
			NodeInfo: corev1.NodeSystemInfo{
				Architecture:    karpv1.ArchitectureAmd64,
				OperatingSystem: string(corev1.Linux),
				KernelVersion:   "5.4.0-generic",
				OSImage:         "Ubuntu 20.04 LTS",
				KubeletVersion:  "v1.26.0",
			},
		},
	}

	// Add additional labels based on the instance type
	if strings.Contains(instanceType.Name, "gpu") || strings.HasPrefix(instanceType.Name, "g") {
		// Extract GPU information from the instance type
		for resName, quantity := range instanceType.Capacity {
			if strings.Contains(string(resName), "nvidia.com") {
				// Format like "nvidia-tesla-v100"
				gpuType := strings.Replace(string(resName), "nvidia.com/", "nvidia-", 1)
				if gpuType == "nvidia-gpu" {
					gpuType = "nvidia-tesla-v100" // Default if specific model not specified
				}
				
				node.Labels[v1alpha1.LabelInstanceGPUType] = gpuType
				node.Labels[v1alpha1.LabelInstanceGPUCount] = quantity.String()
				break
			}
		}
	}

	// Add custom labels if provided
	for k, v := range nodeClass.Spec.Labels {
		node.Labels[k] = v
	}

	// Store the node in our mock database
	p.instances[providerID] = node

	// Simulate creation delay for realism
	time.Sleep(100 * time.Millisecond)

	return node, nil
}

// Get retrieves an instance by its provider ID
func (p *Provider) Get(ctx context.Context, providerID string) (*corev1.Node, error) {
	logger := log.FromContext(ctx).WithName("YandexInstanceProvider")
	logger.Info("Getting instance", "providerID", providerID)

	// Check if the instance exists in our mock database
	node, ok := p.instances[providerID]
	if !ok {
		return nil, fmt.Errorf("instance not found")
	}

	return node, nil
}

// Delete deletes an instance
func (p *Provider) Delete(ctx context.Context, nodeClaim *karpv1.NodeClaim) error {
	logger := log.FromContext(ctx).WithName("YandexInstanceProvider")
	logger.Info("Deleting instance", "nodeClaim", nodeClaim.Name)

	providerID := nodeClaim.Status.ProviderID
	if providerID == "" {
		return fmt.Errorf("providerID is empty")
	}

	// Check if the instance exists
	_, ok := p.instances[providerID]
	if !ok {
		return cloudprovider.NewNodeClaimNotFoundError(fmt.Errorf("instance not found"))
	}

	// Delete the instance from our mock database
	delete(p.instances, providerID)

	// Simulate deletion delay for realism
	time.Sleep(100 * time.Millisecond)

	return nil
}

// orderInstanceTypesByPrice orders instance types by price
func orderInstanceTypesByPrice(instanceTypes []*cloudprovider.InstanceType, requirements scheduling.Requirements) []*cloudprovider.InstanceType {
	// Order instance types so that we get the cheapest instance types of the available offerings
	sort.Slice(instanceTypes, func(i, j int) bool {
		iPrice := math.MaxFloat64
		jPrice := math.MaxFloat64
		if len(instanceTypes[i].Offerings.Available().Compatible(requirements)) > 0 {
			iPrice = instanceTypes[i].Offerings.Available().Compatible(requirements).Cheapest().Price
		}
		if len(instanceTypes[j].Offerings.Available().Compatible(requirements)) > 0 {
			jPrice = instanceTypes[j].Offerings.Available().Compatible(requirements).Cheapest().Price
		}
		if iPrice == jPrice {
			return instanceTypes[i].Name < instanceTypes[j].Name
		}
		return iPrice < jPrice
	})

	return instanceTypes
}