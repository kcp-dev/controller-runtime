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

package configmap

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/kcp"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/kontext"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Reconciler struct {
	Client client.Client
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("cluster", req.ClusterName)

	// Test get
	var cm corev1.ConfigMap
	if err := r.Client.Get(ctx, req.NamespacedName, &cm); err != nil {
		log.Error(err, "unable to get configmap")
		return ctrl.Result{}, nil
	}

	log.Info("Get: retrieved configMap")
	if cm.Labels["name"] != "" {
		response := fmt.Sprintf("hello-%s", cm.Labels["name"])

		if cm.Labels["response"] != response {
			cm.Labels["response"] = response

			// Test Update
			if err := r.Client.Update(ctx, &cm); err != nil {
				return ctrl.Result{}, err
			}

			log.Info("Update: updated configMap")
			return ctrl.Result{}, nil
		}
	}

	// Test list
	var cms corev1.ConfigMapList
	if err := r.Client.List(ctx, &cms); err != nil {
		log.Error(err, "unable to list configmaps")
		return ctrl.Result{}, nil
	}
	log.Info("List: got", "itemCount", len(cms.Items))
	found := false
	for _, other := range cms.Items {
		cluster, ok := kontext.ClusterFrom(ctx)
		if !ok {
			log.Info("List: got", "clusterName", cluster.String(), "namespace", other.Namespace, "name", other.Name)
		} else if other.Name == cm.Name && other.Namespace == cm.Namespace {
			if found {
				return ctrl.Result{}, fmt.Errorf("there should be listed only one configmap with the given name '%s' for the given namespace '%s' when the clusterName is not available", cm.Name, cm.Namespace)
			}
			found = true
			log.Info("Found in listed configmaps", "namespace", cm.Namespace, "name", cm.Name)
		}
	}

	// If the configmap has a namespace field, create the corresponding namespace
	nsName, exists := cm.Data["namespace"]
	if exists {
		var namespace corev1.Namespace
		if err := r.Client.Get(ctx, types.NamespacedName{Name: nsName}, &namespace); err != nil {
			if !apierrors.IsNotFound(err) {
				log.Error(err, "unable to get namespace")
				return ctrl.Result{}, err
			}

			// Need to create ns
			namespace.SetName(nsName)
			if err = r.Client.Create(ctx, &namespace); err != nil {
				log.Error(err, "unable to create namespace")
				return ctrl.Result{}, err
			}
			log.Info("Create: created ", "namespace", nsName)
			return ctrl.Result{Requeue: true}, nil
		}
		log.Info("Exists", "namespace", nsName)
	}

	// If the configmap has a secretData field, create a secret in the same namespace
	// If the secret already exists but is out of sync, it will be non-destructively patched
	secretData, exists := cm.Data["secretData"]
	if exists {
		var secret corev1.Secret
		secret.SetName(cm.GetName())
		secret.SetNamespace(cm.GetNamespace())
		secret.SetOwnerReferences([]metav1.OwnerReference{{
			Name:       cm.GetName(),
			UID:        cm.GetUID(),
			APIVersion: "v1",
			Kind:       "ConfigMap",
			Controller: func() *bool { x := true; return &x }(),
		}})
		secret.Data = map[string][]byte{"dataFromCM": []byte(secretData)}

		operationResult, err := controllerutil.CreateOrPatch(ctx, r.Client, &secret, func() error {
			secret.Data["dataFromCM"] = []byte(secretData)
			return nil
		})
		if err != nil {
			log.Error(err, "unable to create or patch secret")
			return ctrl.Result{}, err
		}
		log.Info(string(operationResult), "secret", secret.GetName())
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Complete(kcp.WithClusterInContext(r))
}
