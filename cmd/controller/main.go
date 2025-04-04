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
	"os"

	"sigs.k8s.io/karpenter/pkg/cloudprovider/metrics"
	corecontrollers "sigs.k8s.io/karpenter/pkg/controllers"
	"sigs.k8s.io/karpenter/pkg/controllers/state"
	coreoperator "sigs.k8s.io/karpenter/pkg/operator"

	yandex "github.com/tufitko/karpenter-provider-yandex/pkg/cloudprovider"
	"github.com/tufitko/karpenter-provider-yandex/pkg/controllers"
	"github.com/tufitko/karpenter-provider-yandex/pkg/providers/cloudcapacity"
	"github.com/tufitko/karpenter-provider-yandex/pkg/providers/instance"
)

func main() {
	ctx, op := coreoperator.NewOperator()
	log := op.GetLogger()

	log.Info("Karpenter Yandex Cloud Provider version", "version", coreoperator.Version)

	cloudcapacityProvider, err := cloudcapacity.NewProvider(ctx)
	if err != nil {
		log.Error(err, "failed creating cloudcapacity provider")
		os.Exit(1)
	}

	cloudcapacityProvider.Sync(ctx)

	instanceTypes, err := yandex.ConstructInstanceTypes(ctx, cloudcapacityProvider)
	if err != nil {
		log.Error(err, "failed constructing instance types")
		os.Exit(1)
	}

	instanceProvider, err := instance.NewProvider(cloudcapacityProvider)
	if err != nil {
		log.Error(err, "failed creating instance provider")
		os.Exit(1)
	}

	yandexCloudProvider := yandex.NewCloudProvider(ctx, op.GetClient(), instanceTypes, instanceProvider, cloudcapacityProvider)
	cloudProvider := metrics.Decorate(yandexCloudProvider)
	clusterState := state.NewCluster(op.Clock, op.GetClient(), cloudProvider)

	op.
		WithControllers(ctx, corecontrollers.NewControllers(
			ctx,
			op.Manager,
			op.Clock,
			op.GetClient(),
			op.EventRecorder,
			cloudProvider,
			clusterState,
		)...).
		WithControllers(ctx, controllers.NewControllers(
			ctx,
			op.Manager,
			op.Clock,
			op.GetClient(),
			op.EventRecorder,
			cloudProvider,
		)...).
		Start(ctx)
}