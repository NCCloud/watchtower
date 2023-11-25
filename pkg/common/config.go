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
	SyncPeriod           *string   `yaml:"syncPeriod"`
	EnableLeaderElection bool      `yaml:"enableLeaderElection"`
	Watchers             []Watcher `yaml:"watchers"`
}

type Watcher struct {
	Source      Source      `yaml:"source"`
	Filter      Filter      `yaml:"filter"`
	Destination Destination `yaml:"destination"`
}

type Source struct {
	APIVersion  string `yaml:"apiVersion"`
	Kind        string `yaml:"kind"`
	Concurrency *int   `yaml:"concurrency"`
}

type Filter struct {
	Event  EventFilter  `yaml:"event"`
	Object ObjectFilter `yaml:"object"`
}

type EventFilter struct {
	Create CreateEventFilter `yaml:"create"`
	Update UpdateEventFilter `yaml:"update"`
}

type CreateEventFilter struct {
	CreationTimeout *string `yaml:"creationTimeout"`
	Compiled        struct {
		CreationTimeout time.Duration
	}
}

type UpdateEventFilter struct {
	GenerationChanged *bool `yaml:"generationChanged"`
}

type ObjectFilter struct {
	Name        *string             `yaml:"name"`
	Namespace   *string             `yaml:"namespace"`
	Labels      *map[string]string  `yaml:"labels"`
	Annotations *map[string]string  `yaml:"annotations"`
	Custom      *ObjectFilterCustom `yaml:"custom"`
	Compiled    struct {
		Name      *regexp.Regexp
		Namespace *regexp.Regexp
	}
}

type ObjectFilterCustom struct {
	Template string `yaml:"template"`
	Result   string `yaml:"result"`
	Compiled struct {
		Template *template.Template
	}
}

type Destination struct {
	URLTemplate  string              `yaml:"urlTemplate"`
	BodyTemplate string              `yaml:"bodyTemplate"`
	Method       string              `yaml:"method"`
	Headers      map[string][]string `yaml:"headers"`
	Compiled     struct {
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
