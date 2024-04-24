/*
Copyright 2024 The KCP Authors.

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

package widget

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/kcp"
	"sigs.k8s.io/controller-runtime/pkg/log"

	datav1alpha1 "github.com/kcp-dev/controller-runtime/examples/kcp/apis/v1alpha1"
)

// Reconciler reconciles a Widget object
type Reconciler struct {
	Client client.Client
}

// Reconcile TODO
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Include the clusterName from req.ObjectKey in the logger, similar to the namespace and name keys that are already
	// there.
	log = log.WithValues("clusterName", req.ClusterName)

	// You probably wouldn't need to do this, but if you wanted to list all instances across all logical clusters:
	var allWidgets datav1alpha1.WidgetList
	if err := r.Client.List(ctx, &allWidgets); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Listed all widgets across all workspaces", "count", len(allWidgets.Items))

	log.Info("Getting widget")
	var w datav1alpha1.Widget
	if err := r.Client.Get(ctx, req.NamespacedName, &w); err != nil {
		if errors.IsNotFound(err) {
			// Normal - was deleted
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	log.Info("Listing all widgets in the current logical cluster")
	var list datav1alpha1.WidgetList
	if err := r.Client.List(ctx, &list); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Patching widget status to store total widget count in the current logical cluster")
	orig := w.DeepCopy()
	w.Status.Total = len(list.Items)
	if err := r.Client.Status().Patch(ctx, &w, client.MergeFromWithOptions(orig, client.MergeFromWithOptimisticLock{})); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&datav1alpha1.Widget{}).
		Complete(kcp.WithClusterInContext(r))
}
