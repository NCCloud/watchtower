package models

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Flow is a pairing of the tap and the sink. That defines the specific tap will be sink into specified sink.
type Flow struct {
	// Name of the Tap
	// +required
	Tap string `yaml:"tap"`

	// Name of the Sink
	// +required
	Sink string `yaml:"sink"`
}

type CompiledFlow struct {
	Tap   CompiledTap    `yaml:"tap"`
	Sinks []CompiledSink `yaml:"sinks"`
}

func (c CompiledFlow) NewObjectInstance() client.Object {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": c.Tap.APIVersion,
			"kind":       c.Tap.Kind,
		},
	}
}
