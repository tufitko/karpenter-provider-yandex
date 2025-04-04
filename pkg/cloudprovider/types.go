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

package yandex

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/sergelogvinov/karpenter-provider-proxmox/pkg/apis/v1alpha1"
	"github.com/sergelogvinov/karpenter-provider-proxmox/pkg/providers/cloudcapacity"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/resource"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
)

// ConstructInstanceTypes creates Yandex Cloud instance types
func ConstructInstanceTypes(ctx context.Context, cloudcapacityProvider *cloudcapacity.Provider) ([]*cloudprovider.InstanceType, error) {
	var instanceTypes []*cloudprovider.InstanceType

	// Define Yandex instance families
	families := []string{"s", "m", "c", "g"} // standard, memory, compute, gpu
	
	// Generate instance types for each family
	for _, family := range families {
		gpuTypes := []string{""}
		isGPU := false
		
		if family == "g" {
			gpuTypes = []string{"nvidia-tesla-v100", "nvidia-tesla-a100"}
			isGPU = true
		}
		
		for _, gpuType := range gpuTypes {
			for _, cpu := range []int{2, 4, 8, 16, 32, 64, 96} {
				// Memory factor varies by family
				memFactor := 4 // default for standard
				switch family {
				case "m":
					memFactor = 8 // memory-optimized
				case "c":
					memFactor = 2 // compute-optimized
				case "g":
					memFactor = 4 // gpu instances
				}
				
				mem := cpu * memFactor
				
				// Construct instance type name
				var name string
				if isGPU {
					gpuCount := cpu / 8
					if gpuCount < 1 {
						gpuCount = 1
					}
					// For GPU instances, add GPU info to name
					name = fmt.Sprintf("%s2.%dvcpu.%dgb-%s-%d", family, cpu, mem, strings.Split(gpuType, "-")[1], gpuCount)
				} else {
					name = fmt.Sprintf("%s2.%dvcpu.%dgb", family, cpu, mem)
				}
				
				// Create instance type
				instanceType := cloudprovider.InstanceType{
					Name: name,
					Capacity: corev1.ResourceList{
						corev1.ResourceCPU:              resource.MustParse(fmt.Sprintf("%d", cpu)),
						corev1.ResourceMemory:           resource.MustParse(fmt.Sprintf("%dGi", mem)),
						corev1.ResourcePods:             resource.MustParse("110"),
						corev1.ResourceEphemeralStorage: resource.MustParse("100Gi"),
					},
					Overhead: &cloudprovider.InstanceTypeOverhead{
						KubeReserved:   KubeReservedResources(int64(cpu), float64(mem)),
						SystemReserved: SystemReservedResources(),
					},
				}
				
				// Add GPU resources if applicable
				if isGPU {
					gpuCount := cpu / 8
					if gpuCount < 1 {
						gpuCount = 1
					}
					
					instanceType.Capacity[resourceNameForGPU(gpuType)] = resource.MustParse(fmt.Sprintf("%d", gpuCount))
				}
				
				// Create offerings
				createOfferings(cloudcapacityProvider, &instanceType, family, gpuType, isGPU)
				
				instanceTypes = append(instanceTypes, &instanceType)
			}
		}
	}

	return instanceTypes, nil
}

func instanceTypeByName(instanceTypes []*cloudprovider.InstanceType, name string) (*cloudprovider.InstanceType, error) {
	for _, instanceType := range instanceTypes {
		if instanceType.Name == name {
			return instanceType, nil
		}
	}

	return nil, fmt.Errorf("instance type not found")
}

func resourceNameForGPU(gpuType string) corev1.ResourceName {
	switch gpuType {
	case "nvidia-tesla-v100":
		return corev1.ResourceName("nvidia.com/tesla-v100")
	case "nvidia-tesla-a100":
		return corev1.ResourceName("nvidia.com/tesla-a100")
	default:
		return corev1.ResourceName("nvidia.com/gpu")
	}
}

func priceFromResources(resources corev1.ResourceList, family string) float64 {
	// Set base price multipliers according to family
	cpuPrice := 0.0333 // base price per CPU
	memPrice := 0.0044 // base price per GB
	
	switch family {
	case "c":
		cpuPrice = 0.04
		memPrice = 0.0033
	case "m":
		cpuPrice = 0.027
		memPrice = 0.0055
	case "g":
		cpuPrice = 0.05
		memPrice = 0.0044
	}
	
	price := 0.0
	for k, v := range resources {
		switch k {
		case corev1.ResourceCPU:
			price += cpuPrice * v.AsApproximateFloat64()
		case corev1.ResourceMemory:
			price += memPrice * v.AsApproximateFloat64() / (1e9)
		case corev1.ResourceName("nvidia.com/tesla-v100"):
			price += 2.0 * v.AsApproximateFloat64()
		case corev1.ResourceName("nvidia.com/tesla-a100"):
			price += 3.5 * v.AsApproximateFloat64()
		case corev1.ResourceName("nvidia.com/gpu"):
			price += 1.5 * v.AsApproximateFloat64()
		}
	}

	return price
}

func SystemReservedResources() corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("100m"),
		corev1.ResourceMemory: resource.MustParse("256Mi"),
	}
}

func KubeReservedResources(cpu int64, mem float64) corev1.ResourceList {
	// Dynamically scale reservations based on instance size
	cpuReserved := resource.MustParse(fmt.Sprintf("%dm", 50+25*cpu))
	memReserved := resource.MustParse(fmt.Sprintf("%dMi", 256+int64(mem*0.001)))
	
	return corev1.ResourceList{
		corev1.ResourceCPU:    cpuReserved,
		corev1.ResourceMemory: memReserved,
	}
}

func createOfferings(cloudcapacityProvider *cloudcapacity.Provider, instanceType *cloudprovider.InstanceType, family, gpuType string, isGPU bool) {
	zones := cloudcapacityProvider.Zones()
	price := priceFromResources(instanceType.Capacity, family)

	instanceType.Offerings = []cloudprovider.Offering{}

	for _, zone := range zones {
		available := cloudcapacityProvider.Fit(zone, instanceType.Capacity)
		
		// Create base requirements
		requirements := []scheduling.Requirement{
			scheduling.NewRequirement(corev1.LabelInstanceTypeStable, corev1.NodeSelectorOpIn, instanceType.Name),
			scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zone),
			scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, karpv1.CapacityTypeOnDemand),
			scheduling.NewRequirement(v1alpha1.LabelInstanceFamily, corev1.NodeSelectorOpIn, family),
			scheduling.NewRequirement(v1alpha1.LabelInstanceCPUPlatform, corev1.NodeSelectorOpIn, "intel-cascade-lake"),
		}
		
		// Add GPU requirements if applicable
		if isGPU {
			requirements = append(requirements, 
				scheduling.NewRequirement(v1alpha1.LabelInstanceGPUType, corev1.NodeSelectorOpIn, gpuType))
		}

		instanceType.Offerings = append(instanceType.Offerings, cloudprovider.Offering{
			Price:        price,
			Available:    available,
			Requirements: scheduling.NewRequirements(requirements...),
		})
	}
}