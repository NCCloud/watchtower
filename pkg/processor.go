package pkg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"

	"sigs.k8s.io/controller-runtime/pkg/cache"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/nccloud/watchtower/pkg/apis/v1alpha2"
	"github.com/nccloud/watchtower/pkg/common"
)

type Processor struct {
	client           cache.Cache
	watcher          *v1alpha2.Watcher
	httpClient       *http.Client
	templateRenderer *common.TemplateRenderer
}

func NewProcessor(client cache.Cache, watcher *v1alpha2.Watcher) *Processor {
	return &Processor{
		client:           client,
		httpClient:       &http.Client{},
		watcher:          watcher,
		templateRenderer: common.NewTemplateRenderer(client),
	}
}

func (r *Processor) Filter(_ context.Context, oldObj, newObj *unstructured.Unstructured) (bool, error) {
	isUpdate := oldObj != nil && oldObj.GetResourceVersion() != newObj.GetResourceVersion()

	expression := r.watcher.Spec.Filter.Create
	if isUpdate {
		expression = r.watcher.Spec.Filter.Update
	}

	if len(strings.TrimSpace(expression)) == 0 {
		return true, nil
	}

	env, envErr := cel.NewEnv(
		cel.Variable("old", cel.AnyType),
		cel.Variable("new", cel.AnyType),
		cel.Function("now",
			cel.Overload("now", []*cel.Type{}, cel.TimestampType,
				cel.FunctionBinding(func(_ ...ref.Val) ref.Val {
					return types.Timestamp{Time: time.Now()}
				}),
			),
		),
	)
	if envErr != nil {
		return false, envErr
	}

	ast, issues := env.Compile(expression)
	if issues != nil {
		return false, issues.Err()
	}

	program, programErr := env.Program(ast)
	if programErr != nil {
		return false, programErr
	}

	data := map[string]any{
		"new": newObj.Object,
	}

	if oldObj != nil {
		data["old"] = oldObj.Object
	}

	output, _, evalErr := program.Eval(data)
	if evalErr != nil {
		return false, evalErr
	}

	result, ok := output.Value().(bool)
	if !ok {
		return false, errors.New("result type is not bool")
	}

	return result, nil
}

func (r *Processor) Send(ctx context.Context, obj *unstructured.Unstructured) error {
	headerTemplate, headerTemplateParseErr := r.templateRenderer.Parse(r.watcher.Spec.Destination.HeaderTemplate)
	if headerTemplateParseErr != nil {
		return headerTemplateParseErr
	}

	bodyTemplate, bodyTemplateParseErr := r.templateRenderer.Parse(r.watcher.Spec.Destination.BodyTemplate)
	if bodyTemplateParseErr != nil {
		return bodyTemplateParseErr
	}

	urlTemplate, urlTemplateParseErr := r.templateRenderer.Parse(r.watcher.Spec.Destination.URLTemplate)
	if urlTemplateParseErr != nil {
		return urlTemplateParseErr
	}

	header, headerErr := r.templateRenderer.Render(headerTemplate, obj.Object)
	if headerErr != nil {
		return headerErr
	}

	body, bodyErr := r.templateRenderer.Render(bodyTemplate, obj.Object)
	if bodyErr != nil {
		return bodyErr
	}

	url, urlErr := r.templateRenderer.Render(urlTemplate, obj.Object)
	if urlErr != nil {
		return urlErr
	}

	request, requestErr := http.NewRequestWithContext(ctx, r.watcher.Spec.Destination.Method,
		url, bytes.NewReader([]byte(body)))
	if requestErr != nil {
		return requestErr
	}

	request.Header = common.StringToMap(header)

	doRequest, doRequestErr := r.httpClient.Do(request)
	if doRequestErr != nil {
		return doRequestErr
	}
	defer doRequest.Body.Close()

	if doRequest.StatusCode < 200 || doRequest.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", doRequest.StatusCode)
	}

	return nil
}
