package models

import (
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/util/yaml"
)

func NewConfig(path string) (*Config, error) {
	file, readFileErr := os.ReadFile(path)
	if readFileErr != nil {
		return nil, readFileErr
	}

	var cfg Config
	configUnmarshallErr := yaml.Unmarshal(file, &cfg)

	return &cfg, configUnmarshallErr
}

// Config defines the configuration structure of the Watchtower.
type Config struct {
	// List of the Flows.
	Flows []Flow `yaml:"flows"`

	// List of the Taps.
	Taps []Tap `yaml:"taps"`

	// List of the Sinks.
	Sinks []Sink `yaml:"sinks"`
}

type CompiledConfig struct {
	Flows []CompiledFlow `yaml:"taps"`
}

func (c *Config) Compile() (*CompiledConfig, error) {
	compiledTaps := []CompiledTap{}
	compiledSinks := []CompiledSink{}
	compiledFlows := []CompiledFlow{}

	for _, tap := range c.Taps {
		compiledTaps = append(compiledTaps, tap.Compile())
	}

	for _, sink := range c.Sinks {
		compiledSinks = append(compiledSinks, sink.Compile())
	}

	compiledFlowMap := make(map[string]*CompiledFlow)
	for _, flow := range c.Flows {
		if _, exist := compiledFlowMap[flow.Tap]; !exist {
			compiledFlowMap[flow.Tap] = &CompiledFlow{}
		}

		for _, compiledTap := range compiledTaps {
			if flow.Tap == compiledTap.Name {
				compiledFlowMap[flow.Tap].Tap = compiledTap

				break
			}
		}

		for _, sink := range compiledSinks {
			if flow.Sink == sink.Name {
				compiledFlowMap[flow.Tap].Sinks = append(compiledFlowMap[flow.Tap].Sinks, sink)
			}
		}
	}

	for _, flow := range compiledFlowMap {
		if flow.Tap.Name == "" {
			return nil, fmt.Errorf("flow missing tap")
		}

		if len(flow.Sinks) == 0 {
			return nil, fmt.Errorf("flow missing sinks")
		}

		compiledFlows = append(compiledFlows, CompiledFlow{
			Tap:   flow.Tap,
			Sinks: flow.Sinks,
		})
	}

	return &CompiledConfig{
		Flows: compiledFlows,
	}, nil
}
