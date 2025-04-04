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

package cloudcapacity

import (
	"context"
	"fmt"
	"math/rand"

	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Provider is responsible for getting cloud capacity information
type Provider struct {
	// Mock implementation, will be replaced with real SDK in the future
	capacityZones map[string]NodeCapacity
}

// NodeCapacity represents the capacity of a Yandex Compute Cloud zone
type NodeCapacity struct {
	Name string
	// Capacity is the total amount of resources available in the zone
	Capacity corev1.ResourceList
	// Allocatable is the amount of resources that can be allocated
	Allocatable corev1.ResourceList
}

// NewProvider creates a new Yandex Cloud capacity provider
func NewProvider(ctx context.Context) (*Provider, error) {
	return &Provider{}, nil
}

// Sync synchronizes the capacity information from the cloud
func (p *Provider) Sync(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("YandexCapacityProvider")
	logger.Info("Syncing capacity information")

	// Define zones for ru-central1 region
	zones := []string{
		"ru-central1-a",
		"ru-central1-b",
		"ru-central1-c",
	}

	capacityZones := make(map[string]NodeCapacity)

	// Create mock capacity data for each zone
	for _, zone := range zones {
		// Base capacity for the zone
		cpuCapacity := float64(500 + rand.Intn(1500))
		memCapacity := float64(2048 + rand.Intn(6144))
		
		// Random usage (30-70%)
		cpuUsage := cpuCapacity * (0.3 + 0.4*rand.Float64())
		memUsage := memCapacity * (0.3 + 0.4*rand.Float64())
		
		capacityZones[zone] = NodeCapacity{
			Name: zone,
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%f", cpuCapacity)),
				corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%fGi", memCapacity)),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%f", cpuCapacity-cpuUsage)),
				corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%fGi", memCapacity-memUsage)),
			},
		}
	}

	p.capacityZones = capacityZones
	logger.Info("Capacity information synced", "zones", len(p.capacityZones))

	return nil
}

// Zones returns the list of available zones
func (p *Provider) Zones() []string {
	zones := make([]string, 0, len(p.capacityZones))
	for zone := range p.capacityZones {
		zones = append(zones, zone)
	}

	return zones
}

// Fit checks if the specified resources can fit in the given zone
func (p *Provider) Fit(zone string, req corev1.ResourceList) bool {
	capacity, ok := p.capacityZones[zone]
	if !ok {
		return false
	}

	return capacity.Allocatable.Cpu().Cmp(*req.Cpu()) >= 0 && 
	       capacity.Allocatable.Memory().Cmp(*req.Memory()) >= 0
}

// GetAvailableZones returns the list of zones that can fit the specified resources
func (p *Provider) GetAvailableZones(req corev1.ResourceList) []string {
	zones := []string{}

	for zone := range p.capacityZones {
		if p.Fit(zone, req) {
			zones = append(zones, zone)
		}
	}

	return zones
}