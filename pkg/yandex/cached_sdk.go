package yandex

import (
	"context"
	"crypto/md5"
	"fmt"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
	"github.com/tufitko/karpenter-provider-yandex/pkg/apis/v1alpha1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	CacheTTL        = 10 * time.Minute
	CacheCleanupTTL = time.Minute
)

type CachedSDK struct {
	SDK
	cache *cache.Cache
}

func NewCachedSDK(sdk SDK) CachedSDK {
	return CachedSDK{
		sdk,
		cache.New(CacheTTL, CacheCleanupTTL),
	}
}

func (c CachedSDK) CreateFixedNodeGroup(
	ctx context.Context,
	name string,
	labels map[string]string,
	nodeLabels map[string]string,
	platformId PlatformId,
	coreFraction CoreFraction,
	cpu resource.Quantity,
	mem resource.Quantity,
	preemptible bool,
	zoneId string,
	subnetId string,
	nodeclass *v1alpha1.YandexNodeClass,
	diskType string,
	diskSize int64,
) (string, error) {
	var methodName = "CreateFixedNodeGroup"
	var key = c.generateMD5CacheKey(methodName, name)

	value, exist := c.cache.Get(key)
	if exist {
		return value.(lo.Tuple2[string, error]).Unpack()
	}

	resp, err := c.CreateFixedNodeGroup(ctx, name, labels, nodeLabels, platformId, coreFraction, cpu, mem, preemptible, zoneId, subnetId, nodeclass, diskType, diskSize)

	c.cache.Set(key, lo.Tuple2[string, error]{A: resp, B: err}, CacheTTL)

	return resp, err
}

func (c CachedSDK) DeleteNodeGroup(ctx context.Context, nodeGroupId string) error {
	var methodName = "DeleteNodeGroup"
	var key = c.generateMD5CacheKey(methodName, nodeGroupId)

	value, exist := c.cache.Get(key)
	if exist {
		return value.(error)
	}

	err := c.DeleteNodeGroup(ctx, nodeGroupId)

	c.cache.Set(key, err, CacheTTL)

	return err

}

func (c CachedSDK) generateMD5CacheKey(method string, args ...string) string {
	key := method
	for _, arg := range args {
		key += fmt.Sprintf("-%s", arg)
	}

	return fmt.Sprintf("%x", md5.Sum([]byte(key)))
}
