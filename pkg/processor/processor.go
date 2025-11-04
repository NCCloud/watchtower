package processor

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/nccloud/watchtower/pkg/apis/v1alpha2"
	"github.com/nccloud/watchtower/pkg/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func New(client client.Client, watcher *v1alpha2.Watcher) (Processor, error) {
	var (
		destinationHttp  *DestinationHTTP
		destinationType  DestinationType
		templateRenderer = common.NewTemplateRenderer(client)
	)

	if watcher.Spec.Destination.Http != nil {
		urlTemplate, parseErr := templateRenderer.Parse(watcher.Spec.Destination.Http.URLTemplate)
		if parseErr != nil {
			return nil, parseErr
		}

		bodyTemplate, parseErr := templateRenderer.Parse(watcher.Spec.Destination.Http.BodyTemplate)
		if parseErr != nil {
			return nil, parseErr
		}

		headerTemplate, parseErr := templateRenderer.Parse(watcher.Spec.Destination.Http.HeaderTemplate)
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
		client:           client,
		watcher:          watcher,
		templateRenderer: common.NewTemplateRenderer(client),
		destinationType:  destinationType,
		destinationHttp:  destinationHttp,
	}, nil
}

func (p *processor) Process(ctx context.Context, eventType string, oldObj, newObj *unstructured.Unstructured) error {
	object := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": newObj.GetAPIVersion(),
			"kind":       newObj.GetKind(),
			"metadata": map[string]interface{}{
				"name":      newObj.GetName(),
				"namespace": newObj.GetNamespace(),
			},
		},
	}

	if getErr := p.client.Get(ctx,
		client.ObjectKeyFromObject(object), object); client.IgnoreNotFound(getErr) != nil {
		return getErr
	}

	if passed, filterErr := p.FilterEvent(ctx, eventType, oldObj, newObj); filterErr != nil || !passed {
		return filterErr
	}

	if preflightErr := p.PreFlight(ctx, object); preflightErr != nil {
		return preflightErr
	}

	if flightErr := p.Flight(ctx, eventType, object); flightErr != nil {
		return flightErr
	}

	if postflightErr := p.PostFlight(ctx, object); postflightErr != nil {
		return postflightErr
	}

	return nil
}

func (p *processor) FilterEvent(ctx context.Context, eventType string,
	oldObject, newObject *unstructured.Unstructured) (bool, error) {
	data := map[string]any{
		"newObject": newObject.Object,
		"object":    newObject.Object,
		"now":       time.Now().UTC().Format(time.RFC3339),
	}

	if oldObject != nil {
		data["oldObject"] = oldObject.Object
	}

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

func (p *processor) Flight(ctx context.Context, eventType string, object *unstructured.Unstructured) error {
	data := map[string]any{
		"object": object.Object,
		"now":    time.Now().UTC().Format(time.RFC3339),
	}

	switch p.destinationType {
	case DestinationTypeHTTP:
		return p.sendHTTP(ctx, eventType, data)
	}

	return errors.New("unsupported destination type")
}

func (p *processor) PreFlight(ctx context.Context, latestObj *unstructured.Unstructured) error {
	if p.watcher.Spec.Source.HasLifecyclePolicy(v1alpha2.LifecyclePolicyUseFinalizer) &&
		!controllerutil.ContainsFinalizer(latestObj, v1alpha2.Finalizer) &&
		latestObj.GetDeletionTimestamp() == nil {

		controllerutil.AddFinalizer(latestObj, v1alpha2.Finalizer)

		if patchErr := p.client.Update(ctx, latestObj); patchErr != nil {
			return patchErr
		}
	}

	return nil
}

func (p *processor) PostFlight(ctx context.Context, latestObj *unstructured.Unstructured) error {
	if latestObj.GetDeletionTimestamp() != nil && controllerutil.ContainsFinalizer(latestObj, v1alpha2.Finalizer) {
		controllerutil.RemoveFinalizer(latestObj, v1alpha2.Finalizer)

		if patchErr := p.client.Update(ctx, latestObj); client.IgnoreNotFound(patchErr) != nil {
			return patchErr
		}
	}

	if p.watcher.Spec.Source.HasLifecyclePolicy(v1alpha2.LifecyclePolicyDeleteOnSuccess) {
		if deleteErr := p.client.Delete(ctx, latestObj); client.IgnoreNotFound(deleteErr) != nil {
			return deleteErr
		}
	}

	return nil
}

func (p *processor) sendHTTP(ctx context.Context, eventType string, data any) error {
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
