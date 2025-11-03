# API Reference

## Packages
- [cloud.spaceship.com/v1alpha2](#cloudspaceshipcomv1alpha2)


## cloud.spaceship.com/v1alpha2

Package v1alpha2 contains API Schema definitions for the  v1alpha2 API group

### Resource Types
- [Watcher](#watcher)



#### Destination







_Appears in:_
- [WatcherSpec](#watcherspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `http` _[HTTPDestination](#httpdestination)_ | Http is the HTTP destination configuration. |  |  |


#### Filter







_Appears in:_
- [WatcherSpec](#watcherspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `create` _string_ | CreateFilter allows you to set create event based filters |  |  |
| `update` _string_ | UpdateFilter allows you to set update event based filters |  |  |
| `delete` _string_ | DeleteFilter allows you to set delete event based filters |  |  |


#### HTTPDestination







_Appears in:_
- [Destination](#destination)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `urlTemplate` _string_ | URLTemplate is the template field to set where will be the destination. |  |  |
| `bodyTemplate` _string_ | BodyTemplate is the template field to set what will be sent the destination. |  |  |
| `headerTemplate` _string_ | HeaderTemplate is the template field to set what will be sent the destination. |  |  |


#### Source







_Appears in:_
- [WatcherSpec](#watcherspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | APIVersion is api version of the object like apps/v1, v1 etc. |  |  |
| `kind` _string_ | Kind is the kind of the object like Deployment, Secret, MyCustomResource etc. |  |  |
| `concurrency` _integer_ | Concurrency is how many concurrent workers will be working on processing this source. |  |  |
| `policies` _string array_ | Options allows you to set source specific options |  |  |


#### ValuesFrom







_Appears in:_
- [WatcherSpec](#watcherspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kind` _[ValuesFromKind](#valuesfromkind)_ | Kind specifies whether the source is a Secret or ConfigMap. |  |  |
| `name` _string_ | Name is the name of the Secret or ConfigMap. |  |  |
| `key` _string_ | Key is the specific key within the Secret or ConfigMap to retrieve the value from. |  |  |


#### ValuesFromKind

_Underlying type:_ _string_

ValuesFromKind represents the possible sources for injecting values into an instance.



_Appears in:_
- [ValuesFrom](#valuesfrom)

| Field | Description |
| --- | --- |
| `Secret` | ValuesFromKindSecret specifies that values should be sourced from a Kubernetes Secret.<br /> |
| `ConfigMap` | ValuesFromKindConfigMap specifies that values should be sourced from a Kubernetes ConfigMap.<br /> |


#### Watcher









| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `cloud.spaceship.com/v1alpha2` | | |
| `kind` _string_ | `Watcher` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[WatcherSpec](#watcherspec)_ |  |  |  |


#### WatcherSpec







_Appears in:_
- [Watcher](#watcher)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `source` _[Source](#source)_ | Source defines the source objects of the watching process. |  |  |
| `filter` _[Filter](#filter)_ | Filter helps filter objects during the watching process. |  |  |
| `destination` _[Destination](#destination)_ | Destination sets where the rendered objects will be sent. |  |  |
| `valuesFrom` _[ValuesFrom](#valuesfrom) array_ | ValuesFrom allows merging variables from references. |  |  |


