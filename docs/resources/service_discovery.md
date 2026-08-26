# Service Discovery `sd-service` 

This resources creates a cloudwatch subscription filter. 

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

Check out AWS documentation for cloudwatch subscription filter [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/servicediscovery/create-service.html). 

| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|Name (Resource Name)|The resource name is used as the name of the service discovery|`string`|Yes||
|`discoveryName`|The name that you want to assign to the service |`string`|Yes| |
|`description`|A description for the service |`string`|No| |
|`namespace`|The namespace that you want to use to create the service |`string`|Yes| |
|`routingPolicy`|The routing policy applied to all Route 53 DNS records that Cloud Map creates when an instance is registered. Valid values: `MULTIVALUE`, `WEIGHTED` |`string`|No|`MULTIVALUE` |
|`records`|A list that contains one DnsRecord object for each Route 53 DNS record that you want Cloud Map to create when you register an instance. See [SDDnsRecord](#sddnsrecord) |`[]SDDnsRecord`|No|`[{"type":"A","ttl":"0"}]` |
|`ttl`|`DEPRECATED`. If `records` is not provided. Create a `type` `A` record with this `ttl` |`int64`|No| |
|`tags`|A key value map of resource tags to be applied to this resource. The `GlobalTags` are always applied, any matching keys are overriden from `tags`|`map[string]string`|No|`{}`|
|`dependsOn`|The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this|`[]string`|No|`[]`|

## SDDnsRecord
Represents a DNS record
| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|`ttl`|The amount of time, in seconds, that you want DNS resolvers to cache the settings for this record |`string`|No| |
|`type`|Type of DNS record. Valid values: `A`, `AAAA`, `CNAME`, `SRV`  |`string`|Yes| |
|`values`|Optional static values to register in Cloud Map for this record type. Supported types are `A` (IPv4), `AAAA` (IPv6), and `CNAME` (DNS name). |`[]string`|No|`[]` |

Notes:
- If `type: CNAME` is used, `routingPolicy` must be `WEIGHTED`.
- `values` with `type: SRV` is currently not supported.

Example:

```yaml 
resources:
    example-email-sd:
      discoveryName: example-email.qa
      description: Service discovery for email qa service
      namespace: tb.example
      routingPolicy: WEIGHTED
      records:
        - type: A
          ttl: 0
```

Example with secondary CNAME alias:

```yaml
resources:
    primary-service-sd:
      discoveryName: service1
      namespace: tb.example
      records:
        - type: A
          ttl: 0

    secondary-alias-sd:
      discoveryName: alias2
      namespace: tb.example
      routingPolicy: WEIGHTED
      records:
        - type: CNAME
          ttl: 0
          values:
            - service1.tb.example
```
