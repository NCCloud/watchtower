package pkg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/nccloud/watchtower/pkg/apis/v1alpha1"
	"github.com/nccloud/watchtower/pkg/common"
)

var ErrUnexpectedStatusCode = errors.New("unexpected status code")

type Controller struct {
	client     client.Client
	watcher    *v1alpha1.Watcher
	httpClient *http.Client
	deleted    sync.Map
}

func NewController(client client.Client, httpClient *http.Client, watcher *v1alpha1.Watcher) *Controller {
	return &Controller{
		client:     client,
		httpClient: httpClient,
		watcher:    watcher,
	}
}

func (r *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	logger := log.FromContext(ctx)

	obj := r.watcher.Spec.Source.NewObject()
	getErr := r.client.Get(ctx, req.NamespacedName, obj)
	isDelete := false

	switch {
	case getErr == nil:
		r.deleted.Delete(req.NamespacedName)
	case apierrors.IsNotFound(getErr):
		cached, ok := r.deleted.Load(req.NamespacedName)
		if !ok {
			return ctrl.Result{}, nil
		}

		obj = cached.(*unstructured.Unstructured)
		isDelete = true
	default:
		return ctrl.Result{}, getErr
	}

	logger.Info("Started")

	url, urlErr := common.TemplateExecuteForObject(r.watcher.Spec.Destination.Compiled.URLTemplate, obj)
	if urlErr != nil {
		return ctrl.Result{}, urlErr
	}

	body, bodyErr := common.TemplateExecuteForObject(r.watcher.Spec.Destination.Compiled.BodyTemplate, obj)
	if bodyErr != nil {
		return ctrl.Result{}, bodyErr
	}

	headers := make(http.Header, len(r.watcher.Spec.Destination.Compiled.Headers))

	for name, tmpl := range r.watcher.Spec.Destination.Compiled.Headers {
		rendered, renderErr := common.TemplateExecuteForObject(tmpl, obj)
		if renderErr != nil {
			return ctrl.Result{}, renderErr
		}

		headers.Add(name, string(rendered))
	}

	reqCtx, cancel := context.WithTimeout(ctx, r.watcher.Spec.Destination.Compiled.Timeout)
	defer cancel()

	request, requestErr := http.NewRequestWithContext(reqCtx, r.watcher.Spec.Destination.Compiled.Method,
		string(url), bytes.NewReader(body))
	if requestErr != nil {
		return ctrl.Result{}, requestErr
	}

	request.Header = headers

	response, doErr := r.httpClient.Do(request)
	if doErr != nil {
		return ctrl.Result{}, doErr
	}

	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ctrl.Result{}, fmt.Errorf("%w: %d", ErrUnexpectedStatusCode, response.StatusCode)
	}

	switch {
	case isDelete:
		r.deleted.Delete(req.NamespacedName)
	case r.watcher.Spec.Source.Options.OnSuccess.DeleteObject:
		if deleteErr := r.client.Delete(ctx, obj, client.PropagationPolicy("Background")); client.IgnoreNotFound(deleteErr) != nil {
			return ctrl.Result{}, deleteErr
		}
	}

	logger.Info("Finished", "duration", time.Since(start).String())

	return ctrl.Result{}, nil
}

func (r *Controller) FilterEvent() predicate.Funcs {
	logger := log.Log.WithName("filter").WithValues("watcher", r.watcher.Name)

	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			program := r.watcher.Spec.Filter.Compiled.Create
			if program == nil {
				return true
			}

			obj, ok := e.Object.(*unstructured.Unstructured)
			if !ok {
				return false
			}

			return common.EvalCELPredicate(logger, program, map[string]any{
				"object": obj.Object,
				"now":    time.Now(),
			})
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			program := r.watcher.Spec.Filter.Compiled.Update
			if program == nil {
				return true
			}

			oldObj, ok := e.ObjectOld.(*unstructured.Unstructured)
			if !ok {
				return false
			}

			newObj, ok := e.ObjectNew.(*unstructured.Unstructured)
			if !ok {
				return false
			}

			return common.EvalCELPredicate(logger, program, map[string]any{
				"object":    newObj.Object,
				"oldObject": oldObj.Object,
				"now":       time.Now(),
			})
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			program := r.watcher.Spec.Filter.Compiled.Delete
			if program == nil {
				return false
			}

			obj, ok := e.Object.(*unstructured.Unstructured)
			if !ok {
				return false
			}

			if !common.EvalCELPredicate(logger, program, map[string]any{
				"object": obj.Object,
				"now":    time.Now(),
			}) {
				return false
			}

			r.deleted.Store(types.NamespacedName{
				Name:      obj.GetName(),
				Namespace: obj.GetNamespace(),
			}, obj.DeepCopy())

			return true
		},
	}
}

func (r *Controller) SetupWithManager(mgr ctrl.Manager) error {
	concurrency := 1
	if r.watcher.Spec.Source.Concurrency != nil {
		concurrency = *r.watcher.Spec.Source.Concurrency
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named(r.watcher.GetName()).
		WithEventFilter(r.FilterEvent()).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: concurrency,
		}).
		For(r.watcher.Spec.Source.NewObject()).
		Complete(r)
}
