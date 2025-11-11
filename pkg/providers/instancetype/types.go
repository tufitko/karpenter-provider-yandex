/*
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

package instancetype

import (
	"context"
	"fmt"
	"math"
	"regexp"

	"github.com/samber/lo"
	"github.com/tufitko/karpenter-provider-yandex/pkg/apis/v1alpha1"
	"github.com/tufitko/karpenter-provider-yandex/pkg/yandex"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
)

const (
	MemoryAvailable = "memory.available"
	NodeFSAvailable = "nodefs.available"
)

var (
	instanceTypeScheme = regexp.MustCompile(`(^[a-z]+)(\-[0-9]+tb)?([0-9]+).*\.`)
)

type InstanceConfiguration struct {
	CoreFraction     yandex.CoreFraction
	VCPU             []int
	MemoryPerCore    []float64
	CanBePreemptible bool
}

type ZoneData struct {
	Name      string
	ID        string
	Available bool
}

type Resolver interface {
	// Resolve generates an InstanceType based on raw InstanceTypeInfo and NodeClass setting data
	Resolve(ctx context.Context, info yandex.InstanceType, nodeClass *v1alpha1.YandexNodeClass) *cloudprovider.InstanceType
}

type DefaultResolver struct {
	maxPodsPerNode int
}

func NewDefaultResolver(maxPodsPerNode int) *DefaultResolver {
	return &DefaultResolver{
		maxPodsPerNode: maxPodsPerNode,
	}
}

func (d *DefaultResolver) Resolve(ctx context.Context, info yandex.InstanceType, nodeClass *v1alpha1.YandexNodeClass) *cloudprovider.InstanceType {
	return NewInstanceType(
		ctx,
		info,
		nodeClass,
		d.maxPodsPerNode,
	)
}

func NewInstanceType(
	ctx context.Context,
	info yandex.InstanceType,
	nodeClass *v1alpha1.YandexNodeClass,
	maxPods int,
) *cloudprovider.InstanceType {
	it := &cloudprovider.InstanceType{
		Name:         info.String(),
		Requirements: computeRequirements(info, nodeClass),
		Capacity:     computeCapacity(ctx, info, nodeClass.Spec.DiskSize, maxPods),
		Offerings:    cloudprovider.Offerings{}, // Initialize empty offerings to prevent panic
		Overhead: &cloudprovider.InstanceTypeOverhead{
			KubeReserved:      kubeReservedResources(info.CPU, info.Memory),
			SystemReserved:    corev1.ResourceList{},
			EvictionThreshold: evictionThreshold(nodeClass.Spec.DiskSize),
		},
	}
	return it
}

//nolint:gocyclo
func computeRequirements(
	info yandex.InstanceType,
	nodeClass *v1alpha1.YandexNodeClass,
) scheduling.Requirements {
	capacityTypes := []string{karpv1.CapacityTypeOnDemand}
	if nodeClass.Spec.CanBePreemptible != nil && *nodeClass.Spec.CanBePreemptible {
		capacityTypes = append(capacityTypes, karpv1.CapacityTypeSpot)
	}

	// Available zones is the set intersection between zones where the instance type is available, and zones which are
	// available via the provided EC2NodeClass.
	availableZones := lo.Map(nodeClass.Status.Subnets, func(item v1alpha1.Subnet, index int) string {
		return item.ZoneID
	})
	requirements := scheduling.NewRequirements(
		// Well Known Upstream
		scheduling.NewRequirement(corev1.LabelInstanceTypeStable, corev1.NodeSelectorOpIn, string(info.Platform)),
		scheduling.NewRequirement(corev1.LabelInstanceType, corev1.NodeSelectorOpIn, string(info.Platform)),
		scheduling.NewRequirement(corev1.LabelArchStable, corev1.NodeSelectorOpIn, "amd64"),
		scheduling.NewRequirement(corev1.LabelArchStable, corev1.NodeSelectorOpIn, "amd64"),
		scheduling.NewRequirement(corev1.LabelOSStable, corev1.NodeSelectorOpIn, "linux"),
		scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, availableZones...),
		scheduling.NewRequirement(corev1.LabelFailureDomainBetaZone, corev1.NodeSelectorOpIn, availableZones...),
		// Well Known to Karpenter
		scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, capacityTypes...),
		// Well Known to Yandex
		scheduling.NewRequirement("yandex.cloud/pci-topology", corev1.NodeSelectorOpIn, "k8s"),
		scheduling.NewRequirement("yandex.cloud/preemptible", corev1.NodeSelectorOpIn, "true", "false"),
	)

	return requirements
}

func computeCapacity(_ context.Context, info yandex.InstanceType, diskSize resource.Quantity, podsPerCore int) corev1.ResourceList {
	resourceList := corev1.ResourceList{
		corev1.ResourceCPU:              info.CPU,
		corev1.ResourceMemory:           info.Memory,
		corev1.ResourceEphemeralStorage: diskSize,
		corev1.ResourcePods:             *resource.NewQuantity(int64(podsPerCore), resource.DecimalSI),
	}
	return resourceList
}

func kubeReservedResources(cpu, memory resource.Quantity) corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceMemory:           kubeReservedMemory(memory),
		corev1.ResourceCPU:              kubeReservedCPU(cpu),
		corev1.ResourceEphemeralStorage: kubeReservedEphemeralStorage(),
	}
}

func kubeReservedMemory(mem resource.Quantity) resource.Quantity {
	gi1 := resource.MustParse("1Gi")
	if mem.Cmp(gi1) < 0 {
		return resource.MustParse("255Mi")
	}

	reserved := float64(0)
	m := float64(mem.Value())
	gi := float64(gi1.Value())

	reserved += min(m, 4*gi) * 0.25
	reserved += min(max(m-4*gi, 0), 4*gi) * 0.20
	reserved += min(max(m-8*gi, 0), 8*gi) * 0.10
	reserved += min(max(m-16*gi, 0), 112*gi) * 0.06
	reserved += max(m-128*gi, 0) * 0.02

	return *resource.NewQuantity(int64(reserved), resource.BinarySI)
}

func kubeReservedCPU(cpu resource.Quantity) resource.Quantity {
	// 1 CPU = 1 Core
	cores := float64(cpu.MilliValue() / 1000)
	reserved := float64(0)

	if cores > 0 {
		reserved += min(cores, 1) * 0.06
	}
	if cores > 1 {
		reserved += min(cores-1, 1) * 0.01
	}
	if cores > 2 {
		reserved += min(cores-2, 2) * 0.005
	}
	if cores > 4 {
		reserved += max(cores-4, 0) * 0.0025
	}

	return *resource.NewMilliQuantity(int64(math.Round(reserved*1000)), resource.DecimalSI)
}

func kubeReservedEphemeralStorage() resource.Quantity {
	return resource.MustParse("15Gi") // fixed?
}

func evictionThreshold(storage resource.Quantity) corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceMemory:           resource.MustParse("100Mi"),
		corev1.ResourceEphemeralStorage: resource.MustParse(fmt.Sprint(math.Ceil(float64(storage.Value()) / 100 * 10))),
	}
}
