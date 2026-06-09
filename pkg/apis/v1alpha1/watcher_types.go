package v1alpha1

import (
	"fmt"
	"text/template"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/nccloud/watchtower/pkg/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	DefaultDestinationMethod  = "POST"
	DefaultDestinationTimeout = 30 * time.Second
)

//+kubebuilder:object:root=true
//+kubebuilder:resource:scope=Cluster

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
	// Filter is a set of CEL predicates, one per event type.
	Filter Filter `json:"filter,omitempty" yaml:"filter"`
	// Destination sets where the rendered objects will be sent.
	Destination Destination `json:"destination,omitempty" yaml:"destination"`
	// ValuesFrom allows merging variables from references.
	ValuesFrom ValuesFrom `json:"valuesFrom,omitempty"`
}

type Source struct {
	// APIVersion is api version of the object like apps/v1, v1 etc.
	APIVersion string `json:"apiVersion,omitempty" yaml:"apiVersion"`
	// Kind is the kind of the object like Deployment, Secret, MyCustomResource etc.
	Kind string `json:"kind,omitempty" yaml:"kind"`
	// Concurrency is how many concurrent workers will be working on processing this source.
	Concurrency *int `json:"concurrency,omitempty" yaml:"concurrency"`
	// Options allows you to set source specific options
	Options SourceOptions `json:"options,omitempty" yaml:"options"`
}

type SourceOptions struct {
	// OnSuccess options will be used when the source is successfully processed.
	OnSuccess OnSuccessSourceOptions `json:"onSuccess,omitempty" yaml:"onSuccess"`
}

type OnSuccessSourceOptions struct {
	// DeleteObject will delete the object after it successfully processed.
	// Has no effect on Delete events (the object is already gone).
	DeleteObject bool `json:"deleteObject,omitempty" yaml:"deleteObject"`
}

type Filter struct {
	// Create is a CEL boolean predicate evaluated on Create events.
	// Bindings: object (the new object), now (current timestamp).
	// Omit (empty string) to pass every Create event.
	Create string `json:"create,omitempty" yaml:"create"`
	// Update is a CEL boolean predicate evaluated on Update events.
	// Bindings: object (the new object), oldObject (the previous object), now (current timestamp).
	// Omit (empty string) to pass every Update event.
	Update string `json:"update,omitempty" yaml:"update"`
	// Delete is a CEL boolean predicate evaluated on Delete events.
	// Bindings: object (the last-known state of the deleted object), now (current timestamp).
	// Omit (empty string) to filter out every Delete event (the conservative default).
	Delete   string `json:"delete,omitempty" yaml:"delete"`
	Compiled struct {
		Create cel.Program
		Update cel.Program
		Delete cel.Program
	} `json:"-"`
}

type Destination struct {
	// URLTemplate is the template field to set where will be the destination.
	URLTemplate string `json:"urlTemplate,omitempty" yaml:"urlTemplate"`
	// BodyTemplate is the template field to set what will be sent the destination.
	BodyTemplate string `json:"bodyTemplate,omitempty" yaml:"bodyTemplate"`
	// Headers is a map of header name to a templated value.
	// Keys are sent verbatim; values are rendered as Go templates against the object.
	Headers map[string]string `json:"headers,omitempty" yaml:"headers"`
	// Method is the HTTP method used while calling the destination endpoints.
	// Defaults to POST when unset.
	Method string `json:"method,omitempty" yaml:"method"`
	// Timeout is the per-request HTTP timeout. Defaults to 30s when unset.
	Timeout  *string `json:"timeout,omitempty" yaml:"timeout"`
	Compiled struct {
		URLTemplate  *template.Template
		BodyTemplate *template.Template
		Headers      map[string]*template.Template
		Method       string
		Timeout      time.Duration
	} `json:"-"`
}

type ValuesFrom struct {
	// Secrets are the references that will be merged from.
	Secrets []SecretKeySelector `json:"secrets,omitempty"`
}

type SecretKeySelector struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
}

func (s *Source) NewObject() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": s.APIVersion,
			"kind":       s.Kind,
		},
	}
}

func (w *Watcher) Compile() (*Watcher, error) {
	out := w.DeepCopy()

	create, createErr := compileFilterExpression(out.Spec.Filter.Create, false)
	if createErr != nil {
		return nil, fmt.Errorf("compile filter.create: %w", createErr)
	}

	update, updateErr := compileFilterExpression(out.Spec.Filter.Update, true)
	if updateErr != nil {
		return nil, fmt.Errorf("compile filter.update: %w", updateErr)
	}

	deleteProg, deleteErr := compileFilterExpression(out.Spec.Filter.Delete, false)
	if deleteErr != nil {
		return nil, fmt.Errorf("compile filter.delete: %w", deleteErr)
	}

	out.Spec.Filter.Compiled.Create = create
	out.Spec.Filter.Compiled.Update = update
	out.Spec.Filter.Compiled.Delete = deleteProg

	out.Spec.Destination.Compiled.URLTemplate = common.TemplateParse(out.Spec.Destination.URLTemplate)
	out.Spec.Destination.Compiled.BodyTemplate = common.TemplateParse(out.Spec.Destination.BodyTemplate)
	out.Spec.Destination.Compiled.Headers = make(map[string]*template.Template,
		len(out.Spec.Destination.Headers))

	for k, v := range out.Spec.Destination.Headers {
		out.Spec.Destination.Compiled.Headers[k] = common.TemplateParse(v)
	}

	out.Spec.Destination.Compiled.Method = out.Spec.Destination.Method
	if out.Spec.Destination.Compiled.Method == "" {
		out.Spec.Destination.Compiled.Method = DefaultDestinationMethod
	}

	out.Spec.Destination.Compiled.Timeout = DefaultDestinationTimeout
	if out.Spec.Destination.Timeout != nil {
		timeout, timeoutErr := time.ParseDuration(*out.Spec.Destination.Timeout)
		if timeoutErr != nil {
			return nil, fmt.Errorf("parse destination.timeout %q: %w",
				*out.Spec.Destination.Timeout, timeoutErr)
		}

		out.Spec.Destination.Compiled.Timeout = timeout
	}

	return out, nil
}

func (w *Watcher) MustCompile() *Watcher {
	out, err := w.Compile()
	if err != nil {
		panic(err)
	}

	return out
}

func compileFilterExpression(expression string, withOldObject bool) (cel.Program, error) {
	if expression == "" {
		return nil, nil //nolint:nilnil
	}

	vars := []cel.EnvOption{
		cel.Variable("object", cel.DynType),
		cel.Variable("now", cel.TimestampType),
	}
	if withOldObject {
		vars = append(vars, cel.Variable("oldObject", cel.DynType))
	}

	env, envErr := cel.NewEnv(vars...)
	if envErr != nil {
		return nil, fmt.Errorf("cel env: %w", envErr)
	}

	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("expression %q: %w", expression, issues.Err())
	}

	program, programErr := env.Program(ast)
	if programErr != nil {
		return nil, fmt.Errorf("cel program: %w", programErr)
	}

	return program, nil
}

func init() {
	SchemeBuilder.Register(&Watcher{}, &WatcherList{})
}
