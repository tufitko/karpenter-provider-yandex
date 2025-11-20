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

package v1alpha1

import (
	"github.com/tufitko/karpenter-provider-yandex/pkg/apis"

	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

const (
	TerminationFinalizer = apis.Group + "/termination"
	// Labels that can be selected on and are propagated to the node
	LabelInstanceCPUPlatform = apis.Group + "/instance-cpu-platform" // intel-cascade-lake, intel-ice-lake, etc
	LabelInstanceCPU         = apis.Group + "/instance-cpu"          // 2, 4, 8, 16, 32, 64, 128
	LabelInstanceMemory      = apis.Group + "/instance-memory"       // 1Gi, 2Gi, 4Gi, 8Gi, 16Gi, 32Gi, 64Gi, 128Gi
	LabelInstanceType        = apis.Group + "/instance-type"
	LabelInstanceCPUFraction = apis.Group + "/instance-cpu-fraction"
)

func init() {
	v1.RestrictedLabelDomains = v1.RestrictedLabelDomains.Insert(apis.Group)
	v1.WellKnownLabels = v1.WellKnownLabels.Insert(
		LabelInstanceCPUPlatform,
		LabelInstanceCPU,
		LabelInstanceMemory,
		LabelInstanceType,
		LabelInstanceCPUFraction,
	)
}
