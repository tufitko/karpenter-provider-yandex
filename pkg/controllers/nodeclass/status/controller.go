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

package status

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/awslabs/operatorpkg/reasonable"
	"github.com/awslabs/operatorpkg/status"

	"github.com/tufitko/karpenter-provider-yandex/pkg/apis/v1alpha1"
)

const (
	// Condition types
	ConditionTypeAutoPlacement = "AutoPlacement"
)

// Controller reconciles an YandexNodeClass object to update its status
type Controller struct {
	kubeClient client.Client
}

// NewController constructs a controller instance
func NewController(kubeClient client.Client) (*Controller, error) {
	if kubeClient == nil {
		return nil, fmt.Errorf("kubeClient cannot be nil")
	}
	return &Controller{
		kubeClient: kubeClient,
	}, nil
}

func (c *Controller) Name() string {
	return "nodeclass.status"
}

// Reconcile executes a control loop for the resource
func (c *Controller) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	nc := &v1alpha1.YandexNodeClass{}
	if err := c.kubeClient.Get(ctx, req.NamespacedName, nc); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// Initialize status if needed
	if nc.Status.Conditions == nil {
		nc.Status.Conditions = []metav1.Condition{}
	}
	if nc.Status.SelectedInstanceTypes == nil {
		nc.Status.SelectedInstanceTypes = []string{}
	}

	// Validate the nodeclass configuration
	if err := c.validateNodeClass(ctx, nc); err != nil {
		patch := client.MergeFrom(nc.DeepCopy())
		nc.Status.LastValidationTime = metav1.Now()
		nc.Status.ValidationError = err.Error()
		if err := c.kubeClient.Status().Patch(ctx, nc, patch); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, err
	}

	// Clear any previous validation error
	if nc.Status.ValidationError != "" {
		patch := client.MergeFrom(nc.DeepCopy())
		nc.Status.LastValidationTime = metav1.Now()
		nc.Status.ValidationError = ""
		if err := c.kubeClient.Status().Patch(ctx, nc, patch); err != nil {
			return reconcile.Result{}, err
		}
	}

	if nc.Status.ValidationError == "" && len(nc.Status.SelectedInstanceTypes) == 0 {
		nc.Status.SelectedInstanceTypes = []string{"s2.2vcpu.8gb", "s2.4vcpu.16gb"}

		nc.StatusConditions().SetTrue(status.ConditionReady)

		c.updateCondition(nc, ConditionTypeAutoPlacement, metav1.ConditionTrue, "InstanceTypeSelectionSucceeded", "Instance type selection completed successfully")
		if err := c.kubeClient.Status().Update(ctx, nc); err != nil {
			return reconcile.Result{}, fmt.Errorf("updating nodeclass status: %w", err)
		}
	}

	return reconcile.Result{}, nil
}

// validateNodeClass performs validation of the YandexNodeClass configuration
func (c *Controller) validateNodeClass(_ context.Context, nc *v1alpha1.YandexNodeClass) error {
	if nc.Spec.CloudID == "" {
		return fmt.Errorf("CloudID is required")
	}
	
	if nc.Spec.FolderID == "" {
		return fmt.Errorf("FolderID is required")
	}
	
	if nc.Spec.NetworkID == "" {
		return fmt.Errorf("NetworkID is required")
	}
	
	if nc.Spec.ImageID == "" {
		return fmt.Errorf("ImageID is required")
	}

	return nil
}

// updateCondition updates a condition in the nodeclass status
func (c *Controller) updateCondition(nodeClass *v1alpha1.YandexNodeClass, conditionType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	newCondition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: nodeClass.Generation,
	}

	// Find and update existing condition or append new one
	for i, existingCond := range nodeClass.Status.Conditions {
		if existingCond.Type == conditionType {
			if existingCond.Status != status {
				nodeClass.Status.Conditions[i] = newCondition
			}
			return
		}
	}

	// Append new condition if not found
	nodeClass.Status.Conditions = append(nodeClass.Status.Conditions, newCondition)
}

// Register registers the controller with the manager
func (c *Controller) Register(_ context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		Named(c.Name()).
		For(&v1alpha1.YandexNodeClass{}).
		WithOptions(controller.Options{
			RateLimiter:             reasonable.RateLimiter(),
			MaxConcurrentReconciles: 1,
		}).
		Complete(c)
}