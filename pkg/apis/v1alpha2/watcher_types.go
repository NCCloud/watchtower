package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ValuesFromKind represents the possible sources for injecting values into an instance.
type ValuesFromKind string

const (
	// ValuesFromKindSecret specifies that values should be sourced from a Kubernetes Secret.
	ValuesFromKindSecret ValuesFromKind = "Secret"

	// ValuesFromKindConfigMap specifies that values should be sourced from a Kubernetes ConfigMap.
	ValuesFromKindConfigMap ValuesFromKind = "ConfigMap"
)

//+kubebuilder:object:root=true

type Watcher struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec WatcherSpec `json:"spec,omitempty" yaml:"spec,omitempty"`
}

//+kubebuilder:object:root=true

type WatcherList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Watcher `json:"items"`
}

type WatcherSpec struct {
	// Source defines the source objects of the watching process.
	Source Source `json:"source,omitempty" yaml:"source"`

	// Filter helps filter objects during the watching process.
	Filter Filter `json:"filter,omitempty" yaml:"filter"`

	// Destination sets where the rendered objects will be sent.
	Destination Destination `json:"destination,omitempty" yaml:"destination"`

	// ValuesFrom defines a list of sources (Secret/ConfigMap) to fetch values from.
	// They will be merged with the values provided in the Values field.
	ValuesFrom []ValuesFrom `json:"valuesFrom,omitempty"`
}

// ValuesFrom defines a reference to a Secret or ConfigMap to retrieve values.
type ValuesFrom struct {
	// Kind specifies whether the source is a Secret or ConfigMap.
	Kind ValuesFromKind `json:"kind"`

	// Name is the name of the Secret or ConfigMap.
	Name string `json:"name"`

	// Key is the specific key within the Secret or ConfigMap to retrieve the value from.
	Key string `json:"key"`
}

type Source struct {
	// APIVersion is api version of the object like apps/v1, v1 etc.
	APIVersion string `json:"apiVersion,omitempty" yaml:"apiVersion"`

	// Kind is the kind of the object like Deployment, Secret, MyCustomResource etc.
	Kind string `json:"kind,omitempty" yaml:"kind"`

	// Concurrency is how many concurrent workers will be working on processing this source.
	Concurrency *int `json:"concurrency,omitempty" yaml:"concurrency"`

	// Options allows you to set source specific options
	Hooks SourceHooks `json:"hooks,omitempty" yaml:"hooks"`
}

func (s *Source) NewInstance() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": s.APIVersion,
			"kind":       s.Kind,
		},
	}
}

func (s *Source) GetConcurrency() int {
	if s.Concurrency != nil {
		return *s.Concurrency
	}

	return 1
}

type SourceHooks struct {
	// OnSuccess options will be used when the source is successfully processed.
	OnSuccess SourceHooksOnSuccess `json:"onSuccess,omitempty" yaml:"onSuccess"`
}

type SourceHooksOnSuccess struct {
	// Delete will delete the object after it successfully processed.
	Delete bool `json:"delete,omitempty" yaml:"delete"`
}

type Filter struct {
	// Event allows you to set event based filters
	Create string `json:"create,omitempty" yaml:"create"`

	// Object allows you to set object based filters
	Update string `json:"update,omitempty" yaml:"update"`
}

type Destination struct {
	// URLTemplate is the template field to set where will be the destination.
	URLTemplate string `json:"urlTemplate,omitempty" yaml:"urlTemplate"`

	// BodyTemplate is the template field to set what will be sent the destination.
	BodyTemplate string `json:"bodyTemplate,omitempty" yaml:"bodyTemplate"`

	// HeaderTemplate is the template field to set what will be sent the destination.
	HeaderTemplate string `json:"headerTemplate,omitempty" yaml:"headerTemplate"`

	// Method is the HTTP method will be used while calling the destination endpoints.
	Method string `json:"method,omitempty" yaml:"method"`
}

func init() {
	SchemeBuilder.Register(&Watcher{}, &WatcherList{})
}
