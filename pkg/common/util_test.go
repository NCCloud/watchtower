package common_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/nccloud/watchtower/pkg/common"
)

func TestMust_NoError(t *testing.T) {
	//given
	var err error = nil

	//when

	//then
	common.Must(err)
}

func TestMust_PanicWhenErr(t *testing.T) {
	//given
	err := errors.New("test error")

	//when

	//then
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Must() did not panic with non-nil error")
		}
	}()

	common.Must(err)
}

func TestMustReturn_NoError(t *testing.T) {
	//given
	testValue := "test value"
	var err error = nil

	//when
	result := common.MustReturn(testValue, err)

	//then
	if result != testValue {
		t.Errorf("MustReturn() = %v, want %v", result, testValue)
	}
}

func TestMustReturn_PanicWhenErr(t *testing.T) {
	//given
	testValue := "test value"
	err := errors.New("test error")

	//when
	//then
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("MustReturn() did not panic with non-nil error")
		}
	}()

	_ = common.MustReturn(testValue, err)
}

func TestIgnoreError(t *testing.T) {
	//given
	testValue := "test value"
	err := errors.New("test error")

	//when
	result := common.IgnoreError(testValue, err)

	//then
	if result != testValue {
		t.Errorf("IgnoreError() = %v, want %v", result, testValue)
	}
}

func TestStringToMap(t *testing.T) {
	//given
	tests := []struct {
		name     string
		input    string
		expected map[string][]string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: map[string][]string{},
		},
		{
			name:  "single line",
			input: "Content-Type: application/json",
			expected: map[string][]string{
				"Content-Type": {"application/json"},
			},
		},
		{
			name:  "multiple lines",
			input: "Content-Type: application/json\nAccept: text/html\nAuthorization: Bearer token",
			expected: map[string][]string{
				"Content-Type":  {"application/json"},
				"Accept":        {"text/html"},
				"Authorization": {"Bearer token"},
			},
		},
		{
			name:  "multiple values for same key",
			input: "Accept: text/html\nAccept: application/json",
			expected: map[string][]string{
				"Accept": {"text/html", "application/json"},
			},
		},
		{
			name:  "line without colon",
			input: "Content-Type: application/json\nInvalid Line\nAccept: text/html",
			expected: map[string][]string{
				"Content-Type": {"application/json"},
				"Accept":       {"text/html"},
			},
		},
		{
			name:  "quoted strings",
			input: "\"Content-Type\": \"application/json\"\n\"Accept\": \"text/html\"",
			expected: map[string][]string{
				"Content-Type": {"application/json"},
				"Accept":       {"text/html"},
			},
		},
		{
			name:  "extra spaces",
			input: " Content-Type :  application/json \n Accept :  text/html ",
			expected: map[string][]string{
				"Content-Type": {"application/json"},
				"Accept":       {"text/html"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//when
			result := common.StringToMap(tt.input)

			//then
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("StringToMap() = %v, want %v", result, tt.expected)
			}
		})
	}
}
