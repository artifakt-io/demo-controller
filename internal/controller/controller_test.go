package controller_test

import (
	"github.com/artifakt-io/demo-controller/internal/controller"
	v1 "github.com/artifakt-io/demo-controller/pkg/apis/application/v1"
	informers "github.com/artifakt-io/demo-controller/pkg/client/informers/externalversions"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/client-go/tools/cache"
	"reflect"
	"testing"
	"time"

	apps "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"

	"github.com/artifakt-io/demo-controller/pkg/client/clientset/versioned/fake"
)

var (
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }
)

type fixture struct {
	t *testing.T

	client     *fake.Clientset
	kubeclient *k8sfake.Clientset
	// Objects to put in the store.
	applicationLister []*v1.Application
	deploymentLister  []*apps.Deployment
	// Actions expected to happen on the client.
	kubeactions []core.Action
	actions     []core.Action
	// Objects from here preloaded into NewSimpleFake.
	kubeobjects []runtime.Object
	objects     []runtime.Object
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{}
	f.t = t
	f.objects = []runtime.Object{}
	f.kubeobjects = []runtime.Object{}
	return f
}

func newApplication(name, imageName string, replicas *int32) *v1.Application {
	return &v1.Application{
		TypeMeta: metav1.TypeMeta{APIVersion: v1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: v1.ApplicationSpec{
			ImageName: imageName,
			Replicas:  replicas,
		},
	}
}

func (f *fixture) newController() (*controller.Controller, informers.SharedInformerFactory, kubeinformers.SharedInformerFactory) {
	f.client = fake.NewSimpleClientset(f.objects...)
	f.kubeclient = k8sfake.NewSimpleClientset(f.kubeobjects...)

	i := informers.NewSharedInformerFactory(f.client, noResyncPeriodFunc())
	k8sI := kubeinformers.NewSharedInformerFactory(f.kubeclient, noResyncPeriodFunc())

	c := controller.NewController(
		f.kubeclient,
		f.client,
		k8sI.Apps().V1().Deployments(),
		i.Cloudest().V1().Applications())

	c.ApplicationsSynced = alwaysReady
	c.DeploymentsSynced = alwaysReady
	c.Recorder = &record.FakeRecorder{}

	for _, a := range f.applicationLister {
		_ = i.Cloudest().V1().Applications().Informer().GetIndexer().Add(a)
	}

	for _, d := range f.deploymentLister {
		_ = k8sI.Apps().V1().Deployments().Informer().GetIndexer().Add(d)
	}

	return c, i, k8sI
}

func (f *fixture) run(appName string) {
	f.runController(appName, true, false)
}

func (f *fixture) runExpectError(appName string) {
	f.runController(appName, true, true)
}

func (f *fixture) runController(appName string, startInformers bool, expectError bool) {
	c, i, k8sI := f.newController()
	if startInformers {
		stopCh := make(chan struct{})
		defer close(stopCh)
		i.Start(stopCh)
		k8sI.Start(stopCh)
	}

	err := c.SyncHandler(appName)
	if !expectError && err != nil {
		f.t.Errorf("error syncing application: %v", err)
	} else if expectError && err == nil {
		f.t.Error("expected error syncing application, got nil")
	}

	actions := filterInformerActions(f.client.Actions())
	for i, action := range actions {
		if len(f.actions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(actions)-len(f.actions), actions[i:])
			break
		}

		expectedAction := f.actions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.actions) > len(actions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.actions)-len(actions), f.actions[len(actions):])
	}

	k8sActions := filterInformerActions(f.kubeclient.Actions())
	for i, action := range k8sActions {
		if len(f.kubeactions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(k8sActions)-len(f.kubeactions), k8sActions[i:])
			break
		}

		expectedAction := f.kubeactions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.kubeactions) > len(k8sActions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.kubeactions)-len(k8sActions), f.kubeactions[len(k8sActions):])
	}
}

// checkAction verifies that expected and actual actions are equal and both have
// same attached resources
func checkAction(expected, actual core.Action, t *testing.T) {
	if !(expected.Matches(actual.GetVerb(), actual.GetResource().Resource) && actual.GetSubresource() == expected.GetSubresource()) {
		t.Errorf("Expected\n\t%#v\ngot\n\t%#v", expected, actual)
		return
	}

	if reflect.TypeOf(actual) != reflect.TypeOf(expected) {
		t.Errorf("Action has wrong type. Expected: %t. Got: %t", expected, actual)
		return
	}

	switch a := actual.(type) {
	case core.CreateActionImpl:
		e, _ := expected.(core.CreateActionImpl)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expObject, object))
		}
	case core.UpdateActionImpl:
		e, _ := expected.(core.UpdateActionImpl)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expObject, object))
		}
	case core.PatchActionImpl:
		e, _ := expected.(core.PatchActionImpl)
		expPatch := e.GetPatch()
		patch := a.GetPatch()

		if !reflect.DeepEqual(expPatch, patch) {
			t.Errorf("Action %s %s has wrong patch\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expPatch, patch))
		}
	default:
		t.Errorf("Uncaptured Action %s %s, you should explicitly add a case to capture it",
			actual.GetVerb(), actual.GetResource().Resource)
	}
}

func filterInformerActions(actions []core.Action) []core.Action {
	ret := []core.Action{}
	for _, action := range actions {
		if len(action.GetNamespace()) == 0 &&
			(action.Matches("list", "applications") ||
				action.Matches("watch", "applications") ||
				action.Matches("list", "deployments") ||
				action.Matches("watch", "deployments")) {
			continue
		}
		ret = append(ret, action)
	}

	return ret
}

func (f *fixture) expectCreateDeploymentAction(d *apps.Deployment) {
	f.kubeactions = append(f.kubeactions, core.NewCreateAction(schema.GroupVersionResource{Resource: "deployments"}, d.Namespace, d))
}

func (f *fixture) expectUpdateDeploymentAction(d *apps.Deployment) {
	f.kubeactions = append(f.kubeactions, core.NewUpdateAction(schema.GroupVersionResource{Resource: "deployments"}, d.Namespace, d))
}

func (f *fixture) expectUpdateApplicationStatusAction(app *v1.Application) {
	action := core.NewUpdateAction(v1.SchemeGroupVersion.WithResource("applications"), app.Namespace, app)
	action.Subresource = "status"
	f.actions = append(f.actions, action)
}

func getKey(app *v1.Application, t *testing.T) string {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(app)
	if err != nil {
		t.Errorf("Unexpected error getting key for foo %v: %v", app.Name, err)
		return ""
	}
	return key
}

func int32Ptr(i int32) *int32 { return &i }

// Real test start from here

func TestCreatesDeployment(t *testing.T) {
	f := newFixture(t)
	app := newApplication("test", "nginx", int32Ptr(1))

	f.applicationLister = append(f.applicationLister, app)
	f.objects = append(f.objects, app)

	expDeployment := controller.NewDeployment(app)
	f.expectCreateDeploymentAction(expDeployment)

	expectApp := app.DeepCopy()
	expectApp.Status.DeploymentRefNamespace = expDeployment.Namespace
	expectApp.Status.DeploymentRefName = expDeployment.Name
	f.expectUpdateApplicationStatusAction(expectApp)

	f.run(getKey(app, t))
}

func TestDoNothing(t *testing.T) {
	f := newFixture(t)
	app := newApplication("test", "nginx", int32Ptr(1))
	deployment := controller.NewDeployment(app)
	app.Status.DeploymentRefNamespace = deployment.Namespace
	app.Status.DeploymentRefName = deployment.Name

	f.applicationLister = append(f.applicationLister, app)
	f.objects = append(f.objects, app)
	f.deploymentLister = append(f.deploymentLister, deployment)
	f.kubeobjects = append(f.kubeobjects, deployment)

	f.run(getKey(app, t))
}

func TestUpdateDeploymentReplicas(t *testing.T) {
	f := newFixture(t)
	app := newApplication("test", "nginx", int32Ptr(1))

	expDeployment := controller.NewDeployment(app)
	f.expectUpdateDeploymentAction(expDeployment)

	app.Status.DeploymentRefNamespace = expDeployment.Namespace
	app.Status.DeploymentRefName = expDeployment.Name

	deployment := controller.NewDeployment(app)
	deployment.Spec.Replicas = int32Ptr(2)

	f.applicationLister = append(f.applicationLister, app)
	f.objects = append(f.objects, app)
	f.deploymentLister = append(f.deploymentLister, deployment)
	f.kubeobjects = append(f.kubeobjects, deployment)

	f.run(getKey(app, t))
}

func TestUpdateDeploymentImageName(t *testing.T) {
	f := newFixture(t)
	app := newApplication("test", "nginx", int32Ptr(1))

	expDeployment := controller.NewDeployment(app)
	f.expectUpdateDeploymentAction(expDeployment)

	app.Status.DeploymentRefNamespace = expDeployment.Namespace
	app.Status.DeploymentRefName = expDeployment.Name

	deployment := controller.NewDeployment(app)
	deployment.Spec.Template.Spec.Containers[0].Image = "mysql"

	f.applicationLister = append(f.applicationLister, app)
	f.objects = append(f.objects, app)
	f.deploymentLister = append(f.deploymentLister, deployment)
	f.kubeobjects = append(f.kubeobjects, deployment)

	f.run(getKey(app, t))
}
