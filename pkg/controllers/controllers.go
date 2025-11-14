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

package controllers

import (
	"context"

	"github.com/awslabs/operatorpkg/controller"
	"github.com/patrickmn/go-cache"
	"github.com/tufitko/karpenter-provider-yandex/pkg/controllers/nodeclaim/garbagecollection"
	"github.com/tufitko/karpenter-provider-yandex/pkg/controllers/nodeclass"
	"github.com/tufitko/karpenter-provider-yandex/pkg/providers/subnet"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/events"
)

func NewControllers(ctx context.Context,
	kubeClient client.Client, recorder events.Recorder,
	subnetProvider subnet.Provider,
	validationCache *cache.Cache,
	cloudProvider cloudprovider.CloudProvider,
) []controller.Controller {

	controllers := []controller.Controller{
		nodeclass.NewController(kubeClient, recorder, subnetProvider, validationCache, false),
		garbagecollection.NewController(kubeClient, cloudProvider),
	}

	return controllers
}
