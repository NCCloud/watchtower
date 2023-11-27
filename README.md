<br><picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://abload.de/img/watchtower4nsdoz.png">
    <img alt="logo" width="700" src="https://abload.de/img/watchtower32hej7.png">
</picture>

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
Also there are few environment variables that can be found in [config.go](https://github.com/NCCloud/tree/main/common/config.go)

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
    method: "POST"
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
      event:
        create:
          creationTimeout: "96h"
      #  update:
      #    generationChanged: true
      object:
        name: "^.*$-token-.*$"
        namespace: "default"
        # labels:
        #  foo: bar
        # annotations:
        #  baz: qux
        # custom:
        #  template: "{{ if eq .Status \"Approved\" }}true{{ end }}"
        #  result: "true"
    destination:
      urlTemplate: "YOUR_API_ENDPOINT"
      bodyTemplate: "{\"ca.crt\":\"{{ index .data \"ca.crt\" }}\",\"token\":\"{{ index .data \"token\" }}\"}"
      method: "PATCH"
      headers:
        Content-Type:
          - "application/json"
```

## 🏷️ Versioning

We use [SemVer](http://semver.org/) for versioning.
To see the available versions, check the [tags on this repository](https://github.com/nccloud/watchtower/tags).

## ⭐️ Documentation

For more information about the functionality provided by this library, refer to the 
[GoDoc Documentation](http://godoc.org/github.com/nccloud/watchtower) and [CRD Documentation](https://github.com/NCCloud/tree/main/docs/api.md).

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
