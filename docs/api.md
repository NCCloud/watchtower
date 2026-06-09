# API Reference

## Packages
- [cloud.spaceship.com/v1alpha1](#cloudspaceshipcomv1alpha1)


## cloud.spaceship.com/v1alpha1

Package v1alpha1 contains API Schema definitions for the  v1alpha1 API group

### Resource Types
- [Watcher](#watcher)



#### Destination







_Appears in:_
- [WatcherSpec](#watcherspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `urlTemplate` _string_ | URLTemplate is the template field to set where will be the destination. |  |  |
| `bodyTemplate` _string_ | BodyTemplate is the template field to set what will be sent the destination. |  |  |
| `headers` _object (keys:string, values:string)_ | Headers is a map of header name to a templated value.<br />Keys are sent verbatim; values are rendered as Go templates against the object. |  |  |
| `method` _string_ | Method is the HTTP method used while calling the destination endpoints.<br />Defaults to POST when unset. |  |  |
| `timeout` _string_ | Timeout is the per-request HTTP timeout. Defaults to 30s when unset. |  |  |


#### Filter







_Appears in:_
- [WatcherSpec](#watcherspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `create` _string_ | Create is a CEL boolean predicate evaluated on Create events.<br />Bindings: object (the new object), now (current timestamp).<br />Omit (empty string) to pass every Create event. |  |  |
| `update` _string_ | Update is a CEL boolean predicate evaluated on Update events.<br />Bindings: object (the new object), oldObject (the previous object), now (current timestamp).<br />Omit (empty string) to pass every Update event. |  |  |
| `delete` _string_ | Delete is a CEL boolean predicate evaluated on Delete events.<br />Bindings: object (the last-known state of the deleted object), now (current timestamp).<br />Omit (empty string) to filter out every Delete event (the conservative default). |  |  |


#### OnSuccessSourceOptions







_Appears in:_
- [SourceOptions](#sourceoptions)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `deleteObject` _boolean_ | DeleteObject will delete the object after it successfully processed.<br />Has no effect on Delete events (the object is already gone). |  |  |


#### SecretKeySelector







_Appears in:_
- [ValuesFrom](#valuesfrom)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ |  |  |  |
| `namespace` _string_ |  |  |  |
| `key` _string_ |  |  |  |


#### Source







_Appears in:_
- [WatcherSpec](#watcherspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | APIVersion is api version of the object like apps/v1, v1 etc. |  |  |
| `kind` _string_ | Kind is the kind of the object like Deployment, Secret, MyCustomResource etc. |  |  |
| `concurrency` _integer_ | Concurrency is how many concurrent workers will be working on processing this source. |  |  |
| `options` _[SourceOptions](#sourceoptions)_ | Options allows you to set source specific options |  |  |


#### SourceOptions







_Appears in:_
- [Source](#source)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `onSuccess` _[OnSuccessSourceOptions](#onsuccesssourceoptions)_ | OnSuccess options will be used when the source is successfully processed. |  |  |


#### ValuesFrom







_Appears in:_
- [WatcherSpec](#watcherspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `secrets` _[SecretKeySelector](#secretkeyselector) array_ | Secrets are the references that will be merged from. |  |  |


#### Watcher









| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `cloud.spaceship.com/v1alpha1` | | |
| `kind` _string_ | `Watcher` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[WatcherSpec](#watcherspec)_ |  |  |  |


#### WatcherSpec







_Appears in:_
- [Watcher](#watcher)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `source` _[Source](#source)_ | Source defines the source objects of the watching process. |  |  |
| `filter` _[Filter](#filter)_ | Filter is a set of CEL predicates, one per event type. |  |  |
| `destination` _[Destination](#destination)_ | Destination sets where the rendered objects will be sent. |  |  |
| `valuesFrom` _[ValuesFrom](#valuesfrom)_ | ValuesFrom allows merging variables from references. |  |  |


