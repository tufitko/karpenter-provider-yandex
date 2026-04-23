/*
Copyright 2025 The Kubernetes Authors.

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

package yandex

import (
	"context"
	_ "embed"
	"fmt"
	"maps"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/tufitko/karpenter-provider-yandex/pkg/apis"
	"github.com/tufitko/karpenter-provider-yandex/pkg/providers/instancetype"
	"github.com/tufitko/karpenter-provider-yandex/pkg/providers/subnet"
	"github.com/tufitko/karpenter-provider-yandex/pkg/yandex"
	"github.com/yandex-cloud/go-genproto/yandex/cloud/k8s/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/karpenter/pkg/events"

	"github.com/awslabs/operatorpkg/status"
	"github.com/go-logr/logr"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/tufitko/karpenter-provider-yandex/pkg/apis/v1alpha1"
	cloudproviderevents "github.com/tufitko/karpenter-provider-yandex/pkg/cloudprovider/events"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
	"sigs.k8s.io/karpenter/pkg/utils/resources"
)

const (
	CloudProviderName    = "yandex"
	YandexProviderPrefix = "yandex://"
)

var _ cloudprovider.CloudProvider = (*CloudProvider)(nil)

type CloudProvider struct {
	kubeClient client.Client
	recorder   events.Recorder
	log        logr.Logger

	instanceTypes instancetype.Provider
	subnets       subnet.Provider

	sdk yandex.SDK
}

func NewCloudProvider(ctx context.Context,
	kubeClient client.Client,
	sdk yandex.SDK,
	recorder events.Recorder,
	instanceTypes instancetype.Provider,
	subnets subnet.Provider,
) (*CloudProvider, error) {
	log := log.FromContext(ctx).WithName(CloudProviderName)
	log.WithName("NewCloudProvider()")
	provider := &CloudProvider{
		kubeClient:    kubeClient,
		sdk:           sdk,
		log:           log,
		recorder:      recorder,
		instanceTypes: instanceTypes,
		subnets:       subnets,
	}
	return provider, nil
}

// Create launches a NodeClaim with the given resource requests and requirements and returns a hydrated
// NodeClaim back with resolved NodeClaim labels for the launched NodeClaim
func (c CloudProvider) Create(ctx context.Context, nodeClaim *karpv1.NodeClaim) (*karpv1.NodeClaim, error) {
	log := c.log.WithName("Create()")
	log.Info("Executed with params", "nodePool", nodeClaim.Name, "spec", nodeClaim.Spec)

	nodeClass, err := c.resolveNodeClassFromNodeClaim(ctx, nodeClaim)
	if err != nil {
		if errors.IsNotFound(err) {
			// We treat a failure to resolve the NodeClass as an ICE since this means there is no capacity possibilities for this NodeClaim
			c.recorder.Publish(cloudproviderevents.NodeClaimFailedToResolveNodeClass(nodeClaim))
			return nil, cloudprovider.NewInsufficientCapacityError(fmt.Errorf("resolving nodeclass, %w", err))
		}
		// Transient error when resolving the NodeClass
		return nil, fmt.Errorf("resolving nodeclass, %w", err)
	}

	nodeClassReady := nodeClass.StatusConditions().Get(status.ConditionReady)
	if nodeClassReady.IsFalse() {
		return nil, cloudprovider.NewNodeClassNotReadyError(fmt.Errorf("%s", nodeClassReady.Message))
	}
	if nodeClassReady.IsUnknown() {
		return nil, cloudprovider.NewCreateError(fmt.Errorf("resolving nodeclass readiness, nodeclass is in Ready=Unknown, %s", nodeClassReady.Message), "NodeClassReadinessUnknown", "NodeClass is in Ready=Unknown")
	}
	if nodeClassReady != nil && nodeClassReady.ObservedGeneration != nodeClass.Generation {
		return nil, cloudprovider.NewNodeClassNotReadyError(fmt.Errorf("nodeclass status has not been reconciled against the latest spec"))
	}

	instanceTypes, err := c.resolveInstanceTypes(ctx, nodeClaim, nodeClass)
	if err != nil {
		return nil, cloudprovider.NewCreateError(fmt.Errorf("resolving instance types, %w", err), "InstanceTypeResolutionFailed", "Error resolving instance types")
	}

	if len(instanceTypes) == 0 {
		return nil, cloudprovider.NewInsufficientCapacityError(fmt.Errorf("all requested instance types were unavailable during launch"))
	}

	log.Info("Successfully resolved instance types", "count", len(instanceTypes))

	reqs := scheduling.NewNodeSelectorRequirementsWithMinValues(nodeClaim.Spec.Requirements...)
	subnets, err := c.subnets.List(ctx, nodeClass)
	if err != nil {
		return nil, fmt.Errorf("listing subnets, %w", err)
	}
	zoneToSubnet := lo.SliceToMap(subnets, func(s subnet.Subnet) (string, subnet.Subnet) {
		return s.ZoneID, s
	})

	instanceTypes = lo.Filter(instanceTypes, func(it *cloudprovider.InstanceType, _ int) bool {
		offerings := lo.Filter(it.Offerings, func(off *cloudprovider.Offering, _ int) bool {
			off.Requirements.Add(it.Requirements.Values()...)
			off.Requirements.Add(
				scheduling.NewRequirement(karpv1.NodePoolLabelKey, corev1.NodeSelectorOpIn, nodeClaim.Labels[karpv1.NodePoolLabelKey]),
				scheduling.NewRequirement("karpenter.yandex.cloud/yandexnodeclass", corev1.NodeSelectorOpIn, nodeClaim.Labels["karpenter.yandex.cloud/yandexnodeclass"]),
			)
			return off.Requirements.IsCompatible(reqs)
		})

		it.Offerings = offerings
		return len(offerings) > 0
	})

	it := instanceTypes[0]

	availableOfferings := it.Offerings.Available()

	spotOfferings := lo.Filter(availableOfferings, func(off *cloudprovider.Offering, _ int) bool {
		return off.CapacityType() == karpv1.CapacityTypeSpot
	})

	// This is very bad, but at the moment there is no normal way to check the availability of a zone to raise a node,
	// so in order to avoid constantly raising nodes in an inaccessible zone,
	// we will choose offering with a random zone.
	var offering *cloudprovider.Offering

	if len(spotOfferings) > 0 {
		offering = spotOfferings[rand.Intn(len(spotOfferings))]
	} else {
		offering = availableOfferings[rand.Intn(len(availableOfferings))]
	}

	var yait yandex.InstanceType
	if err = yait.FromString(it.Name); err != nil {
		return nil, fmt.Errorf("parse instance type name: %w", err)
	}

	labels := maps.Clone(nodeClass.Spec.Labels)
	labels[karpv1.NodePoolLabelKey] = nodeClaim.Labels[karpv1.NodePoolLabelKey]
	labels["karpenter.yandex.cloud/yandexnodeclass"] = nodeClaim.Labels["karpenter.yandex.cloud/yandexnodeclass"]

	nodeLabels := maps.Clone(nodeClass.Spec.NodeLabels)
	nodeLabels[karpv1.NodePoolLabelKey] = nodeClaim.Labels[karpv1.NodePoolLabelKey]
	labels["karpenter.yandex.cloud/yandexnodeclass"] = nodeClaim.Labels["karpenter.yandex.cloud/yandexnodeclass"]
	nodeLabels[v1alpha1.LabelInstanceCPUPlatform] = string(yait.Platform)
	nodeLabels[v1alpha1.LabelInstanceCPU] = yait.CPU.String()
	nodeLabels[v1alpha1.LabelInstanceMemory] = yait.Memory.String()
	nodeLabels[v1alpha1.LabelInstanceCPUFraction] = fmt.Sprintf("%d", yait.CoreFraction)
	labels[karpv1.CapacityTypeLabelKey] = offering.CapacityType()
	nodeLabels[karpv1.CapacityTypeLabelKey] = offering.CapacityType()

	diskType := nodeClass.Spec.DiskType
	diskSize := nodeClass.Spec.DiskSize.Value()

	nodeGroupId, err := c.sdk.CreateFixedNodeGroup(
		ctx,
		nodeClaim.Name,
		labels,
		nodeLabels,
		yait.Platform,
		yait.CoreFraction,
		yait.CPU,
		yait.Memory,
		offering.CapacityType() == karpv1.CapacityTypeSpot,
		offering.Zone(),
		zoneToSubnet[offering.Zone()].ID,
		nodeClass,
		diskType,
		diskSize,
	)
	if err != nil {
		return nil, fmt.Errorf("creating instance, %w", err)
	}

	log.Info("Successfully created instance", "providerID", nodeGroupId)

	ng, err := c.sdk.GetNodeGroup(ctx, nodeGroupId)
	if err != nil {
		return nil, fmt.Errorf("getting node group, %w", err)
	}

	// Bound the ProviderID wait inside a single Create reconcile. When the VM isn't ready
	// yet we don't want to block karpenter-core's launch controller for the full
	// waitForProviderIDTTL — return a CreateError so karpenter-core sets
	// ConditionTypeLaunched=Unknown and re-runs Launch via its own rate-limited retry
	// (CreateFixedNodeGroup is idempotent: the name-guard in pkg/yandex/sdk.go and the
	// 10 min CachedSDK entry make retries cheap and safe).
	createCtx, cancel := context.WithTimeout(ctx, createProviderIDTimeout)
	defer cancel()
	nc, err := c.nodeGroupToNodeClaim(createCtx, ng, it)
	if err != nil && (ng.Status == k8s.NodeGroup_PROVISIONING || ng.Status == k8s.NodeGroup_STARTING) {
		return nil, cloudprovider.NewCreateError(
			fmt.Errorf("nodegroup %s provider id not ready yet: %w", ng.Id, err),
			"ProviderIDNotReady",
			"NodeGroup is still provisioning, will retry",
		)
	}
	return nc, err
}

// Delete removes a NodeClaim from the cloudprovider by its provider id. Delete should return
// NodeClaimNotFoundError if the cloudProvider instance is already terminated and nil if deletion was triggered.
// Karpenter will keep retrying until Delete returns a NodeClaimNotFound error.
func (c CloudProvider) Delete(ctx context.Context, nodeClaim *karpv1.NodeClaim) error {
	log := c.log.WithName("Delete()")
	log.Info("Executed with params", "nodeClaim", nodeClaim.Name)

	nodeGroupId := nodeClaim.Labels["yandex.cloud/node-group-id"]
	if nodeGroupId == "" {
		log.Info("nodeGroupId is empty")
		return cloudprovider.NewNodeClaimNotFoundError(fmt.Errorf("nodeGroupId is empty for nodeclaim %s", nodeClaim.Name))
	}

	err := c.sdk.DeleteNodeGroup(ctx, nodeGroupId)
	if err != nil {
		// NodeGroup already deleted by another NodeClaim / GC path: classify by gRPC
		// status so a localized or differently-worded error message doesn't slip
		// through as a generic error and leak a stuck NodeClaim upstream.
		if isNotFoundError(err) {
			log.Info("NodeGroup already deleted", "nodeGroupId", nodeGroupId)
			// Return NodeClaimNotFoundError to signal that the instance is already terminated
			return cloudprovider.NewNodeClaimNotFoundError(fmt.Errorf("nodegroup %s not found", nodeGroupId))
		}
		log.Error(err, "Failed to delete nodegroup", "nodeGroupId", nodeGroupId)
		// Return other errors as-is for retry
		return err
	}

	log.Info("Successfully deleted NodeGroup", "nodeGroupId", nodeGroupId)
	return nil
}

// Get retrieves a NodeClaim from the cloudprovider by its provider id
func (c CloudProvider) Get(ctx context.Context, providerID string) (*karpv1.NodeClaim, error) {
	log := c.log.WithName("Get()")
	log.Info("Executed with params", "providerID", providerID)

	if providerID == "" {
		return nil, fmt.Errorf("providerID is empty")
	}

	if !strings.HasPrefix(providerID, YandexProviderPrefix) {
		return nil, fmt.Errorf("providerID does not have the correct prefix")
	}

	ng, err := c.sdk.GetNodeGroupByProviderId(ctx, providerID)
	if err != nil {
		// Classify NotFound via gRPC status (instance/NG deleted out from under us) so
		// we return a proper NodeClaimNotFoundError regardless of how the server
		// phrased the message.
		if isNotFoundError(err) {
			log.Info("NodeGroup/Instance not found", "providerID", providerID)
			// Return NodeClaimNotFoundError to signal that the instance is already terminated
			return nil, cloudprovider.NewNodeClaimNotFoundError(fmt.Errorf("instance %s not found", providerID))
		}
		// Return other errors as-is for retry
		return nil, fmt.Errorf("getting node group, %w", err)
	}

	nodeClass, err := c.resolveNodeClassFromNodeGroup(ctx, ng)
	if err != nil {
		return nil, fmt.Errorf("getting node class, %w", err)
	}

	it, err := c.nodeGroupToInstanceType(ctx, ng, nodeClass)
	if err != nil {
		return nil, fmt.Errorf("getting instance type, %w", err)
	}

	return c.nodeGroupToNodeClaim(ctx, ng, it)
}

// List retrieves all NodeClaims from the cloudprovider
func (c CloudProvider) List(ctx context.Context) ([]*karpv1.NodeClaim, error) {
	log := c.log.WithName("List()")

	// todo: do it better and faster

	ngs, err := c.sdk.ListNodeGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing nodes, %w", err)
	}

	var nodeClaims []*karpv1.NodeClaim
	for _, ng := range ngs {
		var nodeClass *v1alpha1.YandexNodeClass
		nodeClass, err = c.resolveNodeClassFromNodeGroup(ctx, ng)
		if err != nil {
			log.Error(err, "failed to resolve yandex node class", "nodeGroup", ng.GetName())
			continue
		}

		var it *cloudprovider.InstanceType
		it, err = c.nodeGroupToInstanceType(ctx, ng, nodeClass)
		if err != nil {
			log.Error(err, "failed to resolve instance type", "nodeGroup", ng.GetName(), "nodeClass", nodeClass.Name)
			continue
		}

		var nc *karpv1.NodeClaim
		nc, err = c.nodeGroupToNodeClaim(ctx, ng, it)
		if err != nil {
			log.Error(err, "failed to find node group", "nodeGroup", ng.Name)
			continue
		}

		nodeClaims = append(nodeClaims, nc)
	}

	log.V(1).Info("Successfully retrieved node claims list", "count", len(nodeClaims))
	return nodeClaims, nil
}

// GetInstanceTypes returns instance types supported by the cloudprovider.
// Availability of types or zone may vary by nodepool or over time.  Regardless of
// availability, the GetInstanceTypes method should always return all instance types,
// even those with no offerings available.
func (c CloudProvider) GetInstanceTypes(ctx context.Context, nodePool *karpv1.NodePool) ([]*cloudprovider.InstanceType, error) {
	nodeClass, err := c.resolveNodeClassFromNodePool(ctx, nodePool)
	if err != nil {
		if errors.IsNotFound(err) {
			c.recorder.Publish(cloudproviderevents.NodePoolFailedToResolveNodeClass(nodePool))
			return nil, nil
		}
		return nil, fmt.Errorf("resolving nodeClass, %w", err)
	}

	instanceTypes, err := c.instanceTypes.List(ctx, nodeClass)
	if err != nil {
		return nil, fmt.Errorf("listing instance types, %w", err)
	}
	return instanceTypes, nil
}

// IsDrifted returns whether a NodeClaim has drifted from the provisioning requirements
// it is tied to.
func (c CloudProvider) IsDrifted(_ context.Context, _ *karpv1.NodeClaim) (cloudprovider.DriftReason, error) {
	return "", nil
}

// RepairPolicy is for CloudProviders to define a set Unhealthy condition for Karpenter
// to monitor on the node.
func (c CloudProvider) RepairPolicies() []cloudprovider.RepairPolicy {
	return []cloudprovider.RepairPolicy{}
}

// Name returns the CloudProvider implementation name.
func (c CloudProvider) Name() string {
	return CloudProviderName
}

func (c CloudProvider) GetSupportedNodeClasses() []status.Object {
	return []status.Object{&v1alpha1.YandexNodeClass{}}
}

func (c CloudProvider) resolveNodeClassFromNodeClaim(ctx context.Context, nodeClaim *karpv1.NodeClaim) (*v1alpha1.YandexNodeClass, error) {
	nodeClass := &v1alpha1.YandexNodeClass{}
	if err := c.kubeClient.Get(ctx, types.NamespacedName{Name: nodeClaim.Spec.NodeClassRef.Name}, nodeClass); err != nil {
		return nil, err
	}
	// For the purposes of NodeClass CloudProvider resolution, we treat deleting NodeClasses as NotFound
	if !nodeClass.DeletionTimestamp.IsZero() {
		// For the purposes of NodeClass CloudProvider resolution, we treat deleting NodeClasses as NotFound,
		// but we return a different error message to be clearer to users
		return nil, newTerminatingNodeClassError(nodeClass.Name)
	}
	return nodeClass, nil
}

func (c CloudProvider) resolveInstanceTypes(ctx context.Context, nodeClaim *karpv1.NodeClaim, class *v1alpha1.YandexNodeClass) ([]*cloudprovider.InstanceType, error) {
	types, err := c.instanceTypes.List(ctx, class)
	if err != nil {
		return nil, err
	}

	reqs := scheduling.NewNodeSelectorRequirementsWithMinValues(nodeClaim.Spec.Requirements...)

	types = lo.Filter(types, func(i *cloudprovider.InstanceType, _ int) bool {
		return len(i.Offerings.Compatible(reqs).Available()) > 0 &&
			resources.Fits(nodeClaim.Spec.Resources.Requests, i.Allocatable())
	})

	sort.Slice(types, func(i, j int) bool {
		return types[i].Offerings.Compatible(reqs).Available().Cheapest().Price < types[j].Offerings.Compatible(reqs).Available().Cheapest().Price
	})

	return types, nil
}

const (
	// waitForProviderIDTTL bounds the ProviderID wait on the Get/List paths, where the NodeClaim
	// is reported to be in PROVISIONING/STARTING and we poll until the VM registers an instance.
	waitForProviderIDTTL = 5 * time.Minute
	// createProviderIDTimeout bounds the ProviderID wait inside a single Create reconcile. If the
	// VM isn't up within this window we return CreateError (ProviderIDNotReady) and let
	// karpenter-core retry instead of holding the reconcile open for waitForProviderIDTTL.
	createProviderIDTimeout = 30 * time.Second
)

// isNotFoundError reports whether err is (or wraps) a gRPC NotFound status. We fall back
// to a substring check for non-gRPC errors (e.g. errors wrapped with fmt.Errorf by our
// own SDK layer where the underlying grpc.Status was dropped).
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if grpcstatus.Code(err) == codes.NotFound {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "not found") || strings.Contains(msg, "NotFound")
}

func (c CloudProvider) nodeGroupToNodeClaim(ctx context.Context, ng *k8s.NodeGroup, instanceType *cloudprovider.InstanceType) (*karpv1.NodeClaim, error) {
	nodeClaim := &karpv1.NodeClaim{}
	labels := map[string]string{}
	annotations := map[string]string{}

	if instanceType != nil {
		for key, req := range instanceType.Requirements {
			if req.Len() == 1 {
				labels[key] = req.Values()[0]
			}
		}
		resourceFilter := func(n corev1.ResourceName, v resource.Quantity) bool {
			if resources.IsZero(v) {
				return false
			}
			return true
		}
		nodeClaim.Status.Capacity = lo.PickBy(instanceType.Capacity, resourceFilter)

		// Safely call Allocatable() only if Offerings is not nil
		if instanceType.Offerings != nil {
			nodeClaim.Status.Allocatable = lo.PickBy(instanceType.Allocatable(), resourceFilter)
		} else {
			// If Offerings is nil, use Capacity as fallback (with overhead subtraction if possible)
			nodeClaim.Status.Allocatable = lo.PickBy(instanceType.Capacity, resourceFilter)
			log.FromContext(ctx).Info("InstanceType has nil Offerings, using Capacity as Allocatable", "instanceType", instanceType.Name)
		}
	}

	nodeClaim.Labels = lo.Assign(labels, c.nodeGroupLabels(ng))
	nodeClaim.Annotations = annotations
	nodeClaim.CreationTimestamp = metav1.Time{Time: ng.GetCreatedAt().AsTime()}

	if lo.Contains(
		[]k8s.NodeGroup_Status{
			k8s.NodeGroup_STOPPING,
			k8s.NodeGroup_STOPPED,
			k8s.NodeGroup_DELETING,
		},
		ng.Status,
	) {
		nodeClaim.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	}

	providerID, lastErr := c.sdk.ProviderIdFor(ctx, ng.Id)
	if lastErr != nil && (ng.Status == k8s.NodeGroup_PROVISIONING || ng.Status == k8s.NodeGroup_STARTING) {
		// Poll for the ProviderID while the NodeGroup is still coming up. The loop exits
		// immediately if ctx is cancelled — otherwise a cancelled reconcile context would
		// keep calling the Yandex Cloud API and surface
		//   "rpc error: code = Canceled desc = cancelled received from server"
		// for the full waitForProviderIDTTL (incident 2026-04-23).
		pollErr := wait.PollUntilContextTimeout(ctx, time.Second, waitForProviderIDTTL, true, func(ctx context.Context) (bool, error) {
			id, err := c.sdk.ProviderIdFor(ctx, ng.Id)
			if err != nil {
				lastErr = err
				return false, nil
			}
			providerID = id
			lastErr = nil
			return true, nil
		})
		if pollErr != nil && lastErr == nil {
			lastErr = pollErr
		}
	}
	if lastErr != nil || providerID == "" {
		return nil, fmt.Errorf("failed to determine provider id: %w", lastErr)
	}
	nodeClaim.Status.ProviderID = providerID

	return nodeClaim, nil
}

func (c CloudProvider) nodeGroupToYandexInstanceType(ng *k8s.NodeGroup) yandex.InstanceType {
	var yait yandex.InstanceType
	yait.Platform = yandex.PlatformId(ng.GetNodeTemplate().GetPlatformId())
	yait.CoreFraction = yandex.CoreFraction(ng.GetNodeTemplate().GetResourcesSpec().GetCoreFraction())
	yait.CPU = *resource.NewQuantity(ng.GetNodeTemplate().GetResourcesSpec().GetCores(), resource.DecimalSI)
	yait.Memory = *resource.NewQuantity(ng.GetNodeTemplate().GetResourcesSpec().GetMemory(), resource.BinarySI)
	return yait
}

func (c CloudProvider) nodeGroupLabels(ng *k8s.NodeGroup) map[string]string {
	labels := make(map[string]string)
	labels = lo.Assign(labels, ng.GetNodeLabels())

	var zoneID string
	if len(ng.GetAllocationPolicy().GetLocations()) > 0 {
		zoneID = ng.GetAllocationPolicy().GetLocations()[0].GetZoneId()
	}

	// yandex-provided labels
	labels["beta.kubernetes.io/arch"] = "amd64"
	labels[corev1.LabelArchStable] = "amd64"
	labels[corev1.LabelInstanceType] = ng.GetNodeTemplate().GetPlatformId()
	labels[corev1.LabelInstanceTypeStable] = ng.GetNodeTemplate().GetPlatformId()
	labels["beta.kubernetes.io/os"] = "linux"
	labels[corev1.LabelOSStable] = "linux"
	labels[corev1.LabelZoneFailureDomain] = zoneID
	labels[corev1.LabelTopologyZone] = zoneID
	labels[corev1.LabelHostname] = ng.Name + "-1"
	labels["yandex.cloud/node-group-id"] = ng.GetId()
	labels["yandex.cloud/pci-topology"] = "k8s"
	labels["yandex.cloud/preemptible"] = fmt.Sprintf("%t", ng.GetNodeTemplate().GetSchedulingPolicy().GetPreemptible())

	if ng.GetNodeTemplate().GetSchedulingPolicy().GetPreemptible() {
		labels[karpv1.CapacityTypeLabelKey] = karpv1.CapacityTypeSpot
	} else {
		labels[karpv1.CapacityTypeLabelKey] = karpv1.CapacityTypeOnDemand
	}

	return labels
}

func (c CloudProvider) nodeGroupToInstanceType(ctx context.Context, ng *k8s.NodeGroup, nodeClass *v1alpha1.YandexNodeClass) (*cloudprovider.InstanceType, error) {
	yait := c.nodeGroupToYandexInstanceType(ng)
	return c.instanceTypes.GetInstanceType(ctx, nodeClass, yait.String())
}

func (c CloudProvider) resolveNodePoolFromNodeGroup(ctx context.Context, ng *k8s.NodeGroup) (*karpv1.NodePool, error) {
	if nodePoolName, ok := ng.Labels[karpv1.NodePoolLabelKey]; ok {
		nodePool := &karpv1.NodePool{}
		if err := c.kubeClient.Get(ctx, types.NamespacedName{Name: nodePoolName}, nodePool); err != nil {
			return nil, err
		}
		return nodePool, nil
	}
	return nil, errors.NewNotFound(schema.GroupResource{Group: apis.Group, Resource: "nodepools"}, "")
}

func (c CloudProvider) resolveNodeClassFromNodePool(ctx context.Context, nodePool *karpv1.NodePool) (*v1alpha1.YandexNodeClass, error) {
	nodeClass := &v1alpha1.YandexNodeClass{}
	if err := c.kubeClient.Get(ctx, types.NamespacedName{Name: nodePool.Spec.Template.Spec.NodeClassRef.Name}, nodeClass); err != nil {
		return nil, err
	}
	if !nodeClass.DeletionTimestamp.IsZero() {
		// For the purposes of NodeClass CloudProvider resolution, we treat deleting NodeClasses as NotFound,
		// but we return a different error message to be clearer to users
		return nil, newTerminatingNodeClassError(nodeClass.Name)
	}
	return nodeClass, nil
}

func (c CloudProvider) resolveNodeClassFromNodeGroup(ctx context.Context, ng *k8s.NodeGroup) (*v1alpha1.YandexNodeClass, error) {
	np, err := c.resolveNodePoolFromNodeGroup(ctx, ng)
	if err != nil {
		return nil, err
	}
	return c.resolveNodeClassFromNodePool(ctx, np)
}

// newTerminatingNodeClassError returns a NotFound error for handling by
func newTerminatingNodeClassError(name string) *errors.StatusError {
	qualifiedResource := schema.GroupResource{Group: apis.Group, Resource: "ec2nodeclasses"}
	err := errors.NewNotFound(qualifiedResource, name)
	err.ErrStatus.Message = fmt.Sprintf("%s %q is terminating, treating as not found", qualifiedResource.String(), name)
	return err
}
