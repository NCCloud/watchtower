package pkg

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/nccloud/watchtower/pkg/models"
	"github.com/nccloud/watchtower/pkg/utils"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type Controller struct {
	mgrClient client.Client
	flow      models.CompiledFlow
}

func NewController(mgrClient client.Client, flow models.CompiledFlow) *Controller {
	return &Controller{
		mgrClient: mgrClient,
		flow:      flow,
	}
}

func (r *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	obj := r.flow.NewObjectInstance()

	getErr := r.mgrClient.Get(ctx, req.NamespacedName, obj)
	if getErr != nil {
		return ctrl.Result{}, client.IgnoreNotFound(getErr)
	}

	for _, sink := range r.flow.Sinks {
		logger.Info(fmt.Sprintf("Draining %s to %s", r.flow.Tap.Name, sink.Name))

		drainErr := r.Drain(ctx, obj, sink)
		if drainErr != nil {
			return ctrl.Result{}, drainErr
		}
	}

	return ctrl.Result{}, nil
}

func (r *Controller) Drain(ctx context.Context, obj client.Object, sink models.CompiledSink) error {
	url, urlErr := utils.TemplateExecuteForObject(sink.URLTemplate, obj)
	if urlErr != nil {
		return urlErr
	}

	body, bodyErr := utils.TemplateExecuteForObject(sink.BodyTemplate, obj)
	if bodyErr != nil {
		return bodyErr
	}

	request, requestErr := http.NewRequestWithContext(ctx, sink.Method, string(url), bytes.NewReader(body))
	if requestErr != nil {
		return requestErr
	}

	request.Header = sink.Header

	doRequest, doRequestErr := http.DefaultClient.Do(request)
	if doRequestErr != nil {
		return doRequestErr
	}
	defer doRequest.Body.Close()

	if doRequest.StatusCode < 200 || doRequest.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", doRequest.StatusCode)
	}

	return nil
}

func (r *Controller) Filter(object client.Object) bool {
	if r.flow.Tap.Filter.Name != nil && !r.flow.Tap.Filter.Name.MatchString(object.GetName()) {
		return false
	}

	if r.flow.Tap.Filter.Namespace != nil && !r.flow.Tap.Filter.Namespace.MatchString(object.GetNamespace()) {
		return false
	}

	if r.flow.Tap.Filter.Object.Key != nil {
		objectData, objectDataErr := utils.TemplateExecuteForObject(r.flow.Tap.Filter.Object.Key, object)
		if objectDataErr != nil {
			return false
		}

		if r.flow.Tap.Filter.Object.Operand == "==" && string(objectData) != r.flow.Tap.Filter.Object.Value {
			return false
		} else if r.flow.Tap.Filter.Object.Operand == "!=" && string(objectData) == r.flow.Tap.Filter.Object.Value {
			return false
		}
	}

	return true
}

func (r *Controller) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(r.flow.NewObjectInstance()).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(createEvent event.CreateEvent) bool {
				return r.Filter(createEvent.Object)
			},
			DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
				return false
			},
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				return r.Filter(updateEvent.ObjectNew)
			},
			GenericFunc: func(genericEvent event.GenericEvent) bool {
				return r.Filter(genericEvent.Object)
			},
		}).Complete(r)
}
