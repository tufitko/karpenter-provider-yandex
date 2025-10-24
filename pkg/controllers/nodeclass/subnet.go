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

package nodeclass

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/lo"
	"github.com/tufitko/karpenter-provider-yandex/pkg/apis/v1alpha1"
	"github.com/tufitko/karpenter-provider-yandex/pkg/providers/subnet"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Subnet struct {
	subnetProvider subnet.Provider
}

func NewSubnetReconciler(subnetProvider subnet.Provider) *Subnet {
	return &Subnet{
		subnetProvider: subnetProvider,
	}
}

func (s *Subnet) Reconcile(ctx context.Context, nodeClass *v1alpha1.YandexNodeClass) (reconcile.Result, error) {
	subnets, err := s.subnetProvider.List(ctx, nodeClass)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("getting subnets, %w", err)
	}
	if len(subnets) == 0 {
		nodeClass.Status.Subnets = nil
		nodeClass.StatusConditions().SetFalse(v1alpha1.ConditionTypeSubnetsReady, "SubnetsNotFound", "SubnetSelector did not match any Subnets")
		// If users have omitted the necessary tags from their Subnets and later add them, we need to reprocess the information.
		// Returning 'ok' in this case means that the nodeclass will remain in an unready state until the component is restarted.
		return reconcile.Result{RequeueAfter: time.Minute}, nil
	}

	nodeClass.Status.Subnets = lo.Map(subnets, func(sub subnet.Subnet, _ int) v1alpha1.Subnet {
		return v1alpha1.Subnet{
			ID:     sub.ID,
			ZoneID: sub.ZoneID,
		}
	})
	nodeClass.StatusConditions().SetTrue(v1alpha1.ConditionTypeSubnetsReady)
	return reconcile.Result{RequeueAfter: time.Minute}, nil
}
