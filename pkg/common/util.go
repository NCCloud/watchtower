package common

import (
	"bytes"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
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
