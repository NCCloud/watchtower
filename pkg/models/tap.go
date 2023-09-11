package models

import (
	"regexp"
	"text/template"

	"github.com/nccloud/watchtower/pkg/utils"
)

type Tap struct {
	// Name of the Tap
	// +required
	Name string `yaml:"name"`

	// Kind of the Kubernetes Resource
	// +required
	Kind string `yaml:"kind"`

	// APIVersion of the Kubernetes Resource
	// +required
	APIVersion string `yaml:"apiVersion"`

	// Filter that will be used while processing the Resource
	// +optional
	Filter *struct {
		// Filter for the resource names, regular expression can be used.
		// +optional
		Name string `yaml:"name"`

		// Filter for the resource namespaces, regular expression can be used.
		// +optional
		Namespace string `yaml:"namespace"`

		// Filter according to the comparison of the object fields.
		// +optional
		Object *struct {
			// Key of the object field. E.g. ".spec.a.b"
			// +optional
			Key string `yaml:"key"`

			// Value of the object.
			// +optional
			Value string `yaml:"value"`

			// Operator that will be used in while doing evaluation like = or !=.
			// +optional
			Operator string `yaml:"operator"`
		} `yaml:"object"`
	} `yaml:"filter"`
}

type CompiledTap struct {
	Name       string
	Filter     CompiledTapFilter
	Kind       string `yaml:"kind"`
	APIVersion string `yaml:"apiVersion"`
}

type CompiledTapFilter struct {
	Name      *regexp.Regexp
	Namespace *regexp.Regexp
	Object    *CompileTapFiltersObject
}

type CompileTapFiltersObject struct {
	Key     *template.Template
	Value   string
	Operand string
}

func (r Tap) Compile() CompiledTap {
	compiledTap := CompiledTap{
		Name:       r.Name,
		Kind:       r.Kind,
		APIVersion: r.APIVersion,
		Filter: CompiledTapFilter{
			Object: &CompileTapFiltersObject{},
		},
	}

	if r.Filter != nil {
		if r.Filter.Name != "" {
			compiledTap.Filter.Name = regexp.MustCompile(r.Filter.Name)
		}

		if r.Filter.Namespace != "" {
			compiledTap.Filter.Namespace = regexp.MustCompile(r.Filter.Namespace)
		}

		if r.Filter.Object != nil {
			compiledTap.Filter.Object.Key = utils.TemplateParse(r.Filter.Object.Key)
			compiledTap.Filter.Object.Value = r.Filter.Object.Value
			compiledTap.Filter.Object.Operand = r.Filter.Object.Operator
		}
	}

	return compiledTap
}
