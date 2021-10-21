package controller

import (
	"context"
	"fmt"
	v1 "github.com/artifakt-io/demo-controller/pkg/apis/application/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

func (c *Controller) SyncHandler(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}
	app, err := c.ApplicationsLister.Applications(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("Application '%s' in work queue no longer exists", key))
			return nil
		}

		return err
	}

	deployment, err := c.DeploymentsLister.Deployments(app.Status.DeploymentRefNamespace).Get(app.Status.DeploymentRefName)
	if err != nil {
		if errors.IsNotFound(err) {
			deployment, err = c.Kubeclientset.AppsV1().Deployments(app.Namespace).Create(context.TODO(), NewDeployment(app), metav1.CreateOptions{})
		} else {
			return err
		}
	}


	if app.Spec.Replicas != nil && *app.Spec.Replicas != *deployment.Spec.Replicas {
		klog.V(4).Infof("Application %s replicas: %d, deployment replicas: %d", name, *app.Spec.Replicas, *deployment.Spec.Replicas)
		deployment, err = c.Kubeclientset.AppsV1().Deployments(app.Namespace).Update(context.TODO(), NewDeployment(app), metav1.UpdateOptions{})
	}

	container := mainContainerFromDeploymentTemplate(deployment)
	if app.Spec.ImageName != container.Image {
		klog.V(4).Infof("Application %s image: %s, deployment image: %d", name, app.Spec.ImageName, container.Image)
		deployment, err = c.Kubeclientset.AppsV1().Deployments(app.Namespace).Update(context.TODO(), NewDeployment(app), metav1.UpdateOptions{})
	}

	if err != nil {
		return err
	}

	err = c.updateApplicationStatus(app, deployment)
	if err != nil {
		return err
	}

	c.Recorder.Event(app, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}



func mainContainerFromDeploymentTemplate(deployment *appsv1.Deployment) corev1.Container {
	for _, d := range deployment.Spec.Template.Spec.Containers {
		if d.Name == "main" {
			return d
		}
	}
	return corev1.Container{}
}

func NewDeployment(app *v1.Application) *appsv1.Deployment {
	labels := map[string]string{
		"controller": app.Name,
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: app.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(app, v1.SchemeGroupVersion.WithKind("Application")),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: app.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "main",
							Image: app.Spec.ImageName,
						},
					},
				},
			},
		},
	}
}

func (c *Controller) updateApplicationStatus(app *v1.Application, deployment *appsv1.Deployment) error {

	if app.Status.DeploymentRefNamespace != deployment.Namespace ||
		app.Status.DeploymentRefName != deployment.Name {
		appCopy := app.DeepCopy()
		appCopy.Status.DeploymentRefNamespace = deployment.Namespace
		appCopy.Status.DeploymentRefName = deployment.Name
		_, err := c.ApplicationClientset.CloudestV1().Applications(appCopy.Namespace).UpdateStatus(context.TODO(), appCopy, metav1.UpdateOptions{})
		return err
	}
	return nil
}
