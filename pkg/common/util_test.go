package common

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestTemplateParse(t *testing.T) {
	// given
	str := "{{ .Name }}"

	// when
	template := TemplateParse(str)

	// then
	assert.NotNil(t, template)
}

func TestTemplateExecuteForObject(t *testing.T) {
	// given
	template := TemplateParse("{{ .Name }}")
	object := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"Name": "test",
		},
	}

	// when
	result, err := TemplateExecuteForObject(template, object)

	// then
	assert.Nil(t, err)
	assert.Equal(t, "test", string(result))
}

func TestMust(t *testing.T) {
	// given
	err := errors.New("test")

	// when
	f := func() { Must(err) }

	// then
	assert.Panics(t, f)
}

func TestMustReturn(t *testing.T) {
	// given
	err := errors.New("test")
	data := "test"

	// when
	willPanic := func() { MustReturn(data, err) }
	willNotPanic := func() { MustReturn(data, nil) }

	// then
	assert.Panics(t, willPanic)
	assert.NotPanics(t, willNotPanic)
}

func TestMapContains(t *testing.T) {
	// given
	a := map[string]string{
		"key": "value",
	}
	b := map[string]string{
		"key":  "value",
		"key2": "value2",
	}

	// when
	contains := MapContains(b, a)
	notContains := MapContains(a, b)

	// then
	assert.True(t, contains)
	assert.False(t, notContains)
}
