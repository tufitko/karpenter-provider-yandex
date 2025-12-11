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

//go:generate go run tools/price_gen.go ru

package pricing

import (
	"github.com/tufitko/karpenter-provider-yandex/pkg/yandex"
)

type Provider interface {
	OnDemandPrice(yandex.InstanceType) (float64, bool)
	SpotPrice(yandex.InstanceType) (float64, bool)
	DiskPrice(yandex.Disk) (float64, bool)
}

type DefaultProvider struct {
	mapping map[yandex.PlatformId]pricingPlatform
}

func NewDefaultProvider() *DefaultProvider {
	p := &DefaultProvider{
		mapping: ruPricing,
	}

	return p
}

// OnDemandPrice returns the last known on-demand price for a given instance type, returning an error if there is no
// known on-demand pricing for the instance type.
func (p *DefaultProvider) OnDemandPrice(instanceType yandex.InstanceType) (float64, bool) {
	platform, ok := p.mapping[instanceType.Platform]
	if !ok {
		return 0, false
	}

	cpuPrice, ok := platform.perFraction[instanceType.CoreFraction]
	if !ok {
		return 0, false
	}
	memPrice := platform.ram

	return cpuPrice*instanceType.CPU.AsApproximateFloat64() + memPrice*(float64(instanceType.Memory.Value())/1024/1024/1024), true
}

// SpotPrice returns the last known spot price for a given instance type, returning an error
// if there is no known spot pricing for that instance type or zone
func (p *DefaultProvider) SpotPrice(instanceType yandex.InstanceType) (float64, bool) {
	platform, ok := p.mapping[instanceType.Platform]
	if !ok {
		return 0, false
	}

	cpuPrice, ok := platform.preemptiblePerFraction[instanceType.CoreFraction]
	if !ok {
		return 0, false
	}
	memPrice := platform.preemptibleRAM

	return cpuPrice*instanceType.CPU.AsApproximateFloat64() + memPrice*(float64(instanceType.Memory.Value())/1024/1024/1024), true
}

func (p *DefaultProvider) DiskPrice(disk yandex.Disk) (float64, bool) {
	price, ok := ruDiskPricing[disk.Type]
	if !ok {
		return 0, false
	}
	return price * float64(disk.Size), true
}
