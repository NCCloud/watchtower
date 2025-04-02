package common

import (
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"text/template"

	mockCache "github.com/nccloud/watchtower/mocks/sigs.k8s.io/controller-runtime/pkg/cache"
)

func TestNewTemplateRenderer(t *testing.T) {
	//given
	mockClient := new(mockCache.MockCache)

	//when
	renderer := NewTemplateRenderer(mockClient)

	//then
	assert.NotNil(t, renderer)
	assert.NotNil(t, renderer.funcMap)
	assert.Equal(t, mockClient, renderer.kubernetesClient)

	// Check that custom functions are registered
	assert.NotNil(t, renderer.funcMap["kubernetesGet"])
	assert.NotNil(t, renderer.funcMap["kubernetesList"])
	assert.NotNil(t, renderer.funcMap["minioPresignedGetObject"])
	assert.NotNil(t, renderer.funcMap["netLookupIP"])
}

func TestTemplateRenderer_Parse(t *testing.T) {
	//given
	mockClient := new(mockCache.MockCache)
	renderer := NewTemplateRenderer(mockClient)
	templateStr := "Hello {{ . }}"

	//when
	tmpl, err := renderer.Parse(templateStr)

	//then
	require.NoError(t, err)
	assert.NotNil(t, tmpl)
}

func TestTemplateRenderer_Parse_ErrWhenInvalidTemplate(t *testing.T) {
	//given
	mockClient := new(mockCache.MockCache)
	renderer := NewTemplateRenderer(mockClient)
	templateStr := "Hello {{ . "

	//when
	_, err := renderer.Parse(templateStr)

	//then
	require.Error(t, err)
}

func TestTemplateRenderer_Render(t *testing.T) {
	//given
	mockClient := new(mockCache.MockCache)
	renderer := NewTemplateRenderer(mockClient)
	tmpl, _ := template.New("").Parse("Hello {{ . }}")
	data := "World"

	//when
	result, err := renderer.Render(tmpl, data)

	//then
	require.NoError(t, err)
	assert.Equal(t, "Hello World", result)
}

func TestTemplateRenderer_Render_ErrWhenExecuteFails(t *testing.T) {
	//given
	mockClient := new(mockCache.MockCache)
	renderer := NewTemplateRenderer(mockClient)
	tmpl, _ := template.New("").Parse("Hello {{ .MissingField }}")
	data := "World"

	//when
	_, err := renderer.Render(tmpl, data)

	//then
	require.Error(t, err)
}

func TestTemplateRenderer_KubernetesList(t *testing.T) {
	//given
	mockClient := new(mockCache.MockCache)
	renderer := NewTemplateRenderer(mockClient)
	apiVersionKind := "v1;ConfigMap"
	namespace := "default"

	expectedItems := []unstructured.Unstructured{
		{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "cm1",
					"namespace": "default",
				},
				"data": map[string]interface{}{
					"key1": "value1",
				},
			},
		},
		{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "cm2",
					"namespace": "default",
				},
				"data": map[string]interface{}{
					"key2": "value2",
				},
			},
		},
	}

	mockClient.On("List",
		mock.Anything,
		mock.MatchedBy(func(list *unstructured.UnstructuredList) bool {
			return list.Object["apiVersion"] == "v1" &&
				list.Object["kind"] == "ConfigMap"
		}),
		&client.ListOptions{Namespace: namespace}).
		Run(func(args mock.Arguments) {
			list := args.Get(1).(*unstructured.UnstructuredList)
			list.Items = expectedItems
		}).
		Return(nil)

	//when
	result, err := renderer.kubernetesList(apiVersionKind, namespace)

	//then
	require.NoError(t, err)
	items, ok := result.([]unstructured.Unstructured)
	require.True(t, ok)
	assert.Equal(t, 2, len(items))
	assert.Equal(t, "cm1", items[0].Object["metadata"].(map[string]interface{})["name"])
	assert.Equal(t, "cm2", items[1].Object["metadata"].(map[string]interface{})["name"])
	mockClient.AssertExpectations(t)
}

func TestTemplateRenderer_KubernetesList_ErrWhenListFails(t *testing.T) {
	//given
	mockClient := new(mockCache.MockCache)
	renderer := NewTemplateRenderer(mockClient)
	apiVersionKind := "v1;ConfigMap"
	namespace := "default"
	expectedErr := errors.New("list error")

	mockClient.On("List", mock.Anything, mock.Anything, mock.Anything).
		Return(expectedErr)

	//when
	_, err := renderer.kubernetesList(apiVersionKind, namespace)

	//then
	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
	mockClient.AssertExpectations(t)
}

func TestTemplateRenderer_KubernetesGet(t *testing.T) {
	//given
	mockClient := new(mockCache.MockCache)
	renderer := NewTemplateRenderer(mockClient)
	apiVersionKind := "v1;ConfigMap"
	namespacedName := "my-cm/default"

	expectedObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "my-cm",
				"namespace": "default",
			},
			"data": map[string]interface{}{
				"key": "value",
			},
		},
	}

	mockClient.On("Get",
		mock.Anything,
		types.NamespacedName{
			Name:      "my-cm",
			Namespace: "default",
		},
		mock.MatchedBy(func(obj *unstructured.Unstructured) bool {
			return obj.Object["apiVersion"] == "v1" &&
				obj.Object["kind"] == "ConfigMap"
		})).
		Run(func(args mock.Arguments) {
			obj := args.Get(2).(*unstructured.Unstructured)
			obj.Object = expectedObj.Object
		}).
		Return(nil)

	//when
	result, err := renderer.kubernetesGet(apiVersionKind, namespacedName)

	//then
	require.NoError(t, err)
	objMap, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "v1", objMap["apiVersion"])
	assert.Equal(t, "ConfigMap", objMap["kind"])
	assert.Equal(t, "my-cm", objMap["metadata"].(map[string]interface{})["name"])
	assert.Equal(t, "value", objMap["data"].(map[string]interface{})["key"])
	mockClient.AssertExpectations(t)
}

func TestTemplateRenderer_KubernetesGet_ErrWhenGetFails(t *testing.T) {
	//given
	mockClient := new(mockCache.MockCache)
	renderer := NewTemplateRenderer(mockClient)
	apiVersionKind := "v1;ConfigMap"
	namespacedName := "my-cm/default"
	expectedErr := errors.New("get error")

	mockClient.On("Get", mock.Anything, mock.Anything, mock.Anything).
		Return(expectedErr)

	//when
	_, err := renderer.kubernetesGet(apiVersionKind, namespacedName)

	//then
	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
	mockClient.AssertExpectations(t)
}

func TestTemplateRenderer_MinioPresignedGetObject_ErrWhenInvalidCredentials(t *testing.T) {
	//given
	mockClient := new(mockCache.MockCache)
	renderer := NewTemplateRenderer(mockClient)
	endpoint := "https://minio.example.com"
	credentials := "invalid" // Missing colon separator
	bucket := "test-bucket"
	path := "test-path"
	expiry := "1h"

	//when
	_, err := renderer.minioPresignedGetObject(endpoint, credentials, bucket, path, expiry)

	//then
	require.Error(t, err)
}

func TestTemplateRenderer_MinioPresignedGetObject_ErrWhenInvalidExpiry(t *testing.T) {
	//given
	mockClient := new(mockCache.MockCache)
	renderer := NewTemplateRenderer(mockClient)
	endpoint := "https://minio.example.com"
	credentials := "access:secret"
	bucket := "test-bucket"
	path := "test-path"
	expiry := "invalid" // Invalid duration format

	//when
	_, err := renderer.minioPresignedGetObject(endpoint, credentials, bucket, path, expiry)

	//then
	require.Error(t, err)
}

func TestTemplateRenderer_NetLookupIP(t *testing.T) {
	//given
	mockClient := new(mockCache.MockCache)
	renderer := NewTemplateRenderer(mockClient)
	hostname := "localhost" // Always resolvable

	//when
	ips := renderer.netLookupIP(hostname)

	//then
	assert.NotEmpty(t, ips, "Expected to resolve localhost to at least one IP")

	// Check that at least one of the IPs is a loopback address (127.0.0.1 or ::1)
	foundLoopback := false
	for _, ip := range ips {
		if ip.IsLoopback() {
			foundLoopback = true
			break
		}
	}
	assert.True(t, foundLoopback, "Expected to find a loopback address for localhost")
}

// Test with template function integration
func TestTemplateRenderer_TemplateWithKubernetesFunctions(t *testing.T) {
	//given
	mockClient := new(mockCache.MockCache)
	renderer := NewTemplateRenderer(mockClient)

	configMap := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": "default",
			},
			"data": map[string]interface{}{
				"key": "value",
			},
		},
	}

	mockClient.On("Get",
		mock.Anything,
		types.NamespacedName{
			Name:      "test-cm",
			Namespace: "default",
		},
		mock.MatchedBy(func(obj *unstructured.Unstructured) bool {
			return obj.Object["apiVersion"] == "v1" &&
				obj.Object["kind"] == "ConfigMap"
		})).
		Run(func(args mock.Arguments) {
			obj := args.Get(2).(*unstructured.Unstructured)
			obj.Object = configMap.Object
		}).
		Return(nil)

	tmplStr := `{{ with kubernetesGet "v1;ConfigMap" "test-cm/default" }}{{ index . "data" "key" }}{{ end }}`

	//when
	tmpl, err := renderer.Parse(tmplStr)
	require.NoError(t, err)

	result, err := renderer.Render(tmpl, nil)

	//then
	require.NoError(t, err)
	assert.Equal(t, "value", result)
	mockClient.AssertExpectations(t)
}
