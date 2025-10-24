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

package offering

import (
	"context"
	"fmt"

	"github.com/tufitko/karpenter-provider-yandex/pkg/yandex"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"

	"github.com/tufitko/karpenter-provider-yandex/pkg/providers/pricing"
)

type Provider interface {
	InjectOfferings(context.Context, []*cloudprovider.InstanceType, []string) []*cloudprovider.InstanceType
}

type DefaultProvider struct {
	pricingProvider pricing.Provider
	// todo: reservations should be used here
}

func NewDefaultProvider(
	pricingProvider pricing.Provider,
) *DefaultProvider {
	return &DefaultProvider{
		pricingProvider: pricingProvider,
	}
}

func (p *DefaultProvider) InjectOfferings(
	ctx context.Context,
	instanceTypes []*cloudprovider.InstanceType,
	allZones sets.Set[string],
) []*cloudprovider.InstanceType {
	var its []*cloudprovider.InstanceType
	for _, it := range instanceTypes {
		offerings := p.createOfferings(
			ctx,
			it,
			allZones,
		)
		// NOTE: By making this copy one level deep, we can modify the offerings without mutating the results from previous
		// GetInstanceTypes calls. This should still be done with caution - it is currently done here in the provider, and
		// once in the instance provider (filterReservedInstanceTypes)
		its = append(its, &cloudprovider.InstanceType{
			Name:         it.Name,
			Requirements: it.Requirements,
			Offerings:    offerings,
			Capacity:     it.Capacity,
			Overhead:     it.Overhead,
		})
	}
	return its
}

//nolint:gocyclo
func (p *DefaultProvider) createOfferings(
	_ context.Context,
	it *cloudprovider.InstanceType,
	allZones sets.Set[string],
) cloudprovider.Offerings {
	var offerings []*cloudprovider.Offering
	itZones := sets.New(it.Requirements.Get(corev1.LabelTopologyZone).Values()...)

	itName := yandex.InstanceType{}
	_ = itName.FromString(it.Name)

	for zone := range allZones {
		for _, capacityType := range it.Requirements.Get(karpv1.CapacityTypeLabelKey).Values() {
			var price float64
			var hasPrice bool
			switch capacityType {
			case karpv1.CapacityTypeOnDemand:
				price, hasPrice = p.pricingProvider.OnDemandPrice(itName)
			case karpv1.CapacityTypeSpot:
				price, hasPrice = p.pricingProvider.SpotPrice(itName)
			default:
				panic(fmt.Sprintf("invalid capacity type %q in requirements for instance type %q", capacityType, it.Name))
			}
			offering := &cloudprovider.Offering{
				Requirements: scheduling.NewRequirements(
					scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, capacityType),
					scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zone),
				),
				Price:     price,
				Available: hasPrice && itZones.Has(zone),
			}
			offerings = append(offerings, offering)
		}
	}

	return offerings
}
