package yandex

import (
	"context"
	"fmt"
	"maps"
	"math"
	"strings"

	"github.com/samber/lo"
	"github.com/tufitko/karpenter-provider-yandex/pkg/apis/v1alpha1"
	"github.com/yandex-cloud/go-genproto/yandex/cloud/compute/v1"
	"github.com/yandex-cloud/go-genproto/yandex/cloud/k8s/v1"
	"github.com/yandex-cloud/go-genproto/yandex/cloud/operation"
	"github.com/yandex-cloud/go-genproto/yandex/cloud/vpc/v1"
	ycsdk "github.com/yandex-cloud/go-sdk"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/api/resource"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

type SDK interface {
	NetworkID(ctx context.Context) (string, error)
	ListNetworkSubnets(ctx context.Context) ([]*vpc.Subnet, error)
	UsedIPsInSubnet(ctx context.Context, subnetId string) (int, error)
	MaxPodsPerNode(ctx context.Context) (int, error)
	CreateFixedNodeGroup(
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
	) (string, error)
	DeleteNodeGroup(ctx context.Context, nodeGroupId string) error
	GetNodeGroup(ctx context.Context, nodeGroupId string) (*k8s.NodeGroup, error)
	ProviderIdFor(ctx context.Context, nodeGroupId string) (string, error)
	GetNodeGroupByProviderId(ctx context.Context, providerId string) (*k8s.NodeGroup, error)
	ListNodeGroups(ctx context.Context) ([]*k8s.NodeGroup, error)
	GetNodeFromNodeGroup(ctx context.Context, nodeGroupId string) (*k8s.Node, error)
	SecurityGroupExists(ctx context.Context, securityGroupId string) (bool, error)
}

type YCSDK struct {
	*ycsdk.SDK
	clusterID string
}

func NewSDK(ctx context.Context, clusterID string) (*YCSDK, error) {
	sdk, err := buildSDK(ctx)
	if err != nil {
		return nil, err
	}

	return &YCSDK{
		SDK:       sdk,
		clusterID: clusterID,
	}, nil
}

func (p *YCSDK) ClusterID() string {
	return p.clusterID
}

func (p *YCSDK) NetworkID(ctx context.Context) (string, error) {
	cluster, err := p.SDK.Kubernetes().Cluster().Get(ctx, &k8s.GetClusterRequest{
		ClusterId: p.clusterID,
	})
	if err != nil {
		return "", err
	}
	return cluster.NetworkId, nil
}

func (p *YCSDK) ListNetworkSubnets(ctx context.Context) ([]*vpc.Subnet, error) {
	networkId, err := p.NetworkID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get network id: %w", err)
	}
	return p.SDK.VPC().Network().NetworkSubnetsIterator(ctx, &vpc.ListNetworkSubnetsRequest{
		NetworkId: networkId,
	}).TakeAll()
}

func (p *YCSDK) UsedIPsInSubnet(ctx context.Context, subnetId string) (int, error) {
	var res int
	iter := p.SDK.VPC().Subnet().SubnetUsedAddressesIterator(ctx, &vpc.ListUsedAddressesRequest{
		SubnetId: subnetId,
	})
	for iter.Next() {
		addresses, err := iter.Take(100)
		if err != nil {
			return 0, fmt.Errorf("failed to get subnet used addresses: %w", err)
		}
		res += len(addresses)
	}

	return res, nil
}

func (p *YCSDK) MaxPodsPerNode(ctx context.Context) (int, error) {
	cluster, err := p.SDK.Kubernetes().Cluster().Get(ctx, &k8s.GetClusterRequest{
		ClusterId: p.clusterID,
	})
	if err != nil {
		return 0, err
	}

	subnetMask := float64(24)
	if cluster.IpAllocationPolicy != nil && cluster.IpAllocationPolicy.NodeIpv4CidrMaskSize > 0 {
		subnetMask = float64(cluster.IpAllocationPolicy.NodeIpv4CidrMaskSize)
	}

	return int(math.Pow(2, 31-subnetMask)), nil
}

func (p *YCSDK) CreateFixedNodeGroup(
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
	// guard against duplicated node groups
	// this can be removed after stabilization of api and karpenter
	existedNodeGroups, err := p.ListNodeGroups(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list node groups: %w", err)
	}
	for _, existedNodeGroup := range existedNodeGroups {
		if existedNodeGroup.Name == name {
			return existedNodeGroup.Id, nil
		}
	}

	labels = maps.Clone(labels)
	labels["managed-by"] = "karpenter"
	for k, v := range nodeLabels {
		labels[k] = strings.ToLower(v)
	}

	op, err := p.SDK.WrapOperation(p.SDK.Kubernetes().NodeGroup().Create(ctx, &k8s.CreateNodeGroupRequest{
		ClusterId:   p.clusterID,
		Name:        name,
		Description: "karpenter node group",
		Labels:      labels,
		NodeTemplate: &k8s.NodeTemplate{
			Name:       name + "-" + zoneId + "-{instance.index}",
			Labels:     labels,
			PlatformId: string(platformId),
			ResourcesSpec: &k8s.ResourcesSpec{
				CoreFraction: int64(coreFraction),
				Cores:        cpu.Value(),
				Memory:       mem.Value(),
				// todo: gpu
			},
			BootDiskSpec: &k8s.DiskSpec{
				DiskTypeId: diskType,
				DiskSize:   diskSize,
			},
			Metadata: map[string]string{ // todo: configurable
				"enable-oslogin": "true",
			},
			SchedulingPolicy: &k8s.SchedulingPolicy{
				Preemptible: preemptible,
			},
			NetworkInterfaceSpecs: []*k8s.NetworkInterfaceSpec{
				{
					SubnetIds:            []string{subnetId},
					PrimaryV4AddressSpec: &k8s.NodeAddressSpec{},
					SecurityGroupIds:     nodeclass.Spec.SecurityGroups,
				},
			},
			NetworkSettings: &k8s.NodeTemplate_NetworkSettings{
				Type: lo.If(nodeclass.Spec.SoftwareAcceleratedNetworkSettings && coreFraction == CoreFraction100,
					k8s.NodeTemplate_NetworkSettings_SOFTWARE_ACCELERATED,
				).Else(k8s.NodeTemplate_NetworkSettings_STANDARD),
			},
			ContainerRuntimeSettings: &k8s.NodeTemplate_ContainerRuntimeSettings{
				Type: k8s.NodeTemplate_ContainerRuntimeSettings_CONTAINERD,
			},
		},
		ScalePolicy: &k8s.ScalePolicy{
			ScaleType: &k8s.ScalePolicy_FixedScale_{
				FixedScale: &k8s.ScalePolicy_FixedScale{
					Size: 1,
				},
			},
		},
		AllocationPolicy: &k8s.NodeGroupAllocationPolicy{
			Locations: []*k8s.NodeGroupLocation{
				{
					ZoneId: zoneId,
				},
			},
		},
		DeployPolicy: &k8s.DeployPolicy{
			MaxUnavailable: 0,
			MaxExpansion:   1,
		},
		MaintenancePolicy: &k8s.NodeGroupMaintenancePolicy{
			AutoRepair:  true,
			AutoUpgrade: false,
		},
		AllowedUnsafeSysctls: nil,
		NodeTaints: []*k8s.Taint{{
			Key:    karpv1.UnregisteredNoExecuteTaint.Key,
			Value:  karpv1.UnregisteredNoExecuteTaint.Value,
			Effect: k8s.Taint_NO_EXECUTE,
		}},
		NodeLabels: nodeLabels,
	}))
	if err != nil {
		return "", err
	}

	protoMetadata, err := op.Metadata()
	if err != nil {
		return "", fmt.Errorf("error while get Kubernetes node group create operation metadata: %s", err)
	}

	md, ok := protoMetadata.(*k8s.CreateNodeGroupMetadata)
	if !ok {
		return "", fmt.Errorf("could not get Instance ID from create operation metadata")
	}

	return md.GetNodeGroupId(), nil
}

func (p *YCSDK) DeleteNodeGroup(ctx context.Context, nodeGroupId string) error {
	operations, err := p.SDK.Kubernetes().NodeGroup().NodeGroupOperationsIterator(ctx, &k8s.ListNodeGroupOperationsRequest{
		NodeGroupId: nodeGroupId,
	}).TakeAll()
	if err != nil {
		return fmt.Errorf("failed to list node group operations: %w", err)
	}

	operations = lo.Filter(operations, func(item *operation.Operation, _ int) bool {
		typeURL := item.GetMetadata().GetTypeUrl()
		return strings.Contains(typeURL, "DeleteNodeGroup")
	})

	if len(operations) > 0 {
		// deleting in progress
		return nil
	}

	_, err = p.SDK.Kubernetes().NodeGroup().Delete(ctx, &k8s.DeleteNodeGroupRequest{
		NodeGroupId: nodeGroupId,
	})
	return err
}

func (p *YCSDK) GetNodeGroup(ctx context.Context, nodeGroupId string) (*k8s.NodeGroup, error) {
	return p.SDK.Kubernetes().NodeGroup().Get(ctx, &k8s.GetNodeGroupRequest{NodeGroupId: nodeGroupId})
}

func (p *YCSDK) ProviderIdFor(ctx context.Context, nodeGroupId string) (string, error) {
	resp, err := p.SDK.Kubernetes().NodeGroup().ListNodes(ctx, &k8s.ListNodeGroupNodesRequest{
		NodeGroupId: nodeGroupId,
	})
	if err != nil {
		return "", err
	}

	if len(resp.Nodes) == 0 || resp.Nodes[0].GetCloudStatus().GetId() == "" {
		return "", fmt.Errorf("not found")
	}

	return fmt.Sprintf("yandex://%s", resp.Nodes[0].GetCloudStatus().GetId()), nil
}

func (p *YCSDK) GetNodeGroupByProviderId(ctx context.Context, providerId string) (*k8s.NodeGroup, error) {
	instance, err := p.SDK.Compute().Instance().Get(ctx, &compute.GetInstanceRequest{
		InstanceId: strings.TrimPrefix(providerId, "yandex://"),
		View:       compute.InstanceView_BASIC,
	})
	if err != nil {
		return nil, err
	}
	nodeGroupId := instance.Labels["managed-kubernetes-node-group-id"]
	if nodeGroupId == "" {
		return nil, fmt.Errorf("could not get node group id")
	}

	return p.GetNodeGroup(ctx, nodeGroupId)
}

func (p *YCSDK) ListNodeGroups(ctx context.Context) ([]*k8s.NodeGroup, error) {
	cluster, err := p.SDK.Kubernetes().Cluster().Get(ctx, &k8s.GetClusterRequest{
		ClusterId: p.clusterID,
	})
	if err != nil {
		return nil, err
	}

	ngs, err := p.SDK.Kubernetes().NodeGroup().NodeGroupIterator(ctx, &k8s.ListNodeGroupsRequest{
		FolderId: cluster.FolderId,
	}).TakeAll()
	if err != nil {
		return nil, err
	}

	return lo.Filter(ngs, func(item *k8s.NodeGroup, _ int) bool {
		return item.ClusterId == p.clusterID && item.Labels["managed-by"] == "karpenter"
	}), nil
}

func (p *YCSDK) GetNodeFromNodeGroup(ctx context.Context, nodeGroupId string) (*k8s.Node, error) {
	nodes, err := p.SDK.Kubernetes().NodeGroup().ListNodes(ctx, &k8s.ListNodeGroupNodesRequest{
		NodeGroupId: nodeGroupId,
	})
	if err != nil {
		return nil, err
	}
	if len(nodes.Nodes) == 0 {
		return nil, fmt.Errorf("nodes not found")
	}
	return nodes.Nodes[0], nil
}

func (p *YCSDK) SecurityGroupExists(ctx context.Context, securityGroupId string) (bool, error) {
	sg, err := p.SDK.VPC().SecurityGroup().Get(ctx, &vpc.GetSecurityGroupRequest{
		SecurityGroupId: securityGroupId,
	})
	if err == nil {
		networkID, err := p.NetworkID(ctx)
		if err != nil {
			return false, err
		}
		if sg.NetworkId != "" && sg.NetworkId != networkID {
			return false, nil
		}
		return true, nil
	}

	if grpcstatus.Code(err) == codes.NotFound {
		return false, nil
	}
	return false, err
}
