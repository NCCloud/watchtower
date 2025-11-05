package manager

import (
	"log/slog"

	"github.com/puzpuzpuz/xsync/v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

type manager struct {
	logger   *slog.Logger
	cache    cache.Cache
	client   client.Client
	watchers *xsync.MapOf[string, *Watcher]
}
