package pkg

import (
	"bytes"
	"context"
	"errors"
	mockHttp "github.com/nccloud/watchtower/mocks/net/http"
	"io"
	"net/http"
	"testing"

	mockCache "github.com/nccloud/watchtower/mocks/sigs.k8s.io/controller-runtime/pkg/cache"
	"github.com/nccloud/watchtower/pkg/apis/v1alpha2"
	"github.com/nccloud/watchtower/pkg/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestNewProcessor(t *testing.T) {
	//given
	mockCache := &mockCache.MockCache{}
	watcher := &v1alpha2.Watcher{}

	//when
	processor := NewProcessor(mockCache, watcher)

	//then
	assert.NotNil(t, processor)
	assert.IsType(t, &watcherProcessor{}, processor)
}

func TestFilter_EmptyExpression(t *testing.T) {
	//given
	mockCache := &mockCache.MockCache{}
	watcher := &v1alpha2.Watcher{
		Spec: v1alpha2.WatcherSpec{
			Filter: v1alpha2.Filter{
				Create: "",
				Update: "",
			},
		},
	}
	processor := NewProcessor(mockCache, watcher)
	ctx := context.Background()
	newObj := &unstructured.Unstructured{}

	//when
	result, filterErr := processor.Filter(ctx, nil, newObj)

	//then
	assert.True(t, result)
	assert.NoError(t, filterErr)
}

func TestFilter_CreateExpression(t *testing.T) {
	//given
	mockCache := &mockCache.MockCache{}
	watcher := &v1alpha2.Watcher{
		Spec: v1alpha2.WatcherSpec{
			Filter: v1alpha2.Filter{
				Create: "new.metadata.name == 'test'",
			},
		},
	}
	processor := NewProcessor(mockCache, watcher)
	ctx := context.Background()
	newObj := &unstructured.Unstructured{}
	newObj.SetName("test")

	//when
	result, filterErr := processor.Filter(ctx, nil, newObj)

	//then
	assert.True(t, result)
	assert.NoError(t, filterErr)
}

func TestFilter_UpdateExpression(t *testing.T) {
	//given
	mockCache := &mockCache.MockCache{}
	watcher := &v1alpha2.Watcher{
		Spec: v1alpha2.WatcherSpec{
			Filter: v1alpha2.Filter{
				Update: "old.metadata.name != new.metadata.name",
			},
		},
	}
	processor := NewProcessor(mockCache, watcher)
	ctx := context.Background()
	oldObj := &unstructured.Unstructured{}
	oldObj.SetName("old-name")
	oldObj.SetResourceVersion("1")

	newObj := &unstructured.Unstructured{}
	newObj.SetName("new-name")
	newObj.SetResourceVersion("2")

	//when
	result, filterErr := processor.Filter(ctx, oldObj, newObj)

	//then
	assert.True(t, result)
	assert.NoError(t, filterErr)
}

func TestFilter_InvalidExpression(t *testing.T) {
	//given
	mockCache := &mockCache.MockCache{}
	watcher := &v1alpha2.Watcher{
		Spec: v1alpha2.WatcherSpec{
			Filter: v1alpha2.Filter{
				Create: "invalid expression",
			},
		},
	}
	processor := NewProcessor(mockCache, watcher)
	ctx := context.Background()
	newObj := &unstructured.Unstructured{}

	//when
	result, filterErr := processor.Filter(ctx, nil, newObj)

	//then
	assert.False(t, result)
	assert.Error(t, filterErr)
}

func TestFilter_NonBooleanResult(t *testing.T) {
	//given
	mockCache := &mockCache.MockCache{}
	watcher := &v1alpha2.Watcher{
		Spec: v1alpha2.WatcherSpec{
			Filter: v1alpha2.Filter{
				Create: "42",
			},
		},
	}
	processor := NewProcessor(mockCache, watcher)
	ctx := context.Background()
	newObj := &unstructured.Unstructured{}

	//when
	result, filterErr := processor.Filter(ctx, nil, newObj)

	//then
	assert.False(t, result)
	assert.Error(t, filterErr)
	assert.Contains(t, filterErr.Error(), "result type is not bool")
}

func TestSend(t *testing.T) {
	//given
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

	processor := &watcherProcessor{
		client:           mockCacheClient,
		watcher:          watcher,
		httpClient:       &http.Client{Transport: mockRoundTripper},
		templateRenderer: common.NewTemplateRenderer(mockCacheClient),
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

	//when
	sendErr := processor.Send(ctx, obj)

	//then
	assert.NoError(t, sendErr)
	mockRoundTripper.AssertExpectations(t)
}

func TestSend_TemplateError(t *testing.T) {
	//given
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

	processor := &watcherProcessor{
		client:           mockCacheClient,
		watcher:          watcher,
		httpClient:       &http.Client{Transport: mockRoundTripper},
		templateRenderer: common.NewTemplateRenderer(mockCacheClient),
	}

	ctx := context.Background()

	//when
	sendErr := processor.Send(ctx, obj)

	//then
	assert.Error(t, sendErr)
	mockRoundTripper.AssertNotCalled(t, "RoundTrip", mock.Anything)
}

func TestSend_HttpError(t *testing.T) {
	//given
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

	processor := &watcherProcessor{
		client:           mockCacheClient,
		watcher:          watcher,
		httpClient:       &http.Client{Transport: mockRoundTripper},
		templateRenderer: common.NewTemplateRenderer(mockCacheClient),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(nil, httpError)

	ctx := context.Background()

	//when
	sendErr := processor.Send(ctx, obj)

	//then
	assert.Error(t, sendErr)
	assert.Contains(t, sendErr.Error(), httpError.Error())
	mockRoundTripper.AssertExpectations(t)
}

func TestSend_BadStatusCode(t *testing.T) {
	//given
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

	processor := &watcherProcessor{
		client:           mockCacheClient,
		watcher:          watcher,
		httpClient:       &http.Client{Transport: mockRoundTripper},
		templateRenderer: common.NewTemplateRenderer(mockCacheClient),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	ctx := context.Background()

	//when
	sendErr := processor.Send(ctx, obj)

	//then
	assert.Error(t, sendErr)
	assert.Contains(t, sendErr.Error(), "unexpected status code: 500")
	mockRoundTripper.AssertExpectations(t)
}
