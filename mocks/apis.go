package mocks

import (
	"github.com/brianvoe/gofakeit/v6"
	"github.com/nccloud/watchtower/pkg/apis/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

func NewWatcher() *v1alpha1.Watcher {
	return (&v1alpha1.Watcher{
		ObjectMeta: metav1.ObjectMeta{
			Name:      strings.ToLower(strings.ReplaceAll(gofakeit.AppName(), " ", "")),
			Namespace: "default",
		},
		Spec: v1alpha1.WatcherSpec{
			Filter: v1alpha1.Filter{},
			Destination: v1alpha1.Destination{
				URLTemplate:  "www.website.com/{{ .metadata.name }}",
				BodyTemplate: "{{ .data.data }}",
				Method:       "POST",
				Headers: map[string][]string{
					"Content-Type": {
						"application/json",
					},
				},
			},
		},
	}).Compile()
}
