<br><picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://abload.de/img/watchtower4nsdoz.png">
    <img alt="logo" width="700" src="https://abload.de/img/watchtower32hej7.png">
</picture>

## 📖 General Information

Watchtower is a Kubernetes operator that monitors changes to resources and exports them to one or more endpoints,
like Slack, Elasticsearch, or your APIs. It filters objects based on user-specified criteria, prepares a
template, and sends the request to the appropriate endpoint.

## 🚀 Deployment

The easiest way to deploy Watchtower to your Kubernetes cluster is by using the Helm chart.
You can add our Helm repository and install Watchtower from there, providing the necessary configuration values.

Example:
```
helm repo add nccloud https://nccloud.github.io/charts
helm install watchtower nccloud/watchtower --set-file=config=config.yaml # You can check examples section to prepare configuration. 
```
Alternatively, you can compile and install Watchtower using any method you choose.

## ⚙️ Configuration

Watchtower's configuration is stored in the `config.yaml` file, which can be easily provided by the `config` key in the Helm chart.
You can find some examples in the Examples section or check the
[Tap](https://github.com/NCCloud/watchtower/blob/update-readme/pkg/models/tap.go),
[Sink](https://github.com/NCCloud/watchtower/blob/update-readme/pkg/models/sink.go) and
[Flow](https://github.com/NCCloud/watchtower/blob/update-readme/pkg/models/flow.go) for all the fields.

## 📐 Architecture

Watchtower is based on the [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) which helps you to build a Kubernetes operator.
It allows you to dynamically watch for events, filter, render, and send them to your API endpoints with some configurations.
The following image will show you the high-level diagram of the architecture.

![Architecture](https://user-images.githubusercontent.com/23269628/223709739-b6567e76-cb39-49a3-a55b-237a9c57c2dd.jpg)

## 🛠 Development

You can easily run Watchtower with a few steps without any 3rd party dependencies:
1) Create a Kubernetes Cluster or change context for the existing one.
```bash
kind create cluster
```
2) (Optionally) Create a hook from `https://webhook.site` for testing purposes.
3) Edit config.yaml according to your wish.
4) Run the application;
```bash
go run cmd/manager/main.go
```

## 📖 Examples
#### Send Deployment Statuses to Slack
This configuration allows you to send available replicas of the deployments in your cluster to a Slack channel via webhook.

```yaml
# config.yaml
taps:
- name: MyDeployments
  kind: Deployment
  apiVersion: apps/v1
sinks:
- name: MySlackWebhook
  method: POST
  urlTemplate: "YOUR_SLACK_WEBHOOK_URL"
  bodyTemplate: "{\"text\":\"Name: {{ .metadata.name }}\nAvailableReplicas: {{ .status.availableReplicas }}\"}"
flows:
- tap: MyDeployments
  sink: MySlackWebhook
```

#### Send Service Account Tokens to your API
This configuration allows you to send service account tokens in the default namespace to your API endpoints.

```yaml
# config.yaml
taps:
- name: ServiceAccountTokens
  kind: Secret
  apiVersion: v1
  filter:
    name: "^.*$-token-.*$"
    namespace: "default"
    object:
      key: ".type"
      operator: "=="
      value: "kubernetes.io/service-account-token"
sinks:
- name: MyAPIEndpoint
  method: PATCH
  urlTemplate: "YOUR_API_ENDPOINT"
  bodyTemplate: "{\"ca.crt\":\"{{ index .data \"ca.crt\" }}\",\"token\":\"{{ index .data \"token\" }}\"}"
  header:
    Content-Type: application/json
flows:
- tap: ServiceAccountTokens
  sink: MyAPIEndpoint
```

## 🏷️ Versioning

We use [SemVer](http://semver.org/) for versioning.
To see the available versions, check the [tags on this repository](https://github.com/nccloud/watchtower/tags).

## ⭐️ Documentation

For more information about the functionality provided by this library, refer to the [GoDoc](http://godoc.org/github.com/nccloud/watchtower) documentation.

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
