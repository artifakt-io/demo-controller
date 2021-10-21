package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Application is a specification for a application resource
type Application struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationSpec   `json:"spec"`
	Status ApplicationStatus `json:"status"`
}

// ApplicationSpec is the spec for a application resource
type ApplicationSpec struct {
	ImageName string `json:"imageName"`
	Replicas  *int32 `json:"replicas"`
}

// ApplicationStatus is the status for a Foo resource
type ApplicationStatus struct {
	DeploymentRefNamespace string `json:"deploymentRefNamespace,omitempty"`
	DeploymentRefName      string `json:"deploymentRefName,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ApplicationList is a list of Application resources
type ApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Application `json:"items"`
}
