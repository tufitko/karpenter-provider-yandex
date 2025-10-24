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

package subnet

import (
	"context"
	"fmt"
	"math"
	"net"
	"sort"
	"sync"

	"github.com/tufitko/karpenter-provider-yandex/pkg/apis/v1alpha1"
	"github.com/tufitko/karpenter-provider-yandex/pkg/yandex"

	"github.com/mitchellh/hashstructure/v2"
	"github.com/patrickmn/go-cache"
)

type Provider interface {
	List(context.Context, *v1alpha1.YandexNodeClass) ([]Subnet, error)
}

type DefaultProvider struct {
	sync.Mutex
	api   yandex.SDK
	cache *cache.Cache
}

type Subnet struct {
	ID                      string
	ZoneID                  string
	AvailableIPAddressCount int
}

func NewDefaultProvider(api yandex.SDK, cache *cache.Cache) *DefaultProvider {
	return &DefaultProvider{
		api:   api,
		cache: cache,
	}
}

func (p *DefaultProvider) List(ctx context.Context, nodeClass *v1alpha1.YandexNodeClass) ([]Subnet, error) {
	p.Lock()
	defer p.Unlock()

	hash, err := hashstructure.Hash(nodeClass.Spec.SubnetSelectorTerms, hashstructure.FormatV2, &hashstructure.HashOptions{SlicesAsSets: true})
	if err != nil {
		return nil, err
	}

	if subnets, ok := p.cache.Get(fmt.Sprint(hash)); ok {
		return append([]Subnet{}, subnets.([]Subnet)...), nil
	}

	subnets, err := p.api.ListNetworkSubnets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list subnets: %w", err)
	}

	subs := make([]Subnet, 0)

	for _, subnet := range subnets {
		keep := false
		for _, term := range nodeClass.Spec.SubnetSelectorTerms {
			if term.ID != "" && subnet.Id == term.ID {
				keep = true
				break
			}
			if len(term.Labels) == 0 {
				continue
			}
			if yandex.MatchLabels(subnet.Labels, term.Labels) {
				keep = true
				break
			}
		}
		if !keep {
			continue
		}

		var inUseIPs int
		inUseIPs, err = p.api.UsedIPsInSubnet(ctx, subnet.Id)
		if err != nil {
			return nil, fmt.Errorf("failed to list used ips: %w", err)
		}

		var totalIPs int
		for _, cidr := range subnet.V4CidrBlocks {
			var c int
			c, err = calculateIPs(cidr)
			if err != nil {
				return nil, fmt.Errorf("failed to calculate ips: %w", err)
			}
			totalIPs += c
		}

		subs = append(subs, Subnet{
			ID:                      subnet.Id,
			ZoneID:                  subnet.ZoneId,
			AvailableIPAddressCount: totalIPs - inUseIPs,
		})
	}

	sort.Slice(subs, func(i, j int) bool {
		if subs[i].AvailableIPAddressCount == subs[j].AvailableIPAddressCount {
			return subs[i].ZoneID < subs[j].ZoneID
		}
		return subs[i].AvailableIPAddressCount > subs[j].AvailableIPAddressCount
	})

	p.cache.SetDefault(fmt.Sprint(hash), subs)
	return subs, nil
}

// calculateIPs calculates the number of IP addresses that can be used in a CIDR subnet.
func calculateIPs(cidr string) (int, error) {
	_, ipv4Net, err := net.ParseCIDR(cidr)
	if err != nil {
		return 0, err
	}
	maskSize, _ := ipv4Net.Mask.Size()

	totalIPs := int(math.Pow(2, float64(32-maskSize))) - 2
	if totalIPs < 0 {
		totalIPs = 0 // Handles the case of subnets with masks /31 and /32
	}
	return totalIPs, nil
}
