package pkg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
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

// deleteEventSendTimeout caps the synchronous send that the DeleteFunc predicate
// performs. Best-effort: on timeout the event is dropped (no retry, no queue).
const deleteEventSendTimeout = 30 * time.Second

// EventHeaderName is the HTTP header watchtower sets on every outbound request
// to signal the source event type to the destination.
const EventHeaderName = "X-Watchtower-Event"

// EventTypeUpsert is the EventHeaderName value used for the Reconcile path
// (create, update, and informer re-sync — watchtower does not distinguish
// these at Reconcile time, so it ships the current state under one label).
const EventTypeUpsert = "upsert"

// EventTypeDelete is the EventHeaderName value used for delete events.
const EventTypeDelete = "delete"

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

	if sendErr := r.Send(ctx, obj, EventTypeUpsert); sendErr != nil {
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

func (r *Controller) Send(ctx context.Context, obj *unstructured.Unstructured, eventType string) error {
	url, urlErr := common.TemplateExecuteForObject(r.watcher.Spec.Destination.Compiled.URLTemplate, obj)
	if urlErr != nil {
		return urlErr
	}

	body, bodyErr := common.TemplateExecuteForObject(r.watcher.Spec.Destination.Compiled.BodyTemplate, obj)
	if bodyErr != nil {
		return bodyErr
	}

	headers, headersErr := common.TemplateExecuteForObject(r.watcher.Spec.Destination.Compiled.HeaderTemplate, obj)
	if headersErr != nil {
		return headersErr
	}

	request, requestErr := http.NewRequestWithContext(ctx, r.watcher.Spec.Destination.Method,
		string(url), bytes.NewReader(body))
	if requestErr != nil {
		return requestErr
	}

	request.Header = common.StringToMap(string(headers))
	request.Header.Set(EventHeaderName, eventType)

	doRequest, doRequestErr := r.httpClient.Do(request)
	if doRequestErr != nil {
		return doRequestErr
	}

	defer func() {
		_ = doRequest.Body.Close()
	}()

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
		DeleteFunc: r.handleDeleteEvent,
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

	if r.watcher.Spec.Filter.Object.Custom != nil {
		result, executeErr := common.TemplateExecuteForObject(
			r.watcher.Spec.Filter.Object.Custom.Compiled.Template, obj)
		if executeErr != nil {
			return true, executeErr
		}

		if string(result) != r.watcher.Spec.Filter.Object.Custom.Result {
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

// handleDeleteEvent forwards a delete event to the configured Destination
// synchronously and always returns false so the event is never enqueued for
// Reconcile (Reconcile would Get→NotFound→no-op for a deleted object).
//
// Best-effort delivery: failures are logged but not retried, and events that
// occur while watchtower is restarting are lost. Opt in by setting
// `spec.filter.event.delete` on the Watcher CR.
func (r *Controller) handleDeleteEvent(deleteEvent event.DeleteEvent) bool {
	if r.watcher.Spec.Filter.Event.Delete == nil {
		return false
	}

	obj, ok := deleteEvent.Object.(*unstructured.Unstructured)
	if !ok {
		return false
	}

	if filtered, filterErr := r.FilterObject(obj); filterErr != nil || filtered {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), deleteEventSendTimeout)
	defer cancel()

	logger := log.FromContext(ctx).WithValues("watcher", r.watcher.GetName(),
		"name", obj.GetName(), "namespace", obj.GetNamespace())

	if sendErr := r.Send(ctx, obj, EventTypeDelete); sendErr != nil {
		logger.Error(sendErr, "delete-event forwarding failed (best-effort, not retried)")
	} else {
		logger.Info("delete-event forwarded")
	}

	return false
}
