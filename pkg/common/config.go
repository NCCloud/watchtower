package common

import (
	"os"
	"regexp"
	"text/template"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/util/yaml"
)

func NewConfig(path string) (*Config, error) {
	file, readFileErr := os.ReadFile(path)
	if readFileErr != nil {
		return nil, readFileErr
	}

	var config Config

	return &config, yaml.Unmarshal(file, &config)
}

type Config struct {
	// SyncPeriod determines the minimum frequency at which watched resources are re-processed.
	SyncPeriod *string `yaml:"syncPeriod"`
	// LeaderElection allows you to have multiple replicas to run and wait for start.
	LeaderElection bool `yaml:"leaderElection"`
	// Watchers are the configuration of all watching process.
	Watchers []Watcher `yaml:"watchers"`
}

type Watcher struct {
	// Source defines the source objects of the watching process.
	Source Source `yaml:"source"`
	// Filter helps you to filter objects.
	Filter Filter `yaml:"filter"`
	// Destination sets where the rendered objects will be sending.
	Destination Destination `yaml:"destination"`
}

type Source struct {
	// APIVersion is api version of the object like apps/v1, v1 etc.
	APIVersion string `yaml:"apiVersion"`
	// Kind is the kind of the object like Deployment, Secret, MyCustomResource etc.
	Kind string `yaml:"kind"`
	// Concurrency is how many concurrent workers will be working on processing this source.
	Concurrency *int `yaml:"concurrency"`
}

type Filter struct {
	// Event allows you to set event based filters
	Event EventFilter `yaml:"event"`
	// Object allows you to set object based filters
	Object ObjectFilter `yaml:"object"`
}

type EventFilter struct {
	// Create allows you to set create event based filters
	Create CreateEventFilter `yaml:"create"`
	// Update allows you to set update event based filters
	Update UpdateEventFilter `yaml:"update"`
}

type CreateEventFilter struct {
	// CreationTimeout sets what will be the maximum duration can past for the objects in create queue.
	// It also helps to minimize number of object that will be re-sent when application restarts.
	CreationTimeout *string `yaml:"creationTimeout"`
	Compiled        struct {
		CreationTimeout time.Duration
	}
}

type UpdateEventFilter struct {
	// GenerationChanged sets if generation should be different or same according to value. By default, It's not in use.
	GenerationChanged *bool `yaml:"generationChanged"`
}

type ObjectFilter struct {
	// Name is the regular expression to filter object Its name.
	Name *string `yaml:"name"`
	// Namespace is the regular expression to filter object Its namespace.
	Namespace *string `yaml:"namespace"`
	// Labels are the labels to filter object by labels.
	Labels *map[string]string `yaml:"labels"`
	// Annotations are the labels to filter object by annotation.
	Annotations *map[string]string `yaml:"annotations"`
	// Custom is the most advanced way of filtering object by their contents and multiple fields by templating.
	Custom   *ObjectFilterCustom `yaml:"custom"`
	Compiled struct {
		Name      *regexp.Regexp
		Namespace *regexp.Regexp
	}
}

type ObjectFilterCustom struct {
	// Template is the template that will be used to compare result with Result and filter accordingly.
	Template string `yaml:"template"`
	// Result is the result that will be used to compare with the result of the Template.
	Result   string `yaml:"result"`
	Compiled struct {
		Template *template.Template
	}
}

type Destination struct {
	// URLTemplate is the template field to set where will be the destination.
	URLTemplate string `yaml:"urlTemplate"`
	// BodyTemplate is the template field to set what will be sent the destination.
	BodyTemplate string `yaml:"bodyTemplate"`
	// Method is the HTTP method will be used while calling the destination endpoints.
	Method string `yaml:"method"`
	// Method is the HTTP headers will be used while calling the destination endpoints.
	Headers  map[string][]string `yaml:"headers"`
	Compiled struct {
		URLTemplate  *template.Template
		BodyTemplate *template.Template
	}
}

func (s *Source) NewObject() client.Object {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": s.APIVersion,
			"kind":       s.Kind,
		},
	}
}

func (w *Watcher) GetConcurrency() int {
	if w.Source.Concurrency != nil {
		return *w.Source.Concurrency
	}

	return 1
}

func (w *Watcher) Compile() {
	if w.Filter.Object.Custom != nil {
		w.Filter.Object.Custom.Compiled.Template = TemplateParse(w.Filter.Object.Custom.Template)
	}

	if w.Filter.Object.Name != nil {
		w.Filter.Object.Compiled.Name = regexp.MustCompile(*w.Filter.Object.Name)
	}

	if w.Filter.Object.Namespace != nil {
		w.Filter.Object.Compiled.Namespace = regexp.MustCompile(*w.Filter.Object.Namespace)
	}

	if w.Filter.Event.Create.CreationTimeout != nil {
		w.Filter.Event.Create.Compiled.CreationTimeout = MustReturn(
			time.ParseDuration(*w.Filter.Event.Create.CreationTimeout))
	}

	w.Destination.Compiled.URLTemplate = TemplateParse(w.Destination.URLTemplate)
	w.Destination.Compiled.BodyTemplate = TemplateParse(w.Destination.BodyTemplate)
}

func (c *Config) GetSyncPeriod() *time.Duration {
	if c.SyncPeriod != nil {
		return Pointer(MustReturn(time.ParseDuration(*c.SyncPeriod)))
	}

	return nil
}
