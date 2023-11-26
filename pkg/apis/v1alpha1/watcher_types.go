package v1alpha1

import (
	"regexp"
	"text/template"
	"time"

	"github.com/nccloud/watchtower/pkg/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:object:root=true

type Watcher struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec WatcherSpec `json:"spec,omitempty"`
}

type WatcherSpec struct {
	// Source defines the source objects of the watching process.
	Source Source `json:"source"`
	// Filter helps you to filter objects.
	Filter Filter `json:"filter,omitempty"`
	// Destination sets where the rendered objects will be sending.
	Destination Destination `json:"destination"`
}

type Source struct {
	// APIVersion is api version of the object like apps/v1, v1 etc.
	APIVersion string `json:"apiVersion"`
	// Kind is the kind of the object like Deployment, Secret, MyCustomResource etc.
	Kind string `json:"kind"`
	// Concurrency is how many concurrent workers will be working on processing this source.
	Concurrency *int `json:"concurrency,omitempty"`
}

type Filter struct {
	// Event allows you to set event based filters
	Event EventFilter `json:"event,omitempty"`
	// Object allows you to set object based filters
	Object ObjectFilter `json:"object,omitempty"`
}

type EventFilter struct {
	// Create allows you to set create event based filters
	Create CreateEventFilter `json:"create,omitempty"`
	// Update allows you to set update event based filters
	Update UpdateEventFilter `json:"update,omitempty"`
}

type CreateEventFilter struct {
	// CreationTimeout sets what will be the maximum duration can past for the objects in create queue.
	// It also helps to minimize number of object that will be re-sent when application restarts.
	CreationTimeout *string `json:"creationTimeout,omitempty"`
	Compiled        struct {
		CreationTimeout time.Duration
	} `json:"-"`
}

type UpdateEventFilter struct {
	// GenerationChanged sets if generation should be different or same according to value. By default, It's not in use.
	GenerationChanged *bool `json:"generationChanged,omitempty"`
}

type ObjectFilter struct {
	// Name is the regular expression to filter object Its name.
	Name *string `json:"name,omitempty"`
	// Namespace is the regular expression to filter object Its namespace.
	Namespace *string `json:"namespace,omitempty"`
	// Labels are the labels to filter object by labels.
	Labels *map[string]string `json:"labels,omitempty"`
	// Annotations are the labels to filter object by annotation.
	Annotations *map[string]string `json:"annotations,omitempty"`
	// Custom is the most advanced way of filtering object by their contents and multiple fields by templating.
	Custom   *ObjectFilterCustom `json:"custom,omitempty"`
	Compiled struct {
		Name      *regexp.Regexp
		Namespace *regexp.Regexp
	} `json:"-"`
}

type ObjectFilterCustom struct {
	// Template is the template that will be used to compare result with Result and filter accordingly.
	Template string `json:"template"`
	// Result is the result that will be used to compare with the result of the Template.
	Result   string `json:"result"`
	Compiled struct {
		Template *template.Template
	} `json:"-"`
}

type Destination struct {
	// URLTemplate is the template field to set where will be the destination.
	URLTemplate string `json:"urlTemplate"`
	// BodyTemplate is the template field to set what will be sent the destination.
	BodyTemplate string `json:"bodyTemplate"`
	// Method is the HTTP method will be used while calling the destination endpoints.
	Method string `json:"method"`
	// Method is the HTTP headers will be used while calling the destination endpoints.
	Headers  map[string][]string `json:"headers,omitempty"`
	Compiled struct {
		URLTemplate  *template.Template
		BodyTemplate *template.Template
	} `json:"-"`
}

func (s *Source) NewObject() client.Object {
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

func (w *WatcherSpec) Compile() {
	if w.Filter.Object.Custom != nil {
		w.Filter.Object.Custom.Compiled.Template = common.TemplateParse(w.Filter.Object.Custom.Template)
	}

	if w.Filter.Object.Name != nil {
		w.Filter.Object.Compiled.Name = regexp.MustCompile(*w.Filter.Object.Name)
	}

	if w.Filter.Object.Namespace != nil {
		w.Filter.Object.Compiled.Namespace = regexp.MustCompile(*w.Filter.Object.Namespace)
	}

	if w.Filter.Event.Create.CreationTimeout != nil {
		w.Filter.Event.Create.Compiled.CreationTimeout = common.MustReturn(
			time.ParseDuration(*w.Filter.Event.Create.CreationTimeout))
	}

	w.Destination.Compiled.URLTemplate = common.TemplateParse(w.Destination.URLTemplate)
	w.Destination.Compiled.BodyTemplate = common.TemplateParse(w.Destination.BodyTemplate)
}

//+kubebuilder:object:root=true

type WatcherList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Watcher `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Watcher{}, &WatcherList{})
}
