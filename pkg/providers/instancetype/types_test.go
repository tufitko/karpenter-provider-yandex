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
	"testing"

	"github.com/tufitko/karpenter-provider-yandex/pkg/apis/v1alpha1"
	"github.com/tufitko/karpenter-provider-yandex/pkg/providers/instancetype/offering"
	"github.com/tufitko/karpenter-provider-yandex/pkg/providers/pricing"
	"github.com/tufitko/karpenter-provider-yandex/pkg/yandex"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

func TestNoSpotOfferingsForUnsupportedPlatform(t *testing.T) {
	pricingProvider := pricing.NewDefaultProvider()
	offeringProvider := offering.NewDefaultProvider(pricingProvider)

	resolver := NewDefaultResolver(10)

	// PlatformIntelIceLakeComputeOptimized has CanBePreemptible: false
	instanceTypeInfo := yandex.InstanceType{
		Platform:     yandex.PlatformIntelIceLakeComputeOptimized,
		CPU:          resource.MustParse("2"),
		Memory:       resource.MustParse("4Gi"),
		CoreFraction: yandex.CoreFraction100,
	}

	nodeClass := &v1alpha1.YandexNodeClass{
		Spec: v1alpha1.YandexNodeClassSpec{
			DiskType: string(yandex.SSD),
			DiskSize: resource.MustParse("30Gi"),
		},
		Status: v1alpha1.YandexNodeClassStatus{
			Subnets: []v1alpha1.Subnet{
				{ZoneID: "ru-central1-a"},
				{ZoneID: "ru-central1-b"},
			},
		},
	}

	it := resolver.Resolve(context.Background(), instanceTypeInfo, nodeClass, false)

	// Verify requirements don't include spot
	capacityTypes := it.Requirements.Get(karpv1.CapacityTypeLabelKey).Values()
	if len(capacityTypes) != 1 {
		t.Fatalf("Expected exactly 1 capacity type, got %d: %v", len(capacityTypes), capacityTypes)
	}
	if capacityTypes[0] != karpv1.CapacityTypeOnDemand {
		t.Fatalf("Expected capacity type to be %s, got %s", karpv1.CapacityTypeOnDemand, capacityTypes[0])
	}

	allZones := sets.New("ru-central1-a", "ru-central1-b", "ru-central1-d")

	instanceTypes := []*cloudprovider.InstanceType{it}
	result := offeringProvider.InjectOfferings(context.Background(), instanceTypes, allZones, nodeClass)

	if len(result) != 1 {
		t.Fatalf("Expected 1 instance type, got %d", len(result))
	}

	resultIT := result[0]
	offerings := resultIT.Offerings

	spotOfferings := 0
	onDemandOfferings := 0

	for _, offering := range offerings {
		capacityType := offering.Requirements.Get(karpv1.CapacityTypeLabelKey).Values()
		if len(capacityType) != 1 {
			t.Errorf("Expected exactly 1 capacity type per offering, got %d: %v", len(capacityType), capacityType)
			continue
		}

		switch capacityType[0] {
		case karpv1.CapacityTypeSpot:
			spotOfferings++
			t.Errorf("Found spot offering for platform with CanBePreemptible=false: zone=%s, price=%.4f",
				offering.Zone(), offering.Price)
		case karpv1.CapacityTypeOnDemand:
			onDemandOfferings++
		default:
			t.Errorf("Unexpected capacity type: %s", capacityType[0])
		}
	}

	if spotOfferings > 0 {
		t.Fatalf("Expected 0 spot offerings for platform with CanBePreemptible=false, but found %d", spotOfferings)
	}

	if onDemandOfferings == 0 {
		t.Fatalf("Expected at least 1 on-demand offering, but found 0")
	}

	t.Logf("Platform %s with CanBePreemptible=false: %d on-demand offerings, %d spot offerings (expected 0)",
		instanceTypeInfo.Platform, onDemandOfferings, spotOfferings)
}

func TestSpotOfferingsForSupportedPlatform(t *testing.T) {
	pricingProvider := pricing.NewDefaultProvider()
	offeringProvider := offering.NewDefaultProvider(pricingProvider)

	resolver := NewDefaultResolver(10)

	// PlatformIntelIceLake has CanBePreemptible: true
	instanceTypeInfo := yandex.InstanceType{
		Platform:     yandex.PlatformIntelIceLake,
		CPU:          resource.MustParse("2"),
		Memory:       resource.MustParse("4Gi"),
		CoreFraction: yandex.CoreFraction100,
	}

	nodeClass := &v1alpha1.YandexNodeClass{
		Spec: v1alpha1.YandexNodeClassSpec{
			DiskType: string(yandex.SSD),
			DiskSize: resource.MustParse("30Gi"),
		},
		Status: v1alpha1.YandexNodeClassStatus{
			Subnets: []v1alpha1.Subnet{
				{ZoneID: "ru-central1-a"},
				{ZoneID: "ru-central1-b"},
			},
		},
	}

	it := resolver.Resolve(context.Background(), instanceTypeInfo, nodeClass, true)

	capacityTypes := it.Requirements.Get(karpv1.CapacityTypeLabelKey).Values()
	if len(capacityTypes) != 2 {
		t.Fatalf("Expected exactly 2 capacity types, got %d: %v", len(capacityTypes), capacityTypes)
	}

	hasSpot := false
	hasOnDemand := false
	for _, ct := range capacityTypes {
		if ct == karpv1.CapacityTypeSpot {
			hasSpot = true
		}
		if ct == karpv1.CapacityTypeOnDemand {
			hasOnDemand = true
		}
	}

	if !hasSpot {
		t.Fatalf("Expected spot capacity type in requirements when canBePreemptible=true")
	}
	if !hasOnDemand {
		t.Fatalf("Expected on-demand capacity type in requirements")
	}

	allZones := sets.New("ru-central1-a", "ru-central1-b", "ru-central1-d")

	instanceTypes := []*cloudprovider.InstanceType{it}
	result := offeringProvider.InjectOfferings(context.Background(), instanceTypes, allZones, nodeClass)

	if len(result) != 1 {
		t.Fatalf("Expected 1 instance type, got %d", len(result))
	}

	resultIT := result[0]
	offerings := resultIT.Offerings

	spotOfferings := 0
	onDemandOfferings := 0

	for _, offering := range offerings {
		capacityType := offering.Requirements.Get(karpv1.CapacityTypeLabelKey).Values()
		if len(capacityType) != 1 {
			t.Errorf("Expected exactly 1 capacity type per offering, got %d: %v", len(capacityType), capacityType)
			continue
		}

		switch capacityType[0] {
		case karpv1.CapacityTypeSpot:
			spotOfferings++
		case karpv1.CapacityTypeOnDemand:
			onDemandOfferings++
		default:
			t.Errorf("Unexpected capacity type: %s", capacityType[0])
		}
	}

	if spotOfferings == 0 {
		t.Errorf("Expected at least 1 spot offering for platform with CanBePreemptible=true, but found 0")
	}

	if onDemandOfferings == 0 {
		t.Fatalf("Expected at least 1 on-demand offering, but found 0")
	}

	t.Logf("Platform %s with CanBePreemptible=true: %d on-demand offerings, %d spot offerings",
		instanceTypeInfo.Platform, onDemandOfferings, spotOfferings)
}
