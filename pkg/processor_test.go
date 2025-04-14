package pkg

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	mockHttp "github.com/nccloud/watchtower/mocks/net/http"

	mockCache "github.com/nccloud/watchtower/mocks/sigs.k8s.io/controller-runtime/pkg/cache"
	"github.com/nccloud/watchtower/pkg/apis/v1alpha2"
	"github.com/nccloud/watchtower/pkg/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestNewProcessor(t *testing.T) {
	// given
	mockCache := &mockCache.MockCache{}
	watcher := &v1alpha2.Watcher{}

	// when
	processorInstance := NewProcessor(mockCache, nil, watcher)

	// then
	assert.NotNil(t, processorInstance)
	assert.IsType(t, &processor{}, processorInstance)
}

func TestFilter_EmptyExpression(t *testing.T) {
	// given
	mockCache := &mockCache.MockCache{}
	watcher := &v1alpha2.Watcher{
		Spec: v1alpha2.WatcherSpec{
			Filter: v1alpha2.Filter{
				Create: "",
				Update: "",
			},
		},
	}
	processorInstance := NewProcessor(mockCache, nil, watcher)
	ctx := context.Background()
	newObj := &unstructured.Unstructured{}

	// when
	result, filterErr := processorInstance.Filter(ctx, nil, newObj)

	// then
	assert.True(t, result)
	assert.NoError(t, filterErr)
}

func TestFilter_CreateExpression(t *testing.T) {
	// given
	mockCache := &mockCache.MockCache{}
	watcher := &v1alpha2.Watcher{
		Spec: v1alpha2.WatcherSpec{
			Filter: v1alpha2.Filter{
				Create: "new.metadata.name == 'test'",
			},
		},
	}
	processorInstance := NewProcessor(mockCache, nil, watcher)
	ctx := context.Background()
	newObj := &unstructured.Unstructured{}
	newObj.SetName("test")

	// when
	result, filterErr := processorInstance.Filter(ctx, nil, newObj)

	// then
	assert.True(t, result)
	assert.NoError(t, filterErr)
}

func TestFilter_UpdateExpression(t *testing.T) {
	// given
	mockCache := &mockCache.MockCache{}
	watcher := &v1alpha2.Watcher{
		Spec: v1alpha2.WatcherSpec{
			Filter: v1alpha2.Filter{
				Update: "old.metadata.name != new.metadata.name",
			},
		},
	}
	processorInstance := NewProcessor(mockCache, nil, watcher)
	ctx := context.Background()
	oldObj := &unstructured.Unstructured{}
	oldObj.SetName("old-name")
	oldObj.SetResourceVersion("1")

	newObj := &unstructured.Unstructured{}
	newObj.SetName("new-name")
	newObj.SetResourceVersion("2")

	// when
	result, filterErr := processorInstance.Filter(ctx, oldObj, newObj)

	// then
	assert.True(t, result)
	assert.NoError(t, filterErr)
}

func TestFilter_InvalidExpression(t *testing.T) {
	// given
	mockCache := &mockCache.MockCache{}
	watcher := &v1alpha2.Watcher{
		Spec: v1alpha2.WatcherSpec{
			Filter: v1alpha2.Filter{
				Create: "invalid expression",
			},
		},
	}
	processorInstance := NewProcessor(mockCache, nil, watcher)
	ctx := context.Background()
	newObj := &unstructured.Unstructured{}

	// when
	result, filterErr := processorInstance.Filter(ctx, nil, newObj)

	// then
	assert.False(t, result)
	assert.Error(t, filterErr)
}

func TestFilter_NonBooleanResult(t *testing.T) {
	// given
	mockCache := &mockCache.MockCache{}
	watcher := &v1alpha2.Watcher{
		Spec: v1alpha2.WatcherSpec{
			Filter: v1alpha2.Filter{
				Create: "42",
			},
		},
	}
	processorInstance := NewProcessor(mockCache, nil, watcher)
	ctx := context.Background()
	newObj := &unstructured.Unstructured{}

	// when
	result, filterErr := processorInstance.Filter(ctx, nil, newObj)

	// then
	assert.False(t, result)
	assert.Error(t, filterErr)
	assert.Contains(t, filterErr.Error(), "result type is not bool")
}

func TestSend(t *testing.T) {
	// given
	mockCacheClient := &mockCache.MockCache{}
	mockRoundTripper := &mockHttp.MockRoundTripper{}
	mockResponse := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader([]byte("ok"))),
	}

	watcher := &v1alpha2.Watcher{
		Spec: v1alpha2.WatcherSpec{
			Destination: v1alpha2.Destination{
				URLTemplate:    "https://example.com/{{ .metadata.name }}",
				BodyTemplate:   `{"name": "{{ .metadata.name }}"}`,
				HeaderTemplate: "Content-Type: application/json",
				Method:         "POST",
			},
		},
	}

	obj := &unstructured.Unstructured{}
	obj.SetName("test-object")

	processor := &processor{
		kubeClient:       nil,
		watcher:          watcher,
		httpClient:       &http.Client{Transport: mockRoundTripper},
		templateRenderer: common.NewTemplateRenderer(mockCacheClient, nil),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Run(func(args mock.Arguments) {
		req := args.Get(0).(*http.Request)
		assert.Equal(t, "https://example.com/test-object", req.URL.String())
		assert.Equal(t, "POST", req.Method)
		assert.Equal(t, []string{"application/json"}, req.Header["Content-Type"])

		bodyBytes, _ := io.ReadAll(req.Body)
		assert.Equal(t, `{"name": "test-object"}`, string(bodyBytes))
	}).Return(mockResponse, nil)

	ctx := context.Background()

	// when
	sendErr := processor.Send(ctx, obj)

	// then
	assert.NoError(t, sendErr)
	mockRoundTripper.AssertExpectations(t)
}

func TestSend_TemplateError(t *testing.T) {
	// given
	mockCacheClient := &mockCache.MockCache{}
	mockRoundTripper := &mockHttp.MockRoundTripper{}

	watcher := &v1alpha2.Watcher{
		Spec: v1alpha2.WatcherSpec{
			Destination: v1alpha2.Destination{
				URLTemplate:    "https://example.com/{{ .invalidSyntax }",
				BodyTemplate:   `{"name": "{{ .metadata.name }}"}`,
				HeaderTemplate: "Content-Type: application/json",
				Method:         "POST",
			},
		},
	}

	obj := &unstructured.Unstructured{}
	obj.SetName("test-object")

	processor := &processor{
		kubeClient:       nil,
		watcher:          watcher,
		httpClient:       &http.Client{Transport: mockRoundTripper},
		templateRenderer: common.NewTemplateRenderer(mockCacheClient, nil),
	}

	ctx := context.Background()

	// when
	sendErr := processor.Send(ctx, obj)

	// then
	assert.Error(t, sendErr)
	mockRoundTripper.AssertNotCalled(t, "RoundTrip", mock.Anything)
}

func TestSend_HttpError(t *testing.T) {
	// given
	mockCacheClient := &mockCache.MockCache{}
	mockRoundTripper := &mockHttp.MockRoundTripper{}
	httpError := errors.New("http error")

	watcher := &v1alpha2.Watcher{
		Spec: v1alpha2.WatcherSpec{
			Destination: v1alpha2.Destination{
				URLTemplate:    "https://example.com/{{ .metadata.name }}",
				BodyTemplate:   `{"name": "{{ .metadata.name }}"}`,
				HeaderTemplate: "Content-Type: application/json",
				Method:         "POST",
			},
		},
	}

	obj := &unstructured.Unstructured{}
	obj.SetName("test-object")

	processor := &processor{
		kubeClient:       nil,
		watcher:          watcher,
		httpClient:       &http.Client{Transport: mockRoundTripper},
		templateRenderer: common.NewTemplateRenderer(mockCacheClient, nil),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(nil, httpError)

	ctx := context.Background()

	// when
	sendErr := processor.Send(ctx, obj)

	// then
	assert.Error(t, sendErr)
	assert.Contains(t, sendErr.Error(), httpError.Error())
	mockRoundTripper.AssertExpectations(t)
}

func TestSend_BadStatusCode(t *testing.T) {
	// given
	mockCacheClient := &mockCache.MockCache{}
	mockRoundTripper := &mockHttp.MockRoundTripper{}
	mockResponse := &http.Response{
		StatusCode: 500,
		Body:       io.NopCloser(bytes.NewReader([]byte("server error"))),
	}

	watcher := &v1alpha2.Watcher{
		Spec: v1alpha2.WatcherSpec{
			Destination: v1alpha2.Destination{
				URLTemplate:    "https://example.com/{{ .metadata.name }}",
				BodyTemplate:   `{"name": "{{ .metadata.name }}"}`,
				HeaderTemplate: "Content-Type: application/json",
				Method:         "POST",
			},
		},
	}

	obj := &unstructured.Unstructured{}
	obj.SetName("test-object")

	processor := &processor{
		kubeClient:       nil,
		watcher:          watcher,
		httpClient:       &http.Client{Transport: mockRoundTripper},
		templateRenderer: common.NewTemplateRenderer(mockCacheClient, nil),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	ctx := context.Background()

	// when
	sendErr := processor.Send(ctx, obj)

	// then
	assert.Error(t, sendErr)
	assert.Contains(t, sendErr.Error(), "unexpected status code: 500")
	mockRoundTripper.AssertExpectations(t)
}
