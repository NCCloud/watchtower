package pkg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/nccloud/watchtower/pkg/apis/v1alpha1"
	"github.com/nccloud/watchtower/pkg/common"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var ErrUnexpectedStatusCode = errors.New("unexpected status code")

type Controller struct {
	client     client.Client
	watcher    *v1alpha1.Watcher
	httpClient *http.Client
}

func NewController(client client.Client, httpClient *http.Client, watcher *v1alpha1.Watcher) *Controller {
	return &Controller{
		client:     client,
		httpClient: httpClient,
		watcher:    watcher,
	}
}

func (r *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		start  = time.Now()
		logger = log.FromContext(ctx)
		obj    = r.watcher.Spec.Source.NewObject()
	)

	if getErr := r.client.Get(ctx, req.NamespacedName, obj); getErr != nil {
		return ctrl.Result{}, client.IgnoreNotFound(getErr)
	}

	if filtered, filterErr := r.FilterObject(obj); filterErr != nil || filtered {
		return ctrl.Result{}, filterErr
	}

	logger.Info("Started")

	if sendErr := r.Send(ctx, obj); sendErr != nil {
		return ctrl.Result{}, sendErr
	}

	if r.watcher.Spec.Source.Options.OnSuccess.DeleteObject {
		deleteErr := r.client.Delete(ctx, obj, client.PropagationPolicy("Background"))
		if client.IgnoreNotFound(deleteErr) != nil {
			return ctrl.Result{}, deleteErr
		}
	}

	logger.Info("Finished", "duration", time.Since(start).String())

	return ctrl.Result{}, nil
}

func (r *Controller) Send(ctx context.Context, obj *unstructured.Unstructured) error {
	url, urlErr := common.TemplateExecuteForObject(r.watcher.Spec.Destination.Compiled.URLTemplate, obj)
	if urlErr != nil {
		return urlErr
	}

	body, bodyErr := common.TemplateExecuteForObject(r.watcher.Spec.Destination.Compiled.BodyTemplate, obj)
	if bodyErr != nil {
		return bodyErr
	}

	request, requestErr := http.NewRequestWithContext(ctx, r.watcher.Spec.Destination.Method,
		string(url), bytes.NewReader(body))
	if requestErr != nil {
		return requestErr
	}

	request.Header = r.watcher.Spec.Destination.Headers

	doRequest, doRequestErr := r.httpClient.Do(request)
	if doRequestErr != nil {
		return doRequestErr
	}
	defer doRequest.Body.Close()

	if doRequest.StatusCode < 200 || doRequest.StatusCode >= 300 {
		return fmt.Errorf("%w: %d", ErrUnexpectedStatusCode, doRequest.StatusCode)
	}

	return nil
}

func (r *Controller) FilterEvent() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			if r.watcher.Spec.Filter.Event.Create.CreationTimeout != nil {
				return event.Object.GetCreationTimestamp().
					Add(r.watcher.Spec.Filter.Event.Create.Compiled.CreationTimeout).After(time.Now())
			}

			return true
		},
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			if r.watcher.Spec.Filter.Event.Update.GenerationChanged != nil {
				if *r.watcher.Spec.Filter.Event.Update.GenerationChanged {
					return updateEvent.ObjectOld.GetGeneration() != updateEvent.ObjectNew.GetGeneration()
				}

				return updateEvent.ObjectOld.GetGeneration() == updateEvent.ObjectNew.GetGeneration()
			}

			if r.watcher.Spec.Filter.Event.Update.ResourceVersionChanged != nil {
				if *r.watcher.Spec.Filter.Event.Update.ResourceVersionChanged {
					return updateEvent.ObjectOld.GetResourceVersion() != updateEvent.ObjectNew.GetResourceVersion()
				}

				return updateEvent.ObjectOld.GetResourceVersion() == updateEvent.ObjectNew.GetResourceVersion()
			}

			return true
		},
	}
}

func (r *Controller) FilterObject(obj *unstructured.Unstructured) (bool, error) {
	if r.watcher.Spec.Filter.Object.Name != nil &&
		!r.watcher.Spec.Filter.Object.Compiled.Name.MatchString(obj.GetName()) {
		return true, nil
	}

	if r.watcher.Spec.Filter.Object.Namespace != nil &&
		!r.watcher.Spec.Filter.Object.Compiled.Namespace.MatchString(obj.GetNamespace()) {
		return true, nil
	}

	if r.watcher.Spec.Filter.Object.Labels != nil &&
		!common.MapContains(obj.GetLabels(), *r.watcher.Spec.Filter.Object.Labels) {
		return true, nil
	}

	if r.watcher.Spec.Filter.Object.Annotations != nil &&
		!common.MapContains(obj.GetAnnotations(), *r.watcher.Spec.Filter.Object.Annotations) {
		return true, nil
	}

	if customFilter := r.watcher.Spec.Filter.Object.Custom; customFilter != nil {
		renderedResult, executeErr := common.TemplateExecuteForObject(
			customFilter.Compiled.Template, obj)
		if executeErr != nil {
			return true, executeErr
		}

		actual := string(renderedResult)
		if strings.TrimSpace(actual) != strings.TrimSpace(customFilter.Result) {
			return true, nil
		}
	}

	return false, nil
}

func (r *Controller) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(r.watcher.GetName()).
		WithEventFilter(r.FilterEvent()).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.watcher.Spec.GetConcurrency(),
		}).
		For(r.watcher.Spec.Source.NewObject()).
		Complete(r)
}
