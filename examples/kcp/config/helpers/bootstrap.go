package helpers

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"text/template"
	"time"

	extensionsapiserver "k8s.io/apiextensions-apiserver/pkg/apiserver"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	apimachineryerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Bootstrap creates resources in a package's fs by
// continuously retrying the list. This is blocking, i.e. it only returns (with error)
// when the context is closed or with nil when the bootstrapping is successfully completed.
func Bootstrap(ctx context.Context, client client.Client, fs embed.FS) error {
	// bootstrap non-crd resources
	return wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
		if err := CreateResourcesFromFS(ctx, client, fs); err != nil {
			log.FromContext(ctx).WithValues("err", err).Info("failed to bootstrap resources, retrying")
			return false, nil
		}
		return true, nil
	})
}

// CreateResourcesFromFS creates all resources from a filesystem.
func CreateResourcesFromFS(ctx context.Context, client client.Client, fs embed.FS) error {
	files, err := fs.ReadDir(".")
	if err != nil {
		return err
	}

	var errs []error
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if err := CreateResourceFromFS(ctx, client, f.Name(), fs); err != nil {
			errs = append(errs, err)
		}
	}
	return apimachineryerrors.NewAggregate(errs)
}

// CreateResourceFromFS creates given resource file.
func CreateResourceFromFS(ctx context.Context, client client.Client, filename string, fs embed.FS) error {
	raw, err := fs.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("could not read %s: %w", filename, err)
	}

	if len(raw) == 0 {
		return nil // ignore empty files
	}

	d := kubeyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(raw)))
	var errs []error
	for i := 1; ; i++ {
		doc, err := d.Read()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return err
		}
		if len(bytes.TrimSpace(doc)) == 0 {
			continue
		}

		if err := createResourceFromFS(ctx, client, doc); err != nil {
			errs = append(errs, fmt.Errorf("failed to create resource %s doc %d: %w", filename, i, err))
		}
	}
	return apimachineryerrors.NewAggregate(errs)
}

func createResourceFromFS(ctx context.Context, client client.Client, raw []byte) error {
	log := log.FromContext(ctx)

	type Input struct {
		Batteries map[string]bool
	}
	input := Input{
		Batteries: map[string]bool{},
	}
	tmpl, err := template.New("manifest").Parse(string(raw))
	if err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, input); err != nil {
		return fmt.Errorf("failed to execute manifest: %w", err)
	}

	obj, gvk, err := extensionsapiserver.Codecs.UniversalDeserializer().Decode(buf.Bytes(), nil, &unstructured.Unstructured{})
	if err != nil {
		return fmt.Errorf("could not decode raw: %w", err)
	}
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("decoded into incorrect type, got %T, wanted %T", obj, &unstructured.Unstructured{})
	}

	key := types.NamespacedName{
		Namespace: u.GetNamespace(),
		Name:      u.GetName(),
	}
	err = client.Create(ctx, u)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			err = client.Get(ctx, key, u)
			if err != nil {
				return err
			}

			u.SetResourceVersion(u.GetResourceVersion())
			err = client.Update(ctx, u)
			if err != nil {
				return fmt.Errorf("could not update %s %s: %w", gvk.Kind, key.String(), err)
			} else {
				log.WithValues("resource", u.GetName(), "kind", gvk.Kind).Info("updated object")
				return nil
			}
		}
		return err
	}

	log.WithValues("resource", u.GetName(), "kind", gvk.Kind).Info("created object")

	return nil
}
