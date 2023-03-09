package models

import (
	"text/template"

	"github.com/nccloud/watchtower/pkg/utils"
)

// Sink defines the endpoint the resources will be informed to.
type Sink struct {
	// Name of the Sink
	// +required
	Name string `yaml:"name"`

	// Http Method that will be used in sink like POST, GET, PUT etc.
	// +required
	Method string `yaml:"method"`

	// Template of the Url that needs to be crafted and request will be sent.
	// Object fields can be used in the template.
	// +required
	URLTemplate string `yaml:"urlTemplate"`

	// Template of the Body that needs to be crafted and sent.
	// Object fields can be used in the template.
	// +required
	BodyTemplate string `yaml:"bodyTemplate"`

	// Key-Value pairs that will be sent as header in the request.
	// +required
	Header map[string]string `yaml:"header"`
}

type CompiledSink struct {
	Name         string
	Method       string
	URLTemplate  *template.Template
	BodyTemplate *template.Template
	Header       map[string][]string
}

func (c *Sink) Compile() CompiledSink {
	header := make(map[string][]string)
	for key, value := range c.Header {
		header[key] = []string{value}
	}

	return CompiledSink{
		Name:         c.Name,
		Method:       c.Method,
		Header:       header,
		URLTemplate:  utils.TemplateParse(c.URLTemplate),
		BodyTemplate: utils.TemplateParse(c.BodyTemplate),
	}
}
