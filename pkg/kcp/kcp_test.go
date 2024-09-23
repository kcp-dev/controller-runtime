package kcp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/kontext"
)

var _ = Describe("NewClusterAwareClient", Ordered, func() {
	var (
		srv   *httptest.Server
		mu    sync.Mutex
		paths []string
		cfg   *rest.Config
	)

	BeforeAll(func() {
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			mu.Lock()
			paths = append(paths, req.URL.Path)
			mu.Unlock()

			switch req.URL.Path {
			case "/api/v1", "/clusters/root/api/v1", "/clusters/*/api/v1":
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"pods","singularName":"pod","namespaced":true,"kind":"Pod","verbs":["create","delete","deletecollection","get","list","patch","update","watch"],"shortNames":["po"],"categories":["all"],"storageVersionHash":"xPOwRZ+Yhw8="}]}`))
			case "/api/v1/pods", "/clusters/root/api/v1/pods", "/clusters/*/api/v1/pods":
				if req.URL.Query().Get("watch") != "true" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"kind": "PodList","apiVersion": "v1","metadata": {"resourceVersion": "184126176"}, "items": [{"kind":"Pod","apiVersion":"v1","metadata":{"name":"foo","namespace":"default","resourceVersion":"184126176"}}]}`))
					return
				}
				fallthrough
			case "/api/v1/namespaces/default/pods/foo", "/clusters/root/api/v1/namespaces/default/pods/foo":
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"kind":"Pod","apiVersion":"v1","metadata":{"name":"foo","namespace":"default","resourceVersion":"184126176"}}`))
			default:
				_, _ = w.Write([]byte(fmt.Sprintf("Not found %q", req.RequestURI)))
				w.WriteHeader(http.StatusNotFound)
			}
		}))

		cfg = &rest.Config{
			Host: srv.URL,
		}
		Expect(rest.SetKubernetesDefaults(cfg)).To(Succeed())
	})

	BeforeEach(func() {
		mu.Lock()
		defer mu.Unlock()
		paths = []string{}
	})

	AfterAll(func() {
		srv.Close()
	})

	Describe("with typed list", func() {
		It("should work with no cluster in the kontext", func(ctx context.Context) {
			cl, err := NewClusterAwareClient(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())

			pods := &corev1.PodList{}
			err = cl.List(ctx, pods)
			Expect(err).NotTo(HaveOccurred())

			pod := &corev1.Pod{}
			err = cl.Get(ctx, types.NamespacedName{Namespace: "default", Name: "foo"}, pod)
			Expect(err).NotTo(HaveOccurred())

			mu.Lock()
			defer mu.Unlock()
			Expect(paths).To(Equal([]string{"/api/v1", "/api/v1/pods", "/api/v1/namespaces/default/pods/foo"}))
		})

		It("should work with a cluster in the kontext", func(ctx context.Context) {
			cl, err := NewClusterAwareClient(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())

			pods := &corev1.PodList{}
			err = cl.List(kontext.WithCluster(ctx, "root"), pods)
			Expect(err).NotTo(HaveOccurred())

			pod := &corev1.Pod{}
			err = cl.Get(kontext.WithCluster(ctx, "root"), types.NamespacedName{Namespace: "default", Name: "foo"}, pod)
			Expect(err).NotTo(HaveOccurred())

			mu.Lock()
			defer mu.Unlock()
			Expect(paths).To(Equal([]string{"/clusters/root/api/v1", "/clusters/root/api/v1/pods", "/clusters/root/api/v1/namespaces/default/pods/foo"}))
		})

		It("should work with a wildcard cluster in the kontext", func(ctx context.Context) {
			cl, err := NewClusterAwareClient(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())

			pods := &corev1.PodList{}
			err = cl.List(kontext.WithCluster(ctx, "*"), pods)
			Expect(err).NotTo(HaveOccurred())

			mu.Lock()
			defer mu.Unlock()
			Expect(paths).To(Equal([]string{"/clusters/*/api/v1", "/clusters/*/api/v1/pods"}))
		})
	})

	Describe("with unstructured list", func() {
		It("should work with no cluster in the kontext", func(ctx context.Context) {
			cl, err := NewClusterAwareClient(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())

			pods := &unstructured.UnstructuredList{}
			pods.SetAPIVersion("v1")
			pods.SetKind("PodList")
			err = cl.List(ctx, pods)
			Expect(err).NotTo(HaveOccurred())

			pod := &unstructured.Unstructured{}
			pod.SetAPIVersion("v1")
			pod.SetKind("Pod")
			err = cl.Get(ctx, types.NamespacedName{Namespace: "default", Name: "foo"}, pod)
			Expect(err).NotTo(HaveOccurred())

			mu.Lock()
			defer mu.Unlock()
			Expect(paths).To(Equal([]string{"/api/v1", "/api/v1/pods", "/api/v1/namespaces/default/pods/foo"}))
		})

		It("should work with a cluster in the kontext", func(ctx context.Context) {
			cl, err := NewClusterAwareClient(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())

			pods := &unstructured.UnstructuredList{}
			pods.SetAPIVersion("v1")
			pods.SetKind("PodList")
			err = cl.List(kontext.WithCluster(ctx, "root"), pods)
			Expect(err).NotTo(HaveOccurred())

			pod := &unstructured.Unstructured{}
			pod.SetAPIVersion("v1")
			pod.SetKind("Pod")
			err = cl.Get(kontext.WithCluster(ctx, "root"), types.NamespacedName{Namespace: "default", Name: "foo"}, pod)
			Expect(err).NotTo(HaveOccurred())

			mu.Lock()
			defer mu.Unlock()
			Expect(paths).To(Equal([]string{"/clusters/root/api/v1", "/clusters/root/api/v1/pods", "/clusters/root/api/v1/namespaces/default/pods/foo"}))
		})

		It("should work with a wildcard cluster in the kontext", func(ctx context.Context) {
			cl, err := NewClusterAwareClient(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())

			pods := &unstructured.UnstructuredList{}
			pods.SetAPIVersion("v1")
			pods.SetKind("PodList")
			err = cl.List(kontext.WithCluster(ctx, "*"), pods)
			Expect(err).NotTo(HaveOccurred())

			mu.Lock()
			defer mu.Unlock()
			Expect(paths).To(Equal([]string{"/clusters/*/api/v1", "/clusters/*/api/v1/pods"}))
		})
	})

	Describe("with a metadata object", func() {
		It("should work with no cluster in the kontext", func(ctx context.Context) {
			cl, err := NewClusterAwareClient(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())

			pods := &metav1.PartialObjectMetadataList{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PodList"}}
			err = cl.List(ctx, pods)
			Expect(err).NotTo(HaveOccurred())

			mu.Lock()
			defer mu.Unlock()
			Expect(paths).To(Equal([]string{"/api/v1", "/api/v1/pods"}))
		})

		It("should work with a cluster in the kontext", func(ctx context.Context) {
			cl, err := NewClusterAwareClient(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())

			pods := &metav1.PartialObjectMetadataList{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PodList"}}
			err = cl.List(kontext.WithCluster(ctx, "root"), pods)
			Expect(err).NotTo(HaveOccurred())

			mu.Lock()
			defer mu.Unlock()
			Expect(paths).To(Equal([]string{"/clusters/root/api/v1", "/clusters/root/api/v1/pods"}))
		})

		It("should work with a wildcard cluster in the kontext", func(ctx context.Context) {
			cl, err := NewClusterAwareClient(cfg, client.Options{})
			Expect(err).NotTo(HaveOccurred())

			pods := &metav1.PartialObjectMetadataList{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PodList"}}
			err = cl.List(kontext.WithCluster(ctx, "*"), pods)
			Expect(err).NotTo(HaveOccurred())

			mu.Lock()
			defer mu.Unlock()
			Expect(paths).To(Equal([]string{"/clusters/*/api/v1", "/clusters/*/api/v1/pods"}))
		})
	})
})

var _ = Describe("NewClusterAwareCache", Ordered, func() {
	var (
		cancelCtx context.CancelFunc
		srv       *httptest.Server
		mu        sync.Mutex
		paths     []string
		cfg       *rest.Config
		c         cache.Cache
	)

	BeforeAll(func() {
		var ctx context.Context
		ctx, cancelCtx = context.WithCancel(context.Background())

		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			mu.Lock()
			pth := req.URL.Path
			if req.URL.Query().Get("watch") == "true" {
				pth += "?watch=true"
			}
			paths = append(paths, pth)
			mu.Unlock()

			switch {
			case req.URL.Path == "/clusters/*/api/v1":
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"pods","singularName":"pod","namespaced":true,"kind":"Pod","verbs":["create","delete","deletecollection","get","list","patch","update","watch"],"shortNames":["po"],"categories":["all"],"storageVersionHash":"xPOwRZ+Yhw8="}]}`))
			case req.URL.Path == "/clusters/*/api/v1/pods" && req.URL.Query().Get("watch") != "true":
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"kind": "PodList","apiVersion": "v1","metadata": {"resourceVersion": "184126176"}, "items": [
					{"kind":"Pod","apiVersion":"v1","metadata":{"name":"foo","namespace":"default","resourceVersion":"184126176","annotations":{"kcp.io/cluster":"root"}}},
					{"kind":"Pod","apiVersion":"v1","metadata":{"name":"foo","namespace":"default","resourceVersion":"184126093","annotations":{"kcp.io/cluster":"ws"}}}
				]}`))
			case req.URL.Path == "/clusters/*/api/v1/pods" && req.URL.Query().Get("watch") == "true":
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Transfer-Encoding", "chunked")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"type":"ADDED","object":{"kind":"Pod","apiVersion":"v1","metadata":{"name":"bar","namespace":"default","resourceVersion":"184126177","annotations":{"kcp.io/cluster":"root"}}}}`))
				_, _ = w.Write([]byte(`{"type":"ADDED","object":{"kind":"Pod","apiVersion":"v1","metadata":{"name":"bar","namespace":"default","resourceVersion":"184126178","annotations":{"kcp.io/cluster":"ws"}}}}`))
				if w, ok := w.(http.Flusher); ok {
					w.Flush()
				}
				time.Sleep(1 * time.Second)
			default:
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(fmt.Sprintf("Not found %q", req.RequestURI)))
			}
		}))
		go func() {
			<-ctx.Done()
			srv.Close()
		}()

		cfg = &rest.Config{
			Host: srv.URL,
		}
		Expect(rest.SetKubernetesDefaults(cfg)).To(Succeed())

		var err error
		c, err = NewClusterAwareCache(cfg, cache.Options{})
		Expect(err).NotTo(HaveOccurred())
		go func() {
			if err := c.Start(ctx); err != nil {
				Expect(err).NotTo(HaveOccurred())
			}
		}()
		c.WaitForCacheSync(ctx)
	})

	BeforeEach(func() {
		mu.Lock()
		defer mu.Unlock()
		paths = []string{}
	})

	AfterAll(func() {
		cancelCtx()
	})

	It("should always access wildcard clusters and serve other clusters from memory", func(ctx context.Context) {
		pod := &corev1.Pod{}
		err := c.Get(kontext.WithCluster(ctx, "root"), types.NamespacedName{Namespace: "default", Name: "foo"}, pod)
		Expect(err).NotTo(HaveOccurred())

		mu.Lock()
		defer mu.Unlock()
		Expect(paths).To(Equal([]string{"/clusters/*/api/v1", "/clusters/*/api/v1/pods", "/clusters/*/api/v1/pods?watch=true"}))
	})

	It("should return only the pods from the requested cluster", func(ctx context.Context) {
		pod := &corev1.Pod{}
		err := c.Get(kontext.WithCluster(ctx, "root"), types.NamespacedName{Namespace: "default", Name: "foo"}, pod)
		Expect(err).NotTo(HaveOccurred())
		Expect(pod.Annotations).To(HaveKeyWithValue("kcp.io/cluster", "root"))

		pods := &corev1.PodList{}
		err = c.List(kontext.WithCluster(ctx, "root"), pods)
		Expect(err).NotTo(HaveOccurred())
		Expect(pods.Items).To(HaveLen(2))
		Expect(pods.Items[0].Annotations).To(HaveKeyWithValue("kcp.io/cluster", "root"))
		Expect(pods.Items[1].Annotations).To(HaveKeyWithValue("kcp.io/cluster", "root"))
		Expect(sets.New(pods.Items[0].Name, pods.Items[1].Name)).To(Equal(sets.New("foo", "bar")))
	})

	It("should return all pods from all clusters without cluster in context", func(ctx context.Context) {
		pods := &corev1.PodList{}
		err := c.List(ctx, pods)
		Expect(err).NotTo(HaveOccurred())
		Expect(pods.Items).To(HaveLen(4))
	})
})
