# EventBridge Connection `eventbridge-connection`

Creates a connection. A connection defines the authorization type and credentials to use for authorization with an API destination HTTP endpoint. See [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/events/create-connection.html) for more details.

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

| Field | Description | DataType | Required | Default |
|-------|-------------|----------|----------|---------|
|`name`|The name for the connection to create|`string`|Yes| |
|`description`|A description for the connection to create|`string`|No| |
|`connectionParameters`|The connection parameters. See [EventBridgeConnectionParameters](#eventbridgeconnectionparameters)|`EventBridgeConnectionParameters`|Yes| |
|`tags`|**Not applied.** The AWS `CreateConnection` API takes no tags, so nothing set here — and no `globalTags`, including `buildit:resource-id` — reaches the connection|`map[string]string`|No|`{}`|
|`dependsOn`|The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this|`[]string`|No|`[]`|

## EventBridgeConnectionParameters
Contains the connection auth & additional parameters
| Field | Description | DataType | Required | Default |
|-------|-------------|----------|----------|---------|
|`apiKeyName`|Name of the api key |`string`|Yes| |
|`apiKeyValue`|Value of the api key. Read from a secrets manager secret, value supplied as `secretName:key` |`string`|Yes| |
|`invocationParameters`|The parameters to use for invoking the resource endpoint. See [EventBridgeInvocationParameters](#eventbridgeinvocationparameters)|`EventBridgeInvocationParameters`|No| |

## EventBridgeInvocationParameters
Contains the header, body or query`string` parameters
| Field | Description | DataType | Required | Default |
|-------|-------------|----------|----------|---------|
|`headerParameters`|Name of the api key |`string`|Yes| |
|`bodyParameters`|Value of the api key |`string`|Yes| |
|`query`string`Parameters`|The parameters to use for invoking the resource endpoint. See [EventBridgeParameter](#eventbridgeparameter)|`EventBridgeInvocationParameters`|Yes| |

## EventBridgeParameter
Represents a single header, body or query `string` parameters
| Field | Description | DataType | Required | Default |
|-------|-------------|----------|----------|---------|
|`key`|Name of the key |`string`|Yes| |
|`value`|Value of the key. Read from a secrets manager secret, value supplied as `secretName:key` |`string`|Yes| |

Example:

```yaml 
resources:
  eventbridge-connection:
    test-${rand}-conn:
      description: connection for httbin service
      connectionParameters: 
          apiKeyName: Authorization
          apiKeyValue: example/eventbridge/api_connection/parameters:api_key_1  #friendlyname:key
          invocationParameters:
            headerParameters:
              - key: header1  ## key becomes http header name during invocation so only valid characters are accepted, else the header is not added..
                secret: "example/eventbridge/api_connection/parameters:h1"
              - key: header2
                value: header_value_2
            bodyParameters:
              - key: body_1
                secret: "example/eventbridge/api_connection/parameters:b1"
              - key: body_2
                value: body_value_2
            query`string`Parameters:
              - key: query_1
                secret: "example/eventbridge/api_connection/parameters:q1"
              - key: query_2
                value: query_value_2

```
