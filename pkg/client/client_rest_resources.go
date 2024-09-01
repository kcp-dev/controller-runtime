/*
Copyright 2018 The Kubernetes Authors.

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

package client

import (
	"context"
	"net/http"
	"strings"
	"sync"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/kcp-dev/logicalcluster/v3"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/kontext"
)

type clusterResources struct {
	mapper meta.RESTMapper

	// structuredResourceByType stores structured type metadata
	structuredResourceByType map[schema.GroupVersionKind]*resourceMeta
	// unstructuredResourceByType stores unstructured type metadata
	unstructuredResourceByType map[schema.GroupVersionKind]*resourceMeta
}

// clientRestResources creates and stores rest clients and metadata for Kubernetes types.
type clientRestResources struct {
	// httpClient is the http client to use for requests
	httpClient *http.Client

	// config is the rest.Config to talk to an apiserver
	config *rest.Config

	// scheme maps go structs to GroupVersionKinds
	scheme *runtime.Scheme

	// mapper maps GroupVersionKinds to Resources
	mapper func(ctx context.Context) (meta.RESTMapper, error)

	// codecs are used to create a REST client for a gvk
	codecs serializer.CodecFactory

	clusterResources *lru.Cache[logicalcluster.Path, clusterResources]
	mu               sync.RWMutex
}

// newResource maps obj to a Kubernetes Resource and constructs a client for that Resource.
// If the object is a list, the resource represents the item's type instead.
func (c *clientRestResources) newResource(gvk schema.GroupVersionKind, isList, isUnstructured bool, mapper meta.RESTMapper) (*resourceMeta, error) {
	if strings.HasSuffix(gvk.Kind, "List") && isList {
		// if this was a list, treat it as a request for the item's resource
		gvk.Kind = gvk.Kind[:len(gvk.Kind)-4]
	}

	client, err := apiutil.RESTClientForGVK(gvk, isUnstructured, c.config, c.codecs, c.httpClient)
	if err != nil {
		return nil, err
	}
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, err
	}
	return &resourceMeta{Interface: client, mapping: mapping, gvk: gvk}, nil
}

// getResource returns the resource meta information for the given type of object.
// If the object is a list, the resource represents the item's type instead.
func (c *clientRestResources) getResource(ctx context.Context, obj runtime.Object) (*resourceMeta, error) {
	gvk, err := apiutil.GVKForObject(obj, c.scheme)
	if err != nil {
		return nil, err
	}

	_, isUnstructured := obj.(runtime.Unstructured)

	// It's better to do creation work twice than to not let multiple
	// people make requests at once
	c.mu.RLock()
	cluster, _ := kontext.ClusterFrom(ctx)
	cr, found := c.clusterResources.Get(cluster.Path())
	if !found {
		m, err := c.mapper(ctx)
		if err != nil {
			c.mu.RUnlock()
			return nil, err
		}
		cr = clusterResources{
			mapper:                     m,
			structuredResourceByType:   make(map[schema.GroupVersionKind]*resourceMeta),
			unstructuredResourceByType: make(map[schema.GroupVersionKind]*resourceMeta),
		}
		c.clusterResources.Purge()
		c.clusterResources.Add(cluster.Path(), cr)
	}
	resourceByType := cr.structuredResourceByType
	if isUnstructured {
		resourceByType = cr.unstructuredResourceByType
	}
	r, known := resourceByType[gvk]
	c.mu.RUnlock()

	if known {
		return r, nil
	}

	// Initialize a new Client
	c.mu.Lock()
	defer c.mu.Unlock()
	r, err = c.newResource(gvk, meta.IsListType(obj), isUnstructured, cr.mapper)
	if err != nil {
		return nil, err
	}
	resourceByType[gvk] = r
	return r, err
}

// getObjMeta returns objMeta containing both type and object metadata and state.
func (c *clientRestResources) getObjMeta(ctx context.Context, obj runtime.Object) (*objMeta, error) {
	r, err := c.getResource(ctx, obj)
	if err != nil {
		return nil, err
	}
	m, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	return &objMeta{resourceMeta: r, Object: m}, err
}

// resourceMeta stores state for a Kubernetes type.
type resourceMeta struct {
	// client is the rest client used to talk to the apiserver
	rest.Interface
	// gvk is the GroupVersionKind of the resourceMeta
	gvk schema.GroupVersionKind
	// mapping is the rest mapping
	mapping *meta.RESTMapping
}

// isNamespaced returns true if the type is namespaced.
func (r *resourceMeta) isNamespaced() bool {
	return r.mapping.Scope.Name() != meta.RESTScopeNameRoot
}

// resource returns the resource name of the type.
func (r *resourceMeta) resource() string {
	return r.mapping.Resource.Resource
}

// objMeta stores type and object information about a Kubernetes type.
type objMeta struct {
	// resourceMeta contains type information for the object
	*resourceMeta

	// Object contains meta data for the object instance
	metav1.Object
}
