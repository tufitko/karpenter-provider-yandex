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

// Package main
package main

import (
	"flag"
	"os"

	"github.com/tufitko/karpenter-provider-yandex/pkg/operator"
	"sigs.k8s.io/karpenter/pkg/cloudprovider/metrics"
	corecontrollers "sigs.k8s.io/karpenter/pkg/controllers"
	"sigs.k8s.io/karpenter/pkg/controllers/state"
	coreoperator "sigs.k8s.io/karpenter/pkg/operator"

	yandex "github.com/tufitko/karpenter-provider-yandex/pkg/cloudprovider"
	"github.com/tufitko/karpenter-provider-yandex/pkg/controllers"
)

func main() {
	flag.Parse()

	ctx, op := operator.NewOperator(coreoperator.NewOperator())

	log := op.GetLogger()
	log.Info("Karpenter Yandex Cloud Provider version", "version", coreoperator.Version)

	yandexCloudProvider, err := yandex.NewCloudProvider(
		ctx,
		op.GetClient(),
		op.SDK,
		op.EventRecorder,
		op.InstanceTypeProvider,
		op.SubnetProvider,
	)
	if err != nil {
		log.Error(err, "failed creating yandex provider")
		os.Exit(1)
	}
	cloudProvider := metrics.Decorate(yandexCloudProvider)
	overlayUndecoratedCloudProvider := metrics.Decorate(cloudProvider)
	clusterState := state.NewCluster(op.Clock, op.GetClient(), cloudProvider)

	op.
		WithControllers(ctx, corecontrollers.NewControllers(
			ctx,
			op.Manager,
			op.Clock,
			op.GetClient(),
			op.EventRecorder,
			cloudProvider,
			overlayUndecoratedCloudProvider,
			clusterState,
			op.InstanceTypeStore,
		)...).
		WithControllers(ctx, controllers.NewControllers(
			ctx,
			op.GetClient(),
			op.EventRecorder,
			op.SubnetProvider,
			op.ValidationCache,
			cloudProvider,
		)...).
		Start(ctx)
}
