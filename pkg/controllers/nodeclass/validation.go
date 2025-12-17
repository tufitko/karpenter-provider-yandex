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
	"github.com/tufitko/karpenter-provider-yandex/pkg/yandex"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	requeueAfterTime                          = 10 * time.Minute
	ConditionReasonDependenciesNotReady       = "DependenciesNotReady"
	MB                                  int64 = 1 << 20
	GB                                  int64 = 1 << 30
	TB                                  int64 = 1 << 40
	stepNetworkDiskBytes                      = 4 * MB
	maxDefaultBytes                           = 8 * TB // The block_size is not set in the provider. Default block_size=4KB, maximum disk size for block_size 4KB = 8TB.
	stepNonReplicated                         = 93 * GB
)

type Validation struct {
	kubeClient     client.Client
	cache          *cache.Cache
	sdk            yandex.SDK
	dryRunDisabled bool
}

type diskRules struct {
	minBytes  int64
	stepBytes int64
	maxBytes  int64
}

func NewValidationReconciler(
	kubeClient client.Client,
	cache *cache.Cache,
	sdk yandex.SDK,
	dryRunDisabled bool,
) *Validation {
	return &Validation{
		kubeClient:     kubeClient,
		cache:          cache,
		sdk:            sdk,
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

	if reason, msg := validateDisk(nodeClass.Spec); reason != "" {
		nodeClass.StatusConditions().SetFalse(
			v1alpha1.ConditionTypeValidationSucceeded,
			reason,
			msg,
		)
		v.cache.SetDefault(v.cacheKey(nodeClass), reason)
		return reconcile.Result{RequeueAfter: requeueAfterTime}, nil
	}

	if reason, msg := validateSubnetsExist(ctx, v.sdk, nodeClass); reason != "" {
		nodeClass.StatusConditions().SetFalse(
			v1alpha1.ConditionTypeValidationSucceeded,
			reason,
			msg,
		)
		v.cache.SetDefault(v.cacheKey(nodeClass), reason)
		return reconcile.Result{RequeueAfter: requeueAfterTime}, nil
	}

	if reason, msg := validateSecurityGroupsExist(ctx, v.sdk, nodeClass); reason != "" {
		nodeClass.StatusConditions().SetFalse(
			v1alpha1.ConditionTypeValidationSucceeded,
			reason,
			msg,
		)
		v.cache.SetDefault(v.cacheKey(nodeClass), reason)
		return reconcile.Result{RequeueAfter: requeueAfterTime}, nil
	}

	if reason, msg := validateSAN(nodeClass.Spec); reason != "" {
		nodeClass.StatusConditions().SetFalse(
			v1alpha1.ConditionTypeValidationSucceeded,
			reason,
			msg,
		)
		v.cache.SetDefault(v.cacheKey(nodeClass), reason)
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
		nodeClass.Spec.DiskType,
		nodeClass.Spec.DiskSize.String(),
		nodeClass.Spec.SecurityGroups,
		nodeClass.Spec.SoftwareAcceleratedNetworkSettings,
		nodeClass.Spec.CoreFractions,
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

func rulesForDiskType(t string) (diskRules, bool) {
	switch t {
	case "network-ssd", "network-hdd":
		return diskRules{
			minBytes:  stepNetworkDiskBytes,
			stepBytes: stepNetworkDiskBytes,
			maxBytes:  maxDefaultBytes,
		}, true
	case "network-ssd-nonreplicated", "network-ssd-io-m3":
		return diskRules{
			minBytes:  stepNonReplicated,
			stepBytes: stepNonReplicated,
			maxBytes:  256 * TB,
		}, true
	default:
		return diskRules{}, false
	}
}

// validateDisk checks whether nodeClass.Spec.DiskType and nodeClass.Spec.DiskSize comply with Yandex Cloud restrictions.
// Returns an empty reason if everything is correct.
func validateDisk(spec v1alpha1.YandexNodeClassSpec) (reason, msg string) {
	sizeBytes := spec.DiskSize.Value()
	if sizeBytes <= 0 {
		return "InvalidDiskSize", "spec.diskSize must be > 0"
	}

	diskType := spec.DiskType
	if diskType == "" {
		diskType = "network-ssd"
	}

	r, ok := rulesForDiskType(spec.DiskType)
	if !ok {
		return "InvalidDiskType", fmt.Sprintf("unsupported spec.diskType=%q", spec.DiskType)
	}

	if r.minBytes > 0 && sizeBytes < r.minBytes {
		return "InvalidDiskSize", fmt.Sprintf(
			"spec.diskSize must be >= %s for diskType=%s",
			resource.NewQuantity(r.minBytes, resource.BinarySI).String(),
			spec.DiskType,
		)
	}

	if r.stepBytes > 0 && (sizeBytes%r.stepBytes) != 0 {
		return "InvalidDiskSize", fmt.Sprintf(
			"spec.diskSize must be a multiple of %s for diskType=%s",
			resource.NewQuantity(r.stepBytes, resource.BinarySI).String(),
			spec.DiskType,
		)
	}

	if r.maxBytes > 0 && sizeBytes > r.maxBytes {
		if spec.DiskType == "" || spec.DiskType == "network-ssd" || spec.DiskType == "network-hdd" {
			return "InvalidDiskSize", fmt.Sprintf(
				"spec.diskSize must be <= %s for diskType=%s",
				resource.NewQuantity(r.maxBytes, resource.BinarySI).String(),
				lo.If(spec.DiskType == "", "network-ssd").Else(spec.DiskType),
			)
		}
		return "InvalidDiskSize", fmt.Sprintf(
			"spec.diskSize must be <= %s for diskType=%s",
			resource.NewQuantity(r.maxBytes, resource.BinarySI).String(),
			spec.DiskType,
		)
	}

	return "", ""
}

// validateSubnetsExist verifies that all resolved subnets in nodeClass.Status.Subnets
// still exist in Yandex Cloud, belong to the cluster network, and (if present) match the stored ZoneID.
func validateSubnetsExist(ctx context.Context, yc yandex.SDK, nodeClass *v1alpha1.YandexNodeClass) (reason, msg string) {
	if len(nodeClass.Status.Subnets) == 0 {
		return "NoSubnetsResolved", "no subnets resolved in status"
	}

	networkID, err := yc.NetworkID(ctx)
	if err != nil {
		return "SubnetLookupFailed", "failed to get cluster network id: " + err.Error()
	}

	seen := make(map[string]struct{}, len(nodeClass.Status.Subnets))
	for _, st := range nodeClass.Status.Subnets {
		if st.ID == "" {
			return "InvalidSubnet", "status.subnets contains empty id"
		}
		if _, dup := seen[st.ID]; dup {
			continue
		}
		seen[st.ID] = struct{}{}

		sn, err := yc.GetSubnet(ctx, st.ID)
		if err != nil {
			return "SubnetLookupFailed", "failed to get subnet " + st.ID + ": " + err.Error()
		}
		if sn == nil {
			return "SubnetNotFound", "subnet not found: " + st.ID
		}

		if sn.NetworkId != "" && sn.NetworkId != networkID {
			return "SubnetNotFound", "subnet not in cluster network: " + st.ID
		}

		if st.ZoneID != "" && sn.ZoneId != "" && st.ZoneID != sn.ZoneId {
			return "SubnetZoneMismatch", "subnet zone mismatch for " + st.ID + ": status=" + st.ZoneID + ", cloud=" + sn.ZoneId
		}
	}

	return "", ""
}

// validateSecurityGroupsExist verifies that every Security Group ID listed in nodeClass.Spec.SecurityGroups
// exists in Yandex Cloud and belongs to the cluster network (same VPC network as the cluster).
func validateSecurityGroupsExist(ctx context.Context, yc yandex.SDK, nodeClass *v1alpha1.YandexNodeClass) (reason, msg string) {
	for _, sgID := range nodeClass.Spec.SecurityGroups {
		ok, err := yc.SecurityGroupExists(ctx, sgID)
		if err != nil {
			return "SecurityGroupLookupFailed", "failed to get security group " + sgID + ": " + err.Error()
		}
		if !ok {
			return "SecurityGroupNotFound", "security group not found (or not in cluster network): " + sgID
		}
	}
	return "", ""
}

// validateSAN ensures that softwareAcceleratedNetworkSettings is only enabled when a 100% core fraction is possible.
func validateSAN(spec v1alpha1.YandexNodeClassSpec) (reason, msg string) {
	if !spec.SoftwareAcceleratedNetworkSettings {
		return "", ""
	}

	//If CoreFractions is not set, provider defaults to 100%
	if len(spec.CoreFractions) == 0 {
		return "", ""
	}

	for _, cf := range spec.CoreFractions {
		if string(cf) == "100" {
			return "", ""
		}
	}

	return "InvalidSANCoreFractions",
		"softwareAcceleratedNetworkSettings=true requires core_fractions to include 100 "
}
