package processor

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"text/template"
	"time"

	"github.com/nccloud/watchtower/pkg/apis/v1alpha2"
	"github.com/nccloud/watchtower/pkg/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func New(cache cache.Cache, client client.Client, watcher *v1alpha2.Watcher) (Processor, error) {
	var (
		destinationHttp *DestinationHTTP
		destinationType DestinationType
	)

	if watcher.Spec.Destination.Http != nil {
		urlTemplate, parseErr := template.New("").Parse(watcher.Spec.Destination.Http.URLTemplate)
		if parseErr != nil {
			return nil, parseErr
		}

		bodyTemplate, parseErr := template.New("").Parse(watcher.Spec.Destination.Http.BodyTemplate)
		if parseErr != nil {
			return nil, parseErr
		}

		headerTemplate, parseErr := template.New("").Parse(watcher.Spec.Destination.Http.HeaderTemplate)
		if parseErr != nil {
			return nil, parseErr
		}

		destinationType = DestinationTypeHTTP
		destinationHttp = &DestinationHTTP{
			client:         &http.Client{},
			urlTemplate:    urlTemplate,
			bodyTemplate:   bodyTemplate,
			headerTemplate: headerTemplate,
		}
	} else {
		return nil, errors.New("destination is not configured")
	}

	return &processor{
		watcher:          watcher,
		templateRenderer: common.NewTemplateRenderer(cache, client),
		destinationType:  destinationType,
		destinationHttp:  destinationHttp,
	}, nil
}

func (p *processor) Process(ctx context.Context, eventType string, oldObj, newObj *unstructured.Unstructured) error {
	data := p.getData(eventType, oldObj, newObj)

	if passed, filterErr := p.Filter(ctx, eventType, data); filterErr != nil || !passed {
		return filterErr
	}

	switch p.destinationType {
	case DestinationTypeHTTP:
		return p.SendHTTP(ctx, eventType, data)
	default:
		return errors.New("unsupported destination type")
	}
}

func (p *processor) Filter(_ context.Context, eventType string, data map[string]any) (bool, error) {
	if p.watcher.Spec.Filter.Create != nil && eventType == "create" {
		expression, evaluateErr := common.EvaluateCelExpressionBool(data, *p.watcher.Spec.Filter.Create)
		if evaluateErr != nil {
			return false, evaluateErr
		}

		return expression, nil
	} else if p.watcher.Spec.Filter.Update != nil && eventType == "update" {
		expression, evaluateErr := common.EvaluateCelExpressionBool(data, *p.watcher.Spec.Filter.Update)
		if evaluateErr != nil {
			return false, evaluateErr
		}

		return expression, nil
	} else if p.watcher.Spec.Filter.Delete != nil && eventType == "delete" {
		expression, evaluateErr := common.EvaluateCelExpressionBool(data, *p.watcher.Spec.Filter.Delete)
		if evaluateErr != nil {
			return false, evaluateErr
		}

		return expression, nil
	}

	return true, nil
}

func (p *processor) getData(eventType string, oldObj, newObj *unstructured.Unstructured) map[string]any {
	data := map[string]any{
		"eventType": eventType,
		"object":    newObj.Object,
		"now":       time.Now().Format(time.RFC3339),
	}

	switch eventType {
	case "create", "delete":
		break
	case "update":
		data["oldObject"] = oldObj.Object
		data["newObject"] = newObj.Object
	}

	return data
}

func (p *processor) SendHTTP(ctx context.Context, eventType string, data any) error {
	renderedBody, renderErr := p.templateRenderer.Render(p.destinationHttp.bodyTemplate, data)
	if renderErr != nil {
		return renderErr
	}

	renderedHeaders, renderErr := p.templateRenderer.Render(p.destinationHttp.headerTemplate, data)
	if renderErr != nil {
		return renderErr
	}

	renderedURL, renderErr := p.templateRenderer.Render(p.destinationHttp.urlTemplate, data)
	if renderErr != nil {
		return renderErr
	}

	request, requestErr := http.NewRequestWithContext(ctx,
		eventTypeMethodMap[eventType], renderedURL, bytes.NewReader([]byte(renderedBody)))
	if requestErr != nil {
		return requestErr
	}

	request.Header = common.StringToMap(renderedHeaders)

	doResult, doErr := p.destinationHttp.client.Do(request)
	if doErr != nil {
		return doErr
	}

	if doResult.StatusCode < 200 || doResult.StatusCode >= 300 {
		return errors.New("received non-2xx response code: " + doResult.Status)
	}

	return nil
}
