package common

import (
	"context"
	"errors"
	"net"
	stdString "strings"
	"text/template"
	stdTime "time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"sigs.k8s.io/controller-runtime/pkg/cache"

	"github.com/go-sprout/sprout"
	"github.com/go-sprout/sprout/registry/checksum"
	"github.com/go-sprout/sprout/registry/conversion"
	"github.com/go-sprout/sprout/registry/crypto"
	"github.com/go-sprout/sprout/registry/encoding"
	"github.com/go-sprout/sprout/registry/filesystem"
	"github.com/go-sprout/sprout/registry/maps"
	"github.com/go-sprout/sprout/registry/network"
	"github.com/go-sprout/sprout/registry/numeric"
	"github.com/go-sprout/sprout/registry/random"
	"github.com/go-sprout/sprout/registry/reflect"
	"github.com/go-sprout/sprout/registry/regexp"
	"github.com/go-sprout/sprout/registry/semver"
	"github.com/go-sprout/sprout/registry/slices"
	"github.com/go-sprout/sprout/registry/std"
	"github.com/go-sprout/sprout/registry/strings"
	"github.com/go-sprout/sprout/registry/time"
	"github.com/go-sprout/sprout/registry/uniqueid"
	"github.com/minio/minio-go/v7"
	minioCredentials "github.com/minio/minio-go/v7/pkg/credentials"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TemplateRenderer struct {
	funcMap template.FuncMap
	cache   cache.Cache
	client  client.Client
}

func NewTemplateRenderer(cache cache.Cache, client client.Client) *TemplateRenderer {
	sproutHandler := sprout.New()
	if addRegistriesErr := sproutHandler.
		AddRegistries(checksum.NewRegistry(), checksum.NewRegistry(), conversion.NewRegistry(), crypto.NewRegistry(),
			encoding.NewRegistry(), filesystem.NewRegistry(), maps.NewRegistry(), network.NewRegistry(),
			numeric.NewRegistry(), random.NewRegistry(), reflect.NewRegistry(), regexp.NewRegistry(),
			semver.NewRegistry(), slices.NewRegistry(), std.NewRegistry(), strings.NewRegistry(),
			time.NewRegistry(), uniqueid.NewRegistry(),
		); addRegistriesErr != nil {
		panic(addRegistriesErr)
	}

	templateRenderer := &TemplateRenderer{
		funcMap: sproutHandler.Build(),
		cache:   cache,
		client:  client,
	}

	templateRenderer.funcMap["kubernetesGet"] = templateRenderer.kubernetesGet
	templateRenderer.funcMap["kubernetesList"] = templateRenderer.kubernetesList
	templateRenderer.funcMap["minioPresignedGetObject"] = templateRenderer.minioPresignedGetObject
	templateRenderer.funcMap["netLookupIP"] = templateRenderer.netLookupIP

	return templateRenderer
}

func (t *TemplateRenderer) Parse(tmpl string) (*template.Template, error) {
	return template.New("").Funcs(t.funcMap).Parse(tmpl)
}

func (t *TemplateRenderer) Render(tmpl *template.Template, data any) (string, error) {
	script := &stdString.Builder{}
	if templateExecuteErr := tmpl.Execute(script, data); templateExecuteErr != nil {
		return "", templateExecuteErr
	}

	return script.String(), nil
}

func (t *TemplateRenderer) kubernetesList(apiVersionKind, namespace string) (any, error) {
	object := &unstructured.UnstructuredList{
		Object: map[string]interface{}{
			"apiVersion": apiVersionKind[0:stdString.Index(apiVersionKind, ";")],
			"kind":       apiVersionKind[stdString.Index(apiVersionKind, ";")+1:],
		},
	}

	listErr := t.cache.List(context.Background(), object, &client.ListOptions{
		Namespace: namespace,
	})

	return object.Items, listErr
}

func (t *TemplateRenderer) kubernetesGet(apiVersionKind, namespacedName string) (any, error) {
	object := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": apiVersionKind[0:stdString.Index(apiVersionKind, ";")],
			"kind":       apiVersionKind[stdString.Index(apiVersionKind, ";")+1:],
		},
	}
	namespaceName := stdString.Split(namespacedName, "/")

	getErr := t.cache.Get(context.Background(), types.NamespacedName{
		Name:      namespaceName[0],
		Namespace: namespaceName[1],
	}, object)

	return object.Object, getErr
}

func (t *TemplateRenderer) minioPresignedGetObject(endpoint, credentials,
	bucket, path string, expiry string,
) (string, error) {
	var (
		credentialsPart = 2
		parts           = stdString.Split(credentials, ":")
		secure          = stdString.HasPrefix(endpoint, "https://")
	)

	expiryDuration, parseDurationErr := stdTime.ParseDuration(expiry)
	if parseDurationErr != nil {
		return "", parseDurationErr
	}

	if len(parts) != credentialsPart {
		return "", errors.New("")
	}

	if stdString.HasPrefix(endpoint, "http://") {
		endpoint = endpoint[7:]
	} else if stdString.HasPrefix(endpoint, "https://") {
		endpoint = endpoint[8:]
	}

	minioClient, newMinioErr := minio.New(endpoint, &minio.Options{
		Creds:  minioCredentials.NewStaticV4(parts[0], parts[1], ""),
		Secure: secure,
	})
	if newMinioErr != nil {
		return "", newMinioErr
	}

	preSignedURL, preSignedGetObjectErr := minioClient.PresignedGetObject(context.Background(),
		bucket, path, expiryDuration, nil)
	if preSignedGetObjectErr != nil {
		return "", preSignedGetObjectErr
	}

	return preSignedURL.String(), nil
}

func (t *TemplateRenderer) netLookupIP(address string) []net.IP {
	ips, _ := net.LookupIP(address)

	return ips
}
