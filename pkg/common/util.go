package common

import (
	"bytes"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TemplateParse(str string) *template.Template {
	return template.Must(template.New("self").Funcs(sprig.TxtFuncMap()).Parse(str))
}

func TemplateExecuteForObject(template *template.Template, obj client.Object) ([]byte, error) {
	var buffer bytes.Buffer
	if executeErr := template.Execute(&buffer, obj.(*unstructured.Unstructured).Object); executeErr != nil {
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

func Pointer[T any](t T) *T {
	return &t
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
