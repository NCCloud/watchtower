package manager

import (
	"log/slog"

	"github.com/puzpuzpuz/xsync/v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	cache2 "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EventType string

const (
	EventTypeCreate EventType = "create"
	EventTypeUpdate EventType = "update"
	EventTypeDelete EventType = "delete"
)

type WorkqueueItem struct {
	eventType EventType
	newObject *unstructured.Unstructured
	oldObject *unstructured.Unstructured
}

type Watcher struct {
	workqueue    workqueue.TypedRateLimitingInterface[WorkqueueItem]
	registration cache2.ResourceEventHandlerRegistration
	stopCh       chan bool
}

type manager struct {
	logger   *slog.Logger
	cache    cache.Cache
	client   client.Client
	watchers *xsync.MapOf[string, *Watcher]
}
