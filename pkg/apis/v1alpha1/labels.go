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
	"github.com/sergelogvinov/karpenter-provider-proxmox/pkg/apis"

	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

const (
	// Labels that can be selected on and are propagated to the node
	LabelInstanceFamily       = apis.Group + "/instance-family"        // s1, s2, c1, c2, m1, m2, g1, g2
	LabelInstanceCPUPlatform  = apis.Group + "/instance-cpu-platform"  // intel-cascade-lake, intel-ice-lake, etc
	LabelInstanceGPUType      = apis.Group + "/instance-gpu-type"      // nvidia-tesla-v100, nvidia-tesla-a100, etc
	LabelInstanceGPUCount     = apis.Group + "/instance-gpu-count"     // 1, 2, 4, 8
	LabelInstanceCPU          = apis.Group + "/instance-cpu"           // 2, 4, 8, 16, 32, 64, 128
	LabelInstanceMemory       = apis.Group + "/instance-memory"        // 1Gi, 2Gi, 4Gi, 8Gi, 16Gi, 32Gi, 64Gi, 128Gi

	// github.com/awslabs/eks-node-viewer label so that it shows up.
	LabelNodeViewer = "eks-node-viewer/instance-price"

	// Internal labels that are propagated to the node
	YandexLabelKey   = apis.Group + "/node"
	YandexLabelValue = "owned"

	// Annotations
	AnnotationImageID    = apis.Group + "/image-id"
	AnnotationFolderID   = apis.Group + "/folder-id"
	AnnotationInstanceID = apis.Group + "/instance-id"
)

func init() {
	v1.RestrictedLabelDomains = v1.RestrictedLabelDomains.Insert(apis.Group)
	v1.WellKnownLabels = v1.WellKnownLabels.Insert(
		LabelInstanceFamily,
		LabelInstanceCPUPlatform,
		LabelInstanceGPUType,
		LabelInstanceGPUCount,
		LabelInstanceCPU,
		LabelInstanceMemory,
	)
}