package pkg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/nccloud/watchtower/pkg/apis/v1alpha1"
	"github.com/nccloud/watchtower/pkg/common"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var ErrUnexpectedStatusCode = errors.New("unexpected status code")

type Controller struct {
	client  client.Client
	watcher v1alpha1.WatcherSpec
}

func NewController(client client.Client, watcher v1alpha1.WatcherSpec) *Controller {
	return &Controller{
		client:  client,
		watcher: watcher,
	}
}

func (r *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		start  = time.Now()
		logger = log.FromContext(ctx)
		object = r.watcher.Source.NewObject()
	)

	if getErr := r.client.Get(ctx, req.NamespacedName, object); getErr != nil {
		return ctrl.Result{}, client.IgnoreNotFound(getErr)
	}

	if filtered, filterErr := r.Filter(object); filterErr != nil || filtered {
		return ctrl.Result{}, filterErr
	}

	logger.Info("Sending")

	if sendErr := r.Send(ctx, object); sendErr != nil {
		return ctrl.Result{}, sendErr
	}

	logger.Info(fmt.Sprintf("Finished in %s", time.Since(start)))

	return ctrl.Result{}, nil
}

func (r *Controller) Filter(obj client.Object) (bool, error) {
	if r.watcher.Filter.Object.Name != nil &&
		!r.watcher.Filter.Object.Compiled.Name.MatchString(obj.GetName()) {
		return true, nil
	}

	if r.watcher.Filter.Object.Namespace != nil &&
		!r.watcher.Filter.Object.Compiled.Namespace.MatchString(obj.GetNamespace()) {
		return true, nil
	}

	if r.watcher.Filter.Object.Labels != nil &&
		!common.MapContains(obj.GetLabels(), *r.watcher.Filter.Object.Labels) {
		return true, nil
	}

	if r.watcher.Filter.Object.Annotations != nil &&
		!common.MapContains(obj.GetAnnotations(), *r.watcher.Filter.Object.Annotations) {
		return true, nil
	}

	if r.watcher.Filter.Object.Custom != nil {
		result, executeErr := common.TemplateExecuteForObject(
			r.watcher.Filter.Object.Custom.Compiled.Template, obj)
		if executeErr != nil {
			return true, executeErr
		}

		if string(result) != r.watcher.Filter.Object.Custom.Result {
			return true, nil
		}
	}

	return false, nil
}

func (r *Controller) Send(ctx context.Context, obj client.Object) error {
	url, urlErr := common.TemplateExecuteForObject(r.watcher.Destination.Compiled.URLTemplate, obj)
	if urlErr != nil {
		return urlErr
	}

	body, bodyErr := common.TemplateExecuteForObject(r.watcher.Destination.Compiled.BodyTemplate, obj)
	if bodyErr != nil {
		return bodyErr
	}

	request, requestErr := http.NewRequestWithContext(ctx, r.watcher.Destination.Method,
		string(url), bytes.NewReader(body))
	if requestErr != nil {
		return requestErr
	}

	request.Header = r.watcher.Destination.Headers

	doRequest, doRequestErr := http.DefaultClient.Do(request)
	if doRequestErr != nil {
		return doRequestErr
	}
	defer doRequest.Body.Close()

	if doRequest.StatusCode < 200 || doRequest.StatusCode >= 300 {
		return fmt.Errorf("%w: %d", ErrUnexpectedStatusCode, doRequest.StatusCode)
	}

	return nil
}

func (r *Controller) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(event event.CreateEvent) bool {
				if r.watcher.Filter.Event.Create.CreationTimeout != nil {
					return event.Object.GetCreationTimestamp().
						Add(r.watcher.Filter.Event.Create.Compiled.CreationTimeout).After(time.Now())
				}

				return true
			},
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				if r.watcher.Filter.Event.Update.GenerationChanged != nil {
					if *r.watcher.Filter.Event.Update.GenerationChanged {
						return updateEvent.ObjectOld.GetGeneration() != updateEvent.ObjectNew.GetGeneration()
					}

					return updateEvent.ObjectOld.GetGeneration() == updateEvent.ObjectNew.GetGeneration()
				}

				return true
			},
		}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.watcher.GetConcurrency(),
		}).
		For(r.watcher.Source.NewObject()).
		Complete(r)
}
