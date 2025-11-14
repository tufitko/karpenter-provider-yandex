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
	"strings"
	"time"

	"github.com/mitchellh/hashstructure/v2"
	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
	"github.com/tufitko/karpenter-provider-yandex/pkg/apis/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	requeueAfterTime                    = 10 * time.Minute
	ConditionReasonDependenciesNotReady = "DependenciesNotReady"
)

type Validation struct {
	kubeClient     client.Client
	cache          *cache.Cache
	dryRunDisabled bool
}

func NewValidationReconciler(
	kubeClient client.Client,
	cache *cache.Cache,
	dryRunDisabled bool,
) *Validation {
	return &Validation{
		kubeClient:     kubeClient,
		cache:          cache,
		dryRunDisabled: dryRunDisabled,
	}
}

// nolint:gocyclo
func (v *Validation) Reconcile(ctx context.Context, nodeClass *v1alpha1.YandexNodeClass) (reconcile.Result, error) {
	if _, ok := lo.Find(v.requiredConditions(), func(cond string) bool {
		return nodeClass.StatusConditions().Get(cond).IsFalse()
	}); ok {
		// If any of the required status conditions are false, we know validation will fail regardless of the other values.
		nodeClass.StatusConditions().SetFalse(
			v1alpha1.ConditionTypeValidationSucceeded,
			ConditionReasonDependenciesNotReady,
			"Awaiting AMI, Instance Profile, Security Group, and Subnet resolution",
		)
		return reconcile.Result{RequeueAfter: requeueAfterTime}, nil
	}
	if _, ok := lo.Find(v.requiredConditions(), func(cond string) bool {
		return nodeClass.StatusConditions().Get(cond).IsUnknown()
	}); ok {
		// If none of the status conditions are false, but at least one is unknown, we should also consider the validation
		// state to be unknown. Once all required conditions collapse to a true or false state, we can test validation.
		nodeClass.StatusConditions().SetUnknownWithReason(
			v1alpha1.ConditionTypeValidationSucceeded,
			ConditionReasonDependenciesNotReady,
			"Awaiting AMI, Instance Profile, Security Group, and Subnet resolution",
		)
		return reconcile.Result{RequeueAfter: requeueAfterTime}, nil
	}

	if val, ok := v.cache.Get(v.cacheKey(nodeClass)); ok {
		// We still update the status condition even if it's cached since we may have had a conflict error previously
		if val == "" {
			nodeClass.StatusConditions().SetTrue(v1alpha1.ConditionTypeValidationSucceeded)
		} else {
			nodeClass.StatusConditions().SetFalse(
				v1alpha1.ConditionTypeValidationSucceeded,
				val.(string),
				"something went wrong",
			)
		}
		return reconcile.Result{RequeueAfter: requeueAfterTime}, nil
	}

	if v.dryRunDisabled {
		nodeClass.StatusConditions().SetTrue(v1alpha1.ConditionTypeValidationSucceeded)
		v.cache.SetDefault(v.cacheKey(nodeClass), "")
		return reconcile.Result{RequeueAfter: requeueAfterTime}, nil
	}

	v.cache.SetDefault(v.cacheKey(nodeClass), "")
	nodeClass.StatusConditions().SetTrue(v1alpha1.ConditionTypeValidationSucceeded)
	return reconcile.Result{RequeueAfter: requeueAfterTime}, nil
}

func (*Validation) requiredConditions() []string {
	return []string{
		v1alpha1.ConditionTypeSubnetsReady,
	}
}

func (*Validation) cacheKey(nodeClass *v1alpha1.YandexNodeClass) string {
	hash := lo.Must(hashstructure.Hash([]interface{}{
		nodeClass.Status.Subnets,
		nodeClass.Spec.Labels,
	}, hashstructure.FormatV2, &hashstructure.HashOptions{SlicesAsSets: true}))
	return fmt.Sprintf("%s:%016x", nodeClass.Name, hash)
}

// clearCacheEntries removes all cache entries associated with the given nodeclass from the validation cache
func (v *Validation) clearCacheEntries(nodeClass *v1alpha1.YandexNodeClass) {
	var toDelete []string
	for key := range v.cache.Items() {
		parts := strings.Split(key, ":")
		// NOTE: should never occur, indicates malformed cache key
		if len(parts) != 2 {
			continue
		}
		if parts[0] == nodeClass.Name {
			toDelete = append(toDelete, key)
		}
	}
	for _, key := range toDelete {
		v.cache.Delete(key)
	}
}
