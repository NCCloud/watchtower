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

	Spec WatcherSpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

type WatcherList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Watcher `json:"items"`
}

type WatcherSpec struct {
	// Source defines the source objects of the watching process.
	Source Source `json:"source,omitempty" yaml:"source"`
	// Filter helps filter objects during the watching process.
	Filter Filter `json:"filter,omitempty" yaml:"filter"`
	// Destination sets where the rendered objects will be sent.
	Destination Destination `json:"destination,omitempty" yaml:"destination"`
	// ValuesFrom allows merging variables from references.
	ValuesFrom []ValuesFrom `json:"valuesFrom,omitempty"`
}

type Source struct {
	// APIVersion is api version of the object like apps/v1, v1 etc.
	APIVersion string `json:"apiVersion,omitempty" yaml:"apiVersion"`
	// Kind is the kind of the object like Deployment, Secret, MyCustomResource etc.
	Kind string `json:"kind,omitempty" yaml:"kind"`
	// Concurrency is how many concurrent workers will be working on processing this source.
	Concurrency *int `json:"concurrency,omitempty" yaml:"concurrency"`
	// Options allows you to set source specific options
	Policies []string `json:"policies,omitempty" yaml:"policies"`
}

func (s *Source) GetConcurrency() int {
	if s.Concurrency != nil {
		return *s.Concurrency
	}

	return 1
}

func (s *Source) NewInstance() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": s.APIVersion,
			"kind":       s.Kind,
		},
	}
}

type Filter struct {
	// CreateFilter allows you to set create event based filters
	Create *string `json:"create,omitempty" yaml:"create"`
	// UpdateFilter allows you to set update event based filters
	Update *string `json:"update,omitempty" yaml:"update"`
	// DeleteFilter allows you to set delete event based filters
	Delete *string `json:"delete,omitempty" yaml:"delete"`
}

type Destination struct {
	// Http is the HTTP destination configuration.
	Http *HTTPDestination `json:"http,omitempty" yaml:"http"`
}

type HTTPDestination struct {
	// URLTemplate is the template field to set where will be the destination.
	URLTemplate string `json:"urlTemplate,omitempty" yaml:"urlTemplate"`
	// BodyTemplate is the template field to set what will be sent the destination.
	BodyTemplate string `json:"bodyTemplate,omitempty" yaml:"bodyTemplate"`
	// HeaderTemplate is the template field to set what will be sent the destination.
	HeaderTemplate string `json:"headerTemplate,omitempty" yaml:"headerTemplate"`
}

type ValuesFrom struct {
	// Kind specifies whether the source is a Secret or ConfigMap.
	Kind ValuesFromKind `json:"kind"`

	// Name is the name of the Secret or ConfigMap.
	Name string `json:"name"`

	// Key is the specific key within the Secret or ConfigMap to retrieve the value from.
	Key string `json:"key"`
}

func (s *Source) NewObject() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": s.APIVersion,
			"kind":       s.Kind,
		},
	}
}

func (w *WatcherSpec) GetConcurrency() int {
	if w.Source.Concurrency != nil {
		return *w.Source.Concurrency
	}

	return 1
}

func init() {
	SchemeBuilder.Register(&Watcher{}, &WatcherList{})
}
