# API Reference

## Packages
- [cloud.spaceship.com/v1alpha1](#cloudspaceshipcomv1alpha1)


## cloud.spaceship.com/v1alpha1

Package v1alpha1 contains API Schema definitions for the  v1alpha1 API group

### Resource Types
- [Watcher](#watcher)



#### CreateEventFilter







_Appears in:_
- [EventFilter](#eventfilter)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `creationTimeout` _string_ | CreationTimeout sets what will be the maximum duration can past for the objects in create queue.<br />It also helps to minimize number of object that will be re-sent when application restarts. |  |  |


#### CustomObjectFilter







_Appears in:_
- [ObjectFilter](#objectfilter)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `template` _string_ | Template is the template that will be used to compare result with Result and filter accordingly. |  |  |
| `result` _string_ | Result is the result that will be used to compare with the result of the Template. |  |  |


#### Destination







_Appears in:_
- [WatcherSpec](#watcherspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `urlTemplate` _string_ | URLTemplate is the template field to set where will be the destination. |  |  |
| `bodyTemplate` _string_ | BodyTemplate is the template field to set what will be sent the destination. |  |  |
| `method` _string_ | Method is the HTTP method will be used while calling the destination endpoints. |  |  |
| `headers` _object (keys:string, values:string array)_ | Method is the HTTP headers will be used while calling the destination endpoints. |  |  |


#### EventFilter







_Appears in:_
- [Filter](#filter)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `create` _[CreateEventFilter](#createeventfilter)_ | Create allows you to set create event based filters |  |  |
| `update` _[UpdateEventFilter](#updateeventfilter)_ | Update allows you to set update event based filters |  |  |


#### Filter







_Appears in:_
- [WatcherSpec](#watcherspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `event` _[EventFilter](#eventfilter)_ | Event allows you to set event based filters |  |  |
| `object` _[ObjectFilter](#objectfilter)_ | Object allows you to set object based filters |  |  |


#### ObjectFilter







_Appears in:_
- [Filter](#filter)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the regular expression to filter object Its name. |  |  |
| `namespace` _string_ | Namespace is the regular expression to filter object Its namespace. |  |  |
| `labels` _map[string]string_ | Labels are the labels to filter object by labels. |  |  |
| `annotations` _map[string]string_ | Annotations are the labels to filter object by annotation. |  |  |
| `custom` _[CustomObjectFilter](#customobjectfilter)_ | Custom is the most advanced way of filtering object by their contents and multiple fields by templating. |  |  |


#### OnSuccessSourceOptions







_Appears in:_
- [SourceOptions](#sourceoptions)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `deleteObject` _boolean_ | DeleteObject will delete the object after it successfully processed. |  |  |


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


#### UpdateEventFilter







_Appears in:_
- [EventFilter](#eventfilter)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `generationChanged` _boolean_ | GenerationChanged sets if generation should be different or same according to value.<br />It's useful when you want/don't want to send objects when their sub-resources are updated, like status updates.<br />By default, It's not set. |  |  |
| `resourceVersionChanged` _boolean_ | ResourceVersionChanged sets if resource version should be different or same according to value.<br />It's useful when you don't want to re-send objects if their resource version is not changed,<br />like it will happen on full re-synchronization. By default, It's not set. |  |  |


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
| `filter` _[Filter](#filter)_ | Filter helps filter objects during the watching process. |  |  |
| `destination` _[Destination](#destination)_ | Destination sets where the rendered objects will be sent. |  |  |
| `valuesFrom` _[ValuesFrom](#valuesfrom)_ | ValuesFrom allows merging variables from references. |  |  |


