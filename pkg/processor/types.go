package processor

import (
	"context"
	"net/http"
	"text/template"

	"github.com/nccloud/watchtower/pkg/apis/v1alpha2"
	"github.com/nccloud/watchtower/pkg/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type DestinationType string

const (
	DestinationTypeHTTP DestinationType = "HTTP"
)

var eventTypeMethodMap = map[string]string{
	"create": "POST",
	"update": "PUT",
	"delete": "DELETE",
}

type DestinationHTTP struct {
	client         *http.Client
	urlTemplate    *template.Template
	bodyTemplate   *template.Template
	headerTemplate *template.Template
}

type Processor interface {
	Process(ctx context.Context, eventType string, oldObj, newObj *unstructured.Unstructured) error
}

type processor struct {
	watcher          *v1alpha2.Watcher
	templateRenderer *common.TemplateRenderer
	destinationType  DestinationType
	destinationHttp  *DestinationHTTP
}
