package controller

import (
	"fmt"
	v1 "github.com/etiennecoutaud/demo-controller/pkg/apis/application/v1"
	clientset "github.com/etiennecoutaud/demo-controller/pkg/client/clientset/versioned"
	applicationscheme "github.com/etiennecoutaud/demo-controller/pkg/client/clientset/versioned/scheme"
	informers "github.com/etiennecoutaud/demo-controller/pkg/client/informers/externalversions/application/v1"
	listers "github.com/etiennecoutaud/demo-controller/pkg/client/listers/application/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"time"
)

const controllerAgentName = "demo-controller"

const (
	SuccessSynced         = "Synced"
	ErrResourceExists     = "ErrResourceExists"
	MessageResourceExists = "Resource %q already exists and is not managed by demo-controller"
	MessageResourceSynced = "Application synced successfully"
)

// Controller is the controller implementation for application resources
type Controller struct {
	Kubeclientset        kubernetes.Interface
	ApplicationClientset clientset.Interface

	DeploymentsLister appslisters.DeploymentLister
	DeploymentsSynced cache.InformerSynced

	ApplicationsLister listers.ApplicationLister
	ApplicationsSynced cache.InformerSynced

	Workqueue workqueue.RateLimitingInterface
	Recorder  record.EventRecorder
}

// NewController returns a new sample controller
func NewController(
	kubeclientset kubernetes.Interface,
	applicationClientset clientset.Interface,
	deploymentInformer appsinformers.DeploymentInformer,
	applicationInformer informers.ApplicationInformer) *Controller {

	utilruntime.Must(applicationscheme.AddToScheme(scheme.Scheme))
	klog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartStructuredLogging(0)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		Kubeclientset:        kubeclientset,
		ApplicationClientset: applicationClientset,
		DeploymentsLister:    deploymentInformer.Lister(),
		DeploymentsSynced:    deploymentInformer.Informer().HasSynced,
		ApplicationsLister:   applicationInformer.Lister(),
		ApplicationsSynced:   applicationInformer.Informer().HasSynced,
		Workqueue:            workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Applications"),
		Recorder:             recorder,
	}

	klog.Info("Setting up event handlers")
	// Set up an event handler for when Application resources change
	applicationInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueApplication,
		UpdateFunc: func(old, new interface{}) {
			newApp := new.(*v1.Application)
			oldApp := old.(*v1.Application)
			if newApp.ResourceVersion == oldApp.ResourceVersion {
				return
			}
			controller.enqueueApplication(new)
		},
	})

	deploymentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newDepl := new.(*appsv1.Deployment)
			oldDepl := old.(*appsv1.Deployment)
			if newDepl.ResourceVersion == oldDepl.ResourceVersion {
				return
			}
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})

	return controller
}

func (c *Controller) Run(workers int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.Workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	klog.Info("Starting Application controller")

	// Wait for the caches to be synced before starting workers
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.DeploymentsSynced, c.ApplicationsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")
	for i := 0; i < workers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the Workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.Workqueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.Workqueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			c.Workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in Workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// Application resource to be synced.
		if err := c.SyncHandler(key); err != nil {
			// Put the item back on the Workqueue to handle any transient errors.
			c.Workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.Workqueue.Forget(obj)
		klog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

func (c *Controller) enqueueApplication(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.Workqueue.Add(key)
}

func (c *Controller) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		klog.V(4).Infof("Recovered deleted object '%s' from tombstone", object.GetName())
	}
	klog.V(4).Infof("Processing object: %s", object.GetName())
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		// If this object is not owned by a Application, we should not do anything more
		// with it.
		if ownerRef.Kind != "Application" {
			return
		}

		app, err := c.ApplicationsLister.Applications(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			klog.V(4).Infof("ignoring orphaned object '%s' of Application '%s'", object.GetSelfLink(), ownerRef.Name)
			return
		}

		c.enqueueApplication(app)
		return
	}
}
