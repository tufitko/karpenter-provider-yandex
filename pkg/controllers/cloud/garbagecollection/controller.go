package garbagecollection

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/awslabs/operatorpkg/reconciler"
	"github.com/awslabs/operatorpkg/singleton"
	"github.com/tufitko/karpenter-provider-yandex/pkg/yandex"
	"github.com/yandex-cloud/go-genproto/yandex/cloud/k8s/v1"
	"k8s.io/utils/clock"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/karpenter/pkg/operator/injection"
)

// Controller deletes duplicated node groups from cloudprovider
type Controller struct {
	clk clock.Clock
	sdk yandex.SDK
}

func NewController(
	clk clock.Clock,
	sdk yandex.SDK,
) *Controller {
	return &Controller{
		clk: clk,
		sdk: sdk,
	}
}

func (c *Controller) Reconcile(ctx context.Context) (reconciler.Result, error) {
	ctx = injection.WithControllerName(ctx, "cloud.garbagecollection")

	log.FromContext(ctx).Info("garbage collection start")

	nodeGroups, err := c.sdk.ListNodeGroups(ctx)
	if err != nil {
		return reconciler.Result{}, fmt.Errorf("listing node groups: %w", err)
	}

	for _, nodeGroup := range nodeGroups {
		ctx2 := log.IntoContext(ctx, log.FromContext(ctx).WithValues(
			"nodeGroupId", nodeGroup.Id,
			"nodeGroupName", nodeGroup.Name,
		))
		node, err2 := c.sdk.GetNodeFromNodeGroup(ctx2, nodeGroup.Id)
		if err2 != nil {
			log.FromContext(ctx2).Error(err2, "failed to get node from node group")
			continue
		}

		if nodeGroup.Status != k8s.NodeGroup_PROVISIONING {
			continue
		}
		if node.CloudStatus.GetStatus() != "CREATING_INSTANCE" {
			continue
		}
		if !strings.Contains(node.CloudStatus.GetStatusMessage(), "ALREADY_EXISTS") {
			continue
		}

		err2 = c.sdk.DeleteNodeGroup(ctx2, nodeGroup.Id)
		if err2 != nil {
			log.FromContext(ctx2).Error(err2, "failed to delete node group")
		}
		log.FromContext(ctx2).Info("delete duplicated node group")
	}

	log.FromContext(ctx).Info("garbage collection end")

	return reconciler.Result{RequeueAfter: time.Minute * 10}, nil
}

func (c *Controller) Register(_ context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		Named("cloud.garbagecollection").
		WatchesRawSource(singleton.Source()).
		Complete(singleton.AsReconciler(c))
}
