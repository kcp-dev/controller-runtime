/*
Copyright 2024 The Kubernetes Authors.

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

package cache_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("KCP cluster-unaware informer cache", func() {
	// Test whether we can have a cluster-unaware informer cache against a single workspace.
	// I.e. every object has a kcp.io/cluster annotation, but it should not be taken
	// into consideration by the cache to compute the key.
	It("should be able to get the default namespace despite kcp.io/cluster annotation", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c, err := cache.New(cfg, cache.Options{})
		Expect(err).NotTo(HaveOccurred())

		By("Annotating the default namespace with kcp.io/cluster")
		cl, err := client.New(cfg, client.Options{})
		Expect(err).NotTo(HaveOccurred())
		ns := &corev1.Namespace{}
		err = cl.Get(ctx, client.ObjectKey{Name: "default"}, ns)
		Expect(err).NotTo(HaveOccurred())
		ns.Annotations = map[string]string{"kcp.io/cluster": "cluster1"}
		err = cl.Update(ctx, ns)
		Expect(err).NotTo(HaveOccurred())

		go c.Start(ctx) //nolint:errcheck // Start is blocking, and error not relevant here.
		c.WaitForCacheSync(ctx)

		By("By getting the default namespace with the informer")
		err = c.Get(ctx, client.ObjectKey{Name: "default"}, ns)
		Expect(err).NotTo(HaveOccurred())
	})
})

// TODO: get envtest in place with kcp
/*
var _ = Describe("KCP cluster-aware informer cache", func() {
	It("should be able to get the default namespace with kcp.io/cluster annotation", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c, err := kcp.NewClusterAwareCache(cfg, cache.Options{})
		Expect(err).NotTo(HaveOccurred())

		By("Annotating the default namespace with kcp.io/cluster")
		cl, err := client.New(cfg, client.Options{})
		Expect(err).NotTo(HaveOccurred())
		ns := &corev1.Namespace{}
		err = cl.Get(ctx, client.ObjectKey{Name: "default"}, ns)
		Expect(err).NotTo(HaveOccurred())
		ns.Annotations = map[string]string{"kcp.io/cluster": "cluster1"}
		err = cl.Update(ctx, ns)
		Expect(err).NotTo(HaveOccurred())

		go c.Start(ctx) //nolint:errcheck // Start is blocking, and error not relevant here.
		c.WaitForCacheSync(ctx)

		By("By getting the default namespace with the informer, but cluster-less key should fail")
		err = c.Get(ctx, client.ObjectKey{Name: "default"}, ns)
		Expect(err).To(HaveOccurred())

		By("By getting the default namespace with the informer, but cluster-aware key should succeed")
		err = c.Get(kontext.WithCluster(ctx, "cluster1"), client.ObjectKey{Name: "default", Namespace: "cluster1"}, ns)
	})
})
*/
