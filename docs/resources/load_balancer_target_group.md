# Load Balancer Target Group `lb-targetgroup` 

This resources creates a load balancer target group. 

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

Check out AWS documentation for load balancer target group [here](https://docs.aws.amazon.com/cli/latest/reference/elbv2/create-target-group.html) and its attributes [here](https://docs.aws.amazon.com/cli/latest/reference/elbv2/modify-target-group-attributes.html). 


| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|Name (Resource Name)|The resource name is used as the name of the target group|`string`|Yes||
|`vpc`|The name of the target VPC|`string`|Yes||
|`protocol`|The protocol to use for routing traffic to the targets. Valid values: `HTTP`, `HTTPS`, `TLS`, `TCP`, `UDP`, `TCP_UDP`|`string`|Yes||
|`port`|The port on which the targets receive traffic|`int32`|No||
|`targetType`|The type of target. Valid values: `instance`, `ip` |`string`|Yes||
|`healthcheck`|Represents an load balancer healtcheck setup. See [LBHealthCheck](#lbhealthcheck) section for more details. Default to `/ping` setup |`[]LBHealthCheck`|No|`LBHealthCheck{ 30, "/ping", "traffic-port", HTTP, 5, 5, 2, "200",}`|
|`sticky`|Indicate if stickiness is enabled|`bool`|No|`false`|
|`algorithm`|Determines how the load balancer selects targets when routing requests. Valid values: `round_robin`, `least_outstanding_requests`, or `weighted_random`|`string`|No|`least_outstanding_requests`|
|`deregistrationDelay`|The amount of time, in seconds, for Elastic Load Balancing to wait before changing the state of a deregistering target from `draining` to `unused`|`int64`|No|`300`|
|`targets`|Specify only when `targetType` is `ip`. List of targets in this group. Specified in format `ip:port@region`|`[]string`|No||
|`tags`|A key value map of resource tags to be applied to this resource. The `GlobalTags` are always applied, any matching keys are overriden from `tags`|`map[string]string`|No|`{}`|
|`dependsOn`|The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this|`[]string`|No|`[]`|

## LBHealthCheck
Represents an load balancer healtcheck setup.

| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|`interval`|The approximate amount of time, in seconds, between health checks of an individual target. The range is 5-300 |`int32`|Yes||
|`path`|The destination for health checks on the targets |`string`|Yes||
|`port`|The port the load balancer uses when performing health checks on targets |`string`|Yes||
|`protocol`|The protocol the load balancer uses when performing health checks on targets. Valid values: `HTTP`, `HTTPS`, `TLS`, `TCP`, `UDP`, `TCP_UDP` |`string`|Yes||
|`timeout`|The amount of time, in seconds, during which no response from a target means a failed health check |`int32`|Yes||
|`healthyCount`|The number of consecutive health check successes required before considering a target healthy |`int32`|Yes||
|`unhealthyCount`|The number of consecutive health check failures required before considering a target unhealthy |`int32`|Yes||
|`successHttpCodes`|Code to use when checking for a successful response from a target |`string`|Yes||

Example:

```yaml 
resources:
  lb-targetgroup:
    example-core-api-compute-waf-tg:
      vpc: example-vpc
      protocol: HTTP
      port: 80
      targetType: ip
      healthcheck:
        path: /healthcheck
        port: "traffic-port"
        protocol: HTTP
        interval: 15
        timeout: 5
        healthyCount: 2
        unhealthyCount: 8
        successHttpCodes: "200"
```