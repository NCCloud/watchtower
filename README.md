<br><picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://abload.de/img/watchtower4nsdoz.png">
    <img alt="logo" width="700" src="https://abload.de/img/watchtower32hej7.png">
</picture>

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/nccloud/watchtower)
![GitHub Release](https://img.shields.io/github/v/release/nccloud/watchtower)
[![Go Reference](https://pkg.go.dev/badge/github.com/NCCloud/watchtower.svg)](https://pkg.go.dev/github.com/NCCloud/watchtower)
![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/nccloud/watchtower/test.yaml?label=tests)
![GitHub issues](https://img.shields.io/github/issues/nccloud/watchtower)
![GitHub License](https://img.shields.io/github/license/nccloud/watchtower)

## 📖 General Information

Watchtower is CRD-based Kubernetes operator that monitors changes to resources and exports them to one or more endpoints,
like Slack, Elasticsearch, or your APIs. It listen the events and collect the objects, then filter them based on user-specified criteria, prepares a
template, and sends the request to the provided destination.

## 🚀 Deployment

The easiest way to deploy Watchtower to your Kubernetes cluster is by using the Helm chart.
You can add our Helm repository and install Watchtower from there.

Example:
```
helm repo add nccloud https://nccloud.github.io/charts
helm install watchtower nccloud/watchtower
```
Alternatively, you can compile and install Watchtower using any method you choose. Then, you are ready create Watcher custom resources!

## ⚙️ Configuration

Watchtower can be configured by creating and deleting the Watcher CRDs. Examples can be found in de Examples section.
Also there are few environment variables that can be found in [config.go](https://github.com/NCCloud/watchtower/tree/main/pkg/common/config.go)

## 📐 Architecture

Watchtower is based on the [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) which helps you to build a Kubernetes operator.
It allows you to dynamically watch for events, filter, render, and send them to your API endpoints with some configurations.
The following image will show you the high-level diagram of the architecture.

![Architecture](https://github.com/NCCloud/watchtower/assets/23269628/8016a7ce-0d94-4b82-99d2-093bb7bf2cfd)

## 🛠 Development

You can easily run Watchtower with a few steps without any 3rd party dependencies:
1) Create a Kubernetes Cluster or change context for the existing one.
```bash
kind create cluster
```
2) (Optionally) Create a hook from `https://webhook.site` for testing purposes.
3) Install CRDs by running ./devops.sh install
4) (Optionally) Create Watcher resources by checking the examples section.
4) Run the application;
```bash
go run cmd/manager/main.go
```

## 📖 Examples
#### Send Deployment Statuses to Slack (Simple Configuration)
This configuration allows you to send available replicas of the deployments in your cluster to a Slack channel via webhook.

```yaml
apiVersion: cloud.spaceship.com/v1alpha1
kind: Watcher
metadata:
  name: slack-deployment-sender
spec:
  source:
    apiVersion: "apps/v1"
    kind: "Deployment"
  destination:
    urlTemplate: "YOUR_SLACK_WEBHOOK_URL"
    bodyTemplate: |
      { "text": "{{ .metadata.name }}" }
```

#### Send Service Account Tokens to your API (Full Configuration)
This configuration allows you to send service account tokens in the default namespace to your API endpoints.

```yaml
apiVersion: cloud.spaceship.com/v1alpha1
kind: Watcher
metadata:
  name: service-account-token-sender
spec:
  source:
    apiVersion: "v1"
    kind: "Secret"
    concurrency: 10
  filter:
    # Only Create events for matching token secrets created within the last 96h.
    create: |
      now - timestamp(object.metadata.creationTimestamp) < duration('96h') &&
      object.metadata.name.matches('^.*-token-.*$') &&
      object.metadata.namespace == 'default'
  destination:
    urlTemplate: "YOUR_API_ENDPOINT"
    bodyTemplate: "{\"ca.crt\":\"{{ index .data \"ca.crt\" }}\",\"token\":\"{{ index .data \"token\" }}\"}"
    method: "PATCH"
    headers:
      Content-Type: "application/json"
```

## 🎯 Filtering with CEL

`filter.create` and `filter.update` are [CEL](https://cel.dev/) boolean expressions. The event passes when the expression evaluates `true`; omit the field to pass every event of that type.

Bindings:

| Binding | Available in | Description |
| --- | --- | --- |
| `object` | `create`, `update`, `delete` | The current (or last-known, for delete) state of the object. |
| `oldObject` | `update` | The previous state of the object. |
| `now` | `create`, `update`, `delete` | The current timestamp (UTC). |

`filter.delete` is special: omitting it filters out **every** Delete event (the conservative default). Set it to `true` to fire on every deletion, or to a CEL expression to fire selectively.

Recipes:

```cel
// Restart safety: skip Create events for objects older than 1h.
now - timestamp(object.metadata.creationTimestamp) < duration('1h')

// Status phase transition.
object.status.phase != oldObject.status.phase

// Replica scale-up only (ignore scale-downs).
int(object.spec.replicas) > int(oldObject.spec.replicas)

// Any change in .status.conditions (deep equality on the list).
object.status.conditions != oldObject.status.conditions

// Specific label gate.
object.metadata.labels['app'] == 'web'

// Drift detection: desired ≠ actual.
has(object.spec.replicas) && has(object.status.readyReplicas) &&
  int(object.spec.replicas) != int(object.status.readyReplicas)
```

## 🔁 Migrating from older versions

The pre-CEL CRD had a structured `filter.event` + `filter.object` shape. The new shape replaces both with two CEL expressions. A migration helper lives at [`hack/migrate-watcher.sh`](hack/migrate-watcher.sh); it reads an old Watcher YAML and prints the new equivalent.

Mechanical conversion table:

| Old | New |
| --- | --- |
| `filter.event.create.creationTimeout: "1h"` | `filter.create: "now - timestamp(object.metadata.creationTimestamp) < duration('1h')"` |
| `filter.event.update.generationChanged: true` | `filter.update: "object.metadata.generation != oldObject.metadata.generation"` |
| `filter.event.update.resourceVersionChanged: true` | `filter.update: "object.metadata.resourceVersion != oldObject.metadata.resourceVersion"` |
| `filter.event.update.fields: [".a", ".b"]` | `filter.update: "object.a != oldObject.a \|\| object.b != oldObject.b"` |
| `filter.object.name: "rx"` | wrap into `create`/`update`: `object.metadata.name.matches('rx')` |
| `filter.object.namespace: "rx"` | `object.metadata.namespace.matches('rx')` |
| `filter.object.labels: {k: v}` | `object.metadata.labels['k'] == 'v'` |
| `filter.object.annotations: {k: v}` | `object.metadata.annotations['k'] == 'v'` |
| `filter.object.custom: {template, result}` | rewrite by hand into CEL — Go template → CEL is not mechanical |
| `destination.headerTemplate: "..."` (YAML string) | `destination.headers: {k: v}` (map of templated values) |

## 🏷️ Versioning

We use [SemVer](http://semver.org/) for versioning.
To see the available versions, check the [tags on this repository](https://github.com/nccloud/watchtower/tags).

## ⭐️ Documentation

For more information about the functionality provided by this library, refer to the 
[GoDoc Documentation](http://godoc.org/github.com/nccloud/watchtower) and [CRD Documentation](https://github.com/NCCloud/watchtower/tree/main/docs/api.md).

## 🤝 Contribution

We welcome contributions, issues, and feature requests!<br />
If you have any issues or suggestions, please feel free to check the [issues page](https://github.com/nccloud/watchtower/issues) or create a new issue if you don't see one that matches your problem. <br>
Also, please refer to our [contribution guidelines](CONTRIBUTING.md) for details.

## 📝 License
All functionalities are in beta and is subject to change. The code is provided as-is with no warranties.<br>
[Apache 2.0 License](./LICENSE)<br>
<br><br>
<img alt="logo" width="75" src="https://avatars.githubusercontent.com/u/7532706" /><br>
Made with <span style="color: #e25555;">&hearts;</span> by [Namecheap Cloud Team](https://github.com/NCCloud)
