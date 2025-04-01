package main

import (
	"context"
	"log/slog"

	"github.com/nccloud/watchtower/pkg"
	"github.com/nccloud/watchtower/pkg/apis/v1alpha2"
	"github.com/nccloud/watchtower/pkg/common"
	"k8s.io/apimachinery/pkg/runtime"
	cache2 "k8s.io/client-go/tools/cache"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

func main() {
	ctx := context.Background()
	config := common.NewConfig()
	kubeConfig := ctrl.GetConfigOrDie()
	scheme := runtime.NewScheme()
	errChan := make(chan error)

	common.Must(v1alpha2.AddToScheme(scheme))

	cache := common.MustReturn(cache.New(kubeConfig, cache.Options{
		Scheme:     scheme,
		SyncPeriod: &config.SyncPeriod,
	}))

	manager := pkg.NewManager(cache)
	watcherInformer := common.MustReturn(cache.GetInformer(ctx, &v1alpha2.Watcher{}))

	common.MustReturn(watcherInformer.AddEventHandler(cache2.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			watcher := obj.(*v1alpha2.Watcher)
			slog.Info("Watcher added", "name", watcher.Name, "namespace", watcher.Namespace)

			manager.Add(ctx, watcher)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldWatcher := oldObj.(*v1alpha2.Watcher)
			newWatcher := newObj.(*v1alpha2.Watcher)

			if oldWatcher.GetGeneration() != newWatcher.GetGeneration() {
				slog.Info("Watcher updated", "name", newWatcher.Name, "namespace", newWatcher.Namespace)

				manager.Remove(ctx, oldWatcher)
				manager.Add(ctx, newWatcher)
			}
		},
		DeleteFunc: func(obj interface{}) {
			watcher := obj.(*v1alpha2.Watcher)
			slog.Info("Watcher deleted", "name", watcher.Name, "namespace", watcher.Namespace)

			manager.Remove(ctx, obj.(*v1alpha2.Watcher))
		},
	}))

	go func() {
		slog.Info("Starting Watchtower")
		errChan <- cache.Start(ctx)
	}()

	slog.Info("Waiting for cache to sync...")

	if sync := cache.WaitForCacheSync(ctx); !sync {
		panic("cache sync failed")
	}

	slog.Info("Cache synced")

	if err := <-errChan; err != nil {
		panic(err)
	}
}
