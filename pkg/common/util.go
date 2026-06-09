package common

import (
	"bytes"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/go-logr/logr"
	"github.com/google/cel-go/cel"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TemplateParse(str string) *template.Template {
	return template.Must(template.New("self").Funcs(sprig.TxtFuncMap()).Parse(str))
}

func TemplateExecuteForObject(template *template.Template, obj *unstructured.Unstructured) ([]byte, error) {
	var buffer bytes.Buffer
	if executeErr := template.Execute(&buffer, obj.Object); executeErr != nil {
		return nil, executeErr
	}

	return buffer.Bytes(), nil
}

func EvalCELPredicate(logger logr.Logger, program cel.Program, vars map[string]any) bool {
	out, _, evalErr := program.Eval(vars)
	if evalErr != nil {
		logger.V(1).Info("CEL eval failed", "err", evalErr.Error())

		return false
	}

	result, ok := out.Value().(bool)

	return ok && result
}

func Must(e error) {
	if e != nil {
		panic(e)
	}
}

func MustReturn[T any](t T, err error) T {
	Must(err)

	return t
}
