package utils

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
