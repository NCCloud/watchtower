package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"sigs.k8s.io/yaml"
)

type oldWatcher struct {
	APIVersion string         `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`
	Kind       string         `json:"kind,omitempty" yaml:"kind,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       oldSpec        `json:"spec,omitempty" yaml:"spec,omitempty"`
}

type oldSpec struct {
	Source      map[string]any `json:"source,omitempty" yaml:"source,omitempty"`
	Filter      oldFilter      `json:"filter,omitempty" yaml:"filter,omitempty"`
	Destination oldDestination `json:"destination,omitempty" yaml:"destination,omitempty"`
	ValuesFrom  any            `json:"valuesFrom,omitempty" yaml:"valuesFrom,omitempty"`
}

type oldFilter struct {
	Event  oldEventFilter  `json:"event,omitempty" yaml:"event,omitempty"`
	Object oldObjectFilter `json:"object,omitempty" yaml:"object,omitempty"`
}

type oldEventFilter struct {
	Create oldCreateEventFilter `json:"create,omitempty" yaml:"create,omitempty"`
	Update oldUpdateEventFilter `json:"update,omitempty" yaml:"update,omitempty"`
}

type oldCreateEventFilter struct {
	CreationTimeout *string `json:"creationTimeout,omitempty" yaml:"creationTimeout,omitempty"`
}

type oldUpdateEventFilter struct {
	GenerationChanged      *bool    `json:"generationChanged,omitempty"      yaml:"generationChanged,omitempty"`
	ResourceVersionChanged *bool    `json:"resourceVersionChanged,omitempty" yaml:"resourceVersionChanged,omitempty"`
	Fields                 []string `json:"fields,omitempty"                 yaml:"fields,omitempty"`
}

type oldObjectFilter struct {
	Name        *string            `json:"name,omitempty"        yaml:"name,omitempty"`
	Namespace   *string            `json:"namespace,omitempty"   yaml:"namespace,omitempty"`
	Labels      *map[string]string `json:"labels,omitempty"      yaml:"labels,omitempty"`
	Annotations *map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	Custom      *oldCustom         `json:"custom,omitempty"      yaml:"custom,omitempty"`
}

type oldCustom struct {
	Template string `json:"template,omitempty" yaml:"template,omitempty"`
	Result   string `json:"result,omitempty"   yaml:"result,omitempty"`
}

type oldDestination struct {
	URLTemplate    string `json:"urlTemplate,omitempty"    yaml:"urlTemplate,omitempty"`
	BodyTemplate   string `json:"bodyTemplate,omitempty"   yaml:"bodyTemplate,omitempty"`
	HeaderTemplate string `json:"headerTemplate,omitempty" yaml:"headerTemplate,omitempty"`
	Method         string `json:"method,omitempty"         yaml:"method,omitempty"`
}

type newWatcher struct {
	APIVersion string         `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`
	Kind       string         `json:"kind,omitempty"       yaml:"kind,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"   yaml:"metadata,omitempty"`
	Spec       newSpec        `json:"spec,omitempty"       yaml:"spec,omitempty"`
}

type newSpec struct {
	Source      map[string]any `json:"source,omitempty"      yaml:"source,omitempty"`
	Filter      newFilter      `json:"filter,omitempty"      yaml:"filter,omitempty"`
	Destination newDestination `json:"destination,omitempty" yaml:"destination,omitempty"`
	ValuesFrom  any            `json:"valuesFrom,omitempty"  yaml:"valuesFrom,omitempty"`
}

type newFilter struct {
	Create string `json:"create,omitempty" yaml:"create,omitempty"`
	Update string `json:"update,omitempty" yaml:"update,omitempty"`
}

type newDestination struct {
	URLTemplate  string            `json:"urlTemplate,omitempty"  yaml:"urlTemplate,omitempty"`
	BodyTemplate string            `json:"bodyTemplate,omitempty" yaml:"bodyTemplate,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"      yaml:"headers,omitempty"`
	Method       string            `json:"method,omitempty"       yaml:"method,omitempty"`
	Timeout      string            `json:"timeout,omitempty"      yaml:"timeout,omitempty"`
}

func main() {
	src, err := readSource()
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}

	var old oldWatcher
	if unmarshalErr := yaml.Unmarshal(src, &old); unmarshalErr != nil {
		fmt.Fprintln(os.Stderr, "parse:", unmarshalErr)
		os.Exit(1)
	}

	converted, warnings := convert(old)

	out, marshalErr := yaml.Marshal(converted)
	if marshalErr != nil {
		fmt.Fprintln(os.Stderr, "marshal:", marshalErr)
		os.Exit(1)
	}

	if _, writeErr := os.Stdout.Write(out); writeErr != nil {
		fmt.Fprintln(os.Stderr, "write:", writeErr)
		os.Exit(1)
	}

	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}

	if len(warnings) > 0 {
		os.Exit(2)
	}
}

func readSource() ([]byte, error) {
	if len(os.Args) > 1 {
		return os.ReadFile(os.Args[1])
	}

	return io.ReadAll(os.Stdin)
}

func convert(in oldWatcher) (newWatcher, []string) {
	var (
		warnings    []string
		objClauses  = objectClauses(in.Spec.Filter.Object, &warnings)
		createParts []string
		updateParts []string
	)

	if to := in.Spec.Filter.Event.Create.CreationTimeout; to != nil && *to != "" {
		createParts = append(createParts,
			fmt.Sprintf("now - timestamp(object.metadata.creationTimestamp) < duration('%s')", *to))
	}

	createParts = append(createParts, objClauses...)
	updateParts = append(updateParts, objClauses...)

	if gc := in.Spec.Filter.Event.Update.GenerationChanged; gc != nil {
		if *gc {
			updateParts = append(updateParts, "object.metadata.generation != oldObject.metadata.generation")
		} else {
			updateParts = append(updateParts, "object.metadata.generation == oldObject.metadata.generation")
		}
	}

	if rvc := in.Spec.Filter.Event.Update.ResourceVersionChanged; rvc != nil {
		if *rvc {
			updateParts = append(updateParts, "object.metadata.resourceVersion != oldObject.metadata.resourceVersion")
		} else {
			updateParts = append(updateParts, "object.metadata.resourceVersion == oldObject.metadata.resourceVersion")
		}
	}

	if len(in.Spec.Filter.Event.Update.Fields) > 0 {
		updateParts = append(updateParts, fieldsClause(in.Spec.Filter.Event.Update.Fields))
	}

	out := newWatcher{
		APIVersion: in.APIVersion,
		Kind:       in.Kind,
		Metadata:   in.Metadata,
		Spec: newSpec{
			Source:     in.Spec.Source,
			ValuesFrom: in.Spec.ValuesFrom,
			Filter: newFilter{
				Create: joinAnd(createParts),
				Update: joinAnd(updateParts),
			},
			Destination: newDestination{
				URLTemplate:  in.Spec.Destination.URLTemplate,
				BodyTemplate: in.Spec.Destination.BodyTemplate,
				Method:       in.Spec.Destination.Method,
				Headers:      headersFromTemplate(in.Spec.Destination.HeaderTemplate, &warnings),
			},
		},
	}

	return out, warnings
}

func objectClauses(o oldObjectFilter, warnings *[]string) []string {
	var clauses []string

	if o.Name != nil && *o.Name != "" {
		clauses = append(clauses, fmt.Sprintf("object.metadata.name.matches(%s)", celString(*o.Name)))
	}

	if o.Namespace != nil && *o.Namespace != "" {
		clauses = append(clauses, fmt.Sprintf("object.metadata.namespace.matches(%s)", celString(*o.Namespace)))
	}

	if o.Labels != nil {
		for k, v := range *o.Labels {
			clauses = append(clauses, fmt.Sprintf("object.metadata.labels[%s] == %s", celString(k), celString(v)))
		}
	}

	if o.Annotations != nil {
		for k, v := range *o.Annotations {
			clauses = append(clauses, fmt.Sprintf("object.metadata.annotations[%s] == %s", celString(k), celString(v)))
		}
	}

	if o.Custom != nil {
		*warnings = append(*warnings,
			"filter.object.custom is not mechanically convertible. Rewrite the Go template "+
				fmt.Sprintf("%q (result: %q) as a CEL boolean by hand.", o.Custom.Template, o.Custom.Result))
	}

	return clauses
}

func fieldsClause(fields []string) string {
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		path := strings.TrimPrefix(f, ".")
		parts = append(parts, fmt.Sprintf("object.%s != oldObject.%s", path, path))
	}

	if len(parts) == 1 {
		return parts[0]
	}

	return "(" + strings.Join(parts, " || ") + ")"
}

func headersFromTemplate(tmpl string, warnings *[]string) map[string]string {
	if tmpl == "" {
		return nil
	}

	headers := make(map[string]string)

	for _, raw := range strings.Split(tmpl, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		idx := strings.Index(line, ":")
		if idx < 0 {
			*warnings = append(*warnings, fmt.Sprintf("skipped header line without ':' separator: %q", line))

			continue
		}

		key := strings.TrimSpace(strings.Trim(line[:idx], "\" "))
		val := strings.TrimSpace(strings.Trim(line[idx+1:], "\" "))
		headers[key] = val
	}

	return headers
}

func joinAnd(parts []string) string {
	if len(parts) == 0 {
		return ""
	}

	if len(parts) == 1 {
		return parts[0]
	}

	return strings.Join(parts, " &&\n")
}

func celString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `\'`) + "'"
}
