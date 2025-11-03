package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/nccloud/watchtower/pkg/apis/v1alpha2"
	"github.com/nccloud/watchtower/pkg/common"
	"github.com/nccloud/watchtower/pkg/manager"
	"k8s.io/apimachinery/pkg/runtime"
	cache2 "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/lmittmann/tint"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

func main() {
	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{
		TimeFormat: time.RFC3339,
	}))
	logger.Info("Watchtower starting")

	ctx := context.Background()
	config := common.NewConfig()
	kubeConfig := ctrl.GetConfigOrDie()
	scheme := runtime.NewScheme()
	errChan := make(chan error)

	common.Must(clientgoscheme.AddToScheme(scheme))
	common.Must(v1alpha2.AddToScheme(scheme))

	cache := common.MustReturn(cache.New(kubeConfig, cache.Options{
		Scheme:     scheme,
		SyncPeriod: &config.SyncPeriod,
	}))

	client := common.MustReturn(client.New(kubeConfig, client.Options{
		Scheme: scheme,
		Cache:  &client.CacheOptions{Reader: cache},
	}))

	manager := manager.New(logger, cache, client)

	go func() {
		errChan <- cache.Start(ctx)
	}()

	logger.Info("Waiting for cache sync.")

	if sync := cache.WaitForCacheSync(ctx); !sync {
		panic("cache sync failed")
	}

	logger.Info("Cache synced.")

	common.MustReturn(common.MustReturn(cache.GetInformer(ctx, &v1alpha2.Watcher{})).
		AddEventHandler(cache2.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				watcher := obj.(*v1alpha2.Watcher)
				logger.Info("Watcher added.", "name", watcher.Name, "namespace", watcher.Namespace)

				manager.Add(ctx, watcher)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				watcher := newObj.(*v1alpha2.Watcher)
				logger.Info("Watcher updated.", "name", watcher.Name, "namespace", watcher.Namespace)

				manager.Update(ctx, watcher)
			},
			DeleteFunc: func(obj interface{}) {
				watcher := obj.(*v1alpha2.Watcher)
				logger.Info("Watcher deleted.", "name", watcher.Name, "namespace", watcher.Namespace)

				manager.Delete(ctx, obj.(*v1alpha2.Watcher))
			},
		}))

	logger.Info("Watcher informer started")

	if err := <-errChan; err != nil {
		panic(err)
	}
}
