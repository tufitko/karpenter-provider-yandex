package instancetype

//go:generate go run tools/config_gen.go ru

import (
	"context"
	"fmt"
	"sort"

	"github.com/tufitko/karpenter-provider-yandex/pkg/apis/v1alpha1"
	"github.com/tufitko/karpenter-provider-yandex/pkg/providers/instancetype/offering"
	"github.com/tufitko/karpenter-provider-yandex/pkg/yandex"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

type Provider interface {
	List(ctx context.Context, class *v1alpha1.YandexNodeClass) ([]*cloudprovider.InstanceType, error)
	GetInstanceType(ctx context.Context, class *v1alpha1.YandexNodeClass, instanceTypeName string) (*cloudprovider.InstanceType, error)
}

type DefaultProvider struct {
	configuration     map[yandex.PlatformId][]InstanceConfiguration
	offeringProvider  *offering.DefaultProvider
	resolver          Resolver
	allZones          sets.Set[string]
	namesInstanceType map[string]yandex.InstanceType
}

func NewDefaultProvider(resolver Resolver, offeringProvider *offering.DefaultProvider, allZones sets.Set[string]) *DefaultProvider {
	p := &DefaultProvider{
		configuration:    ruAvailableConfigurations,
		resolver:         resolver,
		offeringProvider: offeringProvider,
		allZones:         allZones,
	}

	p.namesInstanceType = p.buildNamesInstanceType()

	return p
}

func (p *DefaultProvider) List(ctx context.Context, class *v1alpha1.YandexNodeClass) ([]*cloudprovider.InstanceType, error) {
	if class == nil {
		return nil, fmt.Errorf("node class is required")
	}

	res := make([]*cloudprovider.InstanceType, 0)
	for platform := range p.configuration {
		types, err := p.generateTypesFor(ctx, platform, class)
		if err != nil {
			return nil, err
		}
		res = append(res, types...)
	}

	sort.Slice(res, func(i, j int) bool {
		return res[i].Offerings.Cheapest().Price < res[j].Offerings.Cheapest().Price
	})
	return res, nil
}

func (p *DefaultProvider) GetInstanceType(ctx context.Context, class *v1alpha1.YandexNodeClass, instanceTypeName string) (*cloudprovider.InstanceType, error) {
	if class == nil {
		return nil, fmt.Errorf("node class is required")
	}

	base, ok := p.namesInstanceType[instanceTypeName]

	if !ok {
		return nil, fmt.Errorf("instance type %s not found", instanceTypeName)
	}

	resolved := p.resolver.Resolve(ctx, base, class)

	withOfferings := p.offeringProvider.InjectOfferings(ctx, []*cloudprovider.InstanceType{resolved}, p.allZones, class)
	if len(withOfferings) == 0 {
		return nil, fmt.Errorf("no offerings for instance type %s", instanceTypeName)
	}

	return withOfferings[0], nil
}

func (p *DefaultProvider) generateTypesFor(ctx context.Context, platform yandex.PlatformId, class *v1alpha1.YandexNodeClass) ([]*cloudprovider.InstanceType, error) {
	res := make([]*cloudprovider.InstanceType, 0)
	for _, configuration := range p.configuration[platform] {
		types := p.generateInstanceTypes(platform, configuration)

		for _, t := range types {
			res = append(res, p.resolver.Resolve(ctx, t, class, configuration.CanBePreemptible))
		}
	}
	return p.offeringProvider.InjectOfferings(ctx, res, p.allZones, class), nil
}

func (p *DefaultProvider) generateInstanceTypes(platform yandex.PlatformId, configuration InstanceConfiguration) []yandex.InstanceType {
	res := make([]yandex.InstanceType, 0)
	for _, cpu := range configuration.VCPU {
		for _, memPerCore := range configuration.MemoryPerCore {
			res = append(res, yandex.InstanceType{
				Platform:     platform,
				CoreFraction: configuration.CoreFraction,
				CPU:          resource.MustParse(fmt.Sprintf("%d", cpu)),
				Memory:       resource.MustParse(fmt.Sprintf("%fGi", memPerCore*float64(cpu))),
			})
		}
	}
	return res
}

func (p *DefaultProvider) buildNamesInstanceType() map[string]yandex.InstanceType {
	names := make(map[string]yandex.InstanceType)
	for platform, configs := range p.configuration {
		for _, configuration := range configs {
			types := p.generateInstanceTypes(platform, configuration)
			for _, t := range types {
				name := t.String()
				names[name] = t
			}
		}
	}
	return names
}
