package common

import (
	"bytes"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/decls"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	RequiredHeaderPartCount = 2
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

func MapContains(a, b map[string]string) bool {
	for key, val := range b {
		valA, contains := a[key]
		if !contains || valA != val {
			return false
		}
	}

	return true
}

func StringToMap(str string) map[string][]string {
	result := make(map[string][]string)

	for _, line := range strings.Split(str, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", RequiredHeaderPartCount)
		if len(parts) == RequiredHeaderPartCount {
			key := strings.TrimSpace(strings.Trim(parts[0], "\" "))
			value := strings.TrimSpace(strings.Trim(parts[1], "\" "))
			result[key] = append(result[key], value)
		}
	}

	return result
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

func EvaluateCelExpressionBool(data map[string]any, criteria string) (bool, error) {
	if len(strings.TrimSpace(criteria)) == 0 {
		return true, nil
	}

	declarations := make([]*decls.VariableDecl, 0, len(data))

	for key := range data {
		declarations = append(declarations, decls.NewVariable(key, cel.AnyType))
	}

	env, newEnvErr := cel.NewEnv(cel.VariableDecls(declarations...))
	if newEnvErr != nil {
		return false, newEnvErr
	}

	ast, issues := env.Compile(criteria)
	if issues != nil && issues.Err() != nil {
		return false, issues.Err()
	}

	program, programErr := env.Program(ast)
	if programErr != nil {
		return false, programErr
	}

	output, _, evalErr := program.Eval(data)
	if evalErr != nil {
		return false, evalErr
	}

	outputBool, isOutputBool := output.Value().(bool)
	if !isOutputBool || !outputBool {
		return false, nil
	}

	return true, nil
}
