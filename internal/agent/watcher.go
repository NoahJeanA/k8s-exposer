package agent

import (
	"context"
	"log/slog"
	"time"

	"github.com/noahjeana/k8s-exposer/pkg/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// ServiceWatcher watches Kubernetes services for exposure annotations
type ServiceWatcher struct {
	clientset kubernetes.Interface
	onChange  func([]types.ExposedService)
	logger    *slog.Logger
}

// NewServiceWatcher creates a new service watcher
func NewServiceWatcher(clientset kubernetes.Interface, onChange func([]types.ExposedService), logger *slog.Logger) *ServiceWatcher {
	return &ServiceWatcher{
		clientset: clientset,
		onChange:  onChange,
		logger:    logger,
	}
}

// Start starts watching services
func (w *ServiceWatcher) Start(ctx context.Context) error {
	w.logger.Info("Starting service watcher")

	// Create informer factory
	factory := informers.NewSharedInformerFactory(w.clientset, 30*time.Second)
	serviceInformer := factory.Core().V1().Services().Informer()

	// Add event handlers
	serviceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			w.logger.Debug("Service added")
			w.handleChange(ctx)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			w.logger.Debug("Service updated")
			w.handleChange(ctx)
		},
		DeleteFunc: func(obj interface{}) {
			w.logger.Debug("Service deleted")
			w.handleChange(ctx)
		},
	})

	// Start informer
	factory.Start(ctx.Done())

	// Wait for cache sync
	w.logger.Info("Waiting for informer cache to sync")
	if !cache.WaitForCacheSync(ctx.Done(), serviceInformer.HasSynced) {
		return ctx.Err()
	}
	w.logger.Info("Informer cache synced")

	// Initial discovery
	w.handleChange(ctx)

	// Keep running until context is canceled
	<-ctx.Done()
	return ctx.Err()
}

// handleChange handles service changes by discovering all exposed services and calling the onChange callback
func (w *ServiceWatcher) handleChange(ctx context.Context) {
	services, err := DiscoverServices(ctx, w.clientset, w.logger)
	if err != nil {
		w.logger.Error("Failed to discover services", "error", err)
		return
	}

	w.onChange(services)
}

// parseServiceAnnotations parses service annotations and returns an ExposedService
func (w *ServiceWatcher) parseServiceAnnotations(svc *corev1.Service) (*types.ExposedService, error) {
	return extractServiceInfo(svc)
}

// StartWithRetry starts the service watcher with retry logic
func (w *ServiceWatcher) StartWithRetry(ctx context.Context) error {
	return wait.PollUntilContextCancel(ctx, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		err := w.Start(ctx)
		if err != nil && err != context.Canceled {
			w.logger.Error("Service watcher failed, retrying", "error", err)
			return false, nil // Retry
		}
		return true, err
	})
}
