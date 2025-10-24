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

package operator

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
	"github.com/tufitko/karpenter-provider-yandex/pkg/operator/options"
	"github.com/tufitko/karpenter-provider-yandex/pkg/providers/instancetype/offering"
	"github.com/tufitko/karpenter-provider-yandex/pkg/providers/pricing"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/operator"

	"time"

	"github.com/tufitko/karpenter-provider-yandex/pkg/providers/instancetype"
	"github.com/tufitko/karpenter-provider-yandex/pkg/providers/subnet"
	yandexsdk "github.com/tufitko/karpenter-provider-yandex/pkg/yandex"
)

const (
	DefaultCacheTTL        = 10 * time.Minute
	ValidationCacheTTL     = 10 * time.Minute
	DefaultCleanupInterval = 1 * time.Minute
)

// Operator is injected into the Yandex CloudProvider's factories
type Operator struct {
	*operator.Operator
	SDK                  yandexsdk.SDK
	ValidationCache      *cache.Cache
	InstanceTypeProvider instancetype.Provider
	SubnetProvider       subnet.Provider
}

func NewOperator(ctx context.Context, operator *operator.Operator) (context.Context, *Operator) {
	log := log.FromContext(ctx)

	log.V(1).Info("initializing yandex cloud provider operator")

	sdk, err := yandexsdk.NewSDK(ctx, options.FromContext(ctx).ClusterID)
	if err != nil {
		log.Error(err, "failed to build yandex sdk")
		os.Exit(1)
	}

	maxPodsPerNode, err := sdk.MaxPodsPerNode(ctx)
	if err != nil {
		log.Error(err, "failed to determine max pods per node")
		os.Exit(1)
	}

	azs := sets.New[string]()
	subnets, err := sdk.ListNetworkSubnets(ctx)
	if err != nil {
		log.Error(err, "failed to list network subnets")
		os.Exit(1)
	}

	for _, s := range subnets {
		azs.Insert(s.ZoneId)
	}

	kubeDNSIP, err := KubeDNSIP(ctx, operator.KubernetesInterface)
	if err != nil {
		log.V(1).Info(fmt.Sprintf("unable to detect the IP of the kube-dns service, %s", err))
	} else {
		log.WithValues("kube-dns-ip", kubeDNSIP).V(1).Info("discovered kube dns")
	}

	validationCache := cache.New(ValidationCacheTTL, DefaultCleanupInterval)

	subnetProvider := subnet.NewDefaultProvider(sdk, cache.New(DefaultCacheTTL, DefaultCleanupInterval))
	pricingProvider := pricing.NewDefaultProvider()
	itResolver := instancetype.NewDefaultResolver(maxPodsPerNode)
	offeringProvider := offering.NewDefaultProvider(pricingProvider)
	instanceTypeProvider := instancetype.NewDefaultProvider(itResolver, offeringProvider, azs)

	log.V(1).Info("yandex cloud provider operator initialized")

	return ctx, &Operator{
		Operator:             operator,
		SDK:                  sdk,
		ValidationCache:      validationCache,
		InstanceTypeProvider: instanceTypeProvider,
		SubnetProvider:       subnetProvider,
	}
}

func KubeDNSIP(ctx context.Context, kubernetesInterface kubernetes.Interface) (net.IP, error) {
	if kubernetesInterface == nil {
		return nil, fmt.Errorf("no K8s client provided")
	}
	dnsService, err := kubernetesInterface.CoreV1().Services("kube-system").Get(ctx, "kube-dns", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	kubeDNSIP := net.ParseIP(dnsService.Spec.ClusterIP)
	if kubeDNSIP == nil {
		return nil, fmt.Errorf("parsing cluster IP")
	}
	return kubeDNSIP, nil
}

func SetupIndexers(ctx context.Context, mgr manager.Manager) {
	lo.Must0(mgr.GetFieldIndexer().IndexField(ctx, &karpv1.NodeClaim{}, "status.instanceID", func(o client.Object) []string {
		if o.(*karpv1.NodeClaim).Status.ProviderID == "" {
			return nil
		}
		// Parse Yandex providerID format: "yandex://instance-id"
		providerID := o.(*karpv1.NodeClaim).Status.ProviderID
		if len(providerID) > 9 && providerID[:9] == "yandex://" {
			return []string{providerID[9:]}
		}
		return nil
	}), "failed to setup nodeclaim instanceID indexer")

	lo.Must0(mgr.GetFieldIndexer().IndexField(ctx, &corev1.Node{}, "spec.instanceID", func(o client.Object) []string {
		if o.(*corev1.Node).Spec.ProviderID == "" {
			return nil
		}
		// Parse Yandex providerID format: "yandex://instance-id"
		providerID := o.(*corev1.Node).Spec.ProviderID
		if len(providerID) > 9 && providerID[:9] == "yandex://" {
			return []string{providerID[9:]}
		}
		return nil
	}), "failed to setup node instanceID indexer")
}
