# EventBridge ApiDestination `eventbridge-apidestination`

Creates an API destination, which is an HTTP invocation endpoint configured as a target for events. Can be targeted by [EventbridgeTarget](./eventbridge_target.md). See [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/events/create-api-destination.html) for more details.

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

| Field | Description | DataType | Required | Default |
|-------|-------------|----------|----------|---------|
| `name` | Name of the api destination | `string` | Yes |  |
| `description` | Description of the api destination | `string` | No |  |
| `method` | The method to use for the request to the HTTP invocation endpoint. Valid values: `POST`, `GET`, `HEAD`, `OPTIONS`, `PUT`, `PATCH`, `DELETE` | `string` | No | `GET` |
| `endpoint` | The URL to the HTTP invocation endpoint for the API destination | `string` | Yes |  |
| `invocationRateLimitPerSecond` | The maximum number of requests per second to send to the HTTP invocation endpoint | `int32` | Yes |  |
| `connectionName` | Name of the connection. Usually matches `name` | `string` | Yes |  |
|`dependsOn`|The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this|`[]string`|No|`[]`|
Example:

```yaml 
resources:
  eventbridge-apidestination:
    api-google-feed-realtime-destination:
      description: 
      endpoint: https://api.example.com/feeds/update
      method: POST
      invocationRateLimitPerSecond: 300
      connectionName: api-google-feed-realtime-connection
```
