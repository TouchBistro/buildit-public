# Load Balancer `load-balancer` 

This resources creates a load balancer. 

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

Check out AWS documentation for load balancer [here](https://docs.aws.amazon.com/cli/latest/reference/elbv2/create-load-balancer.html). 


| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|Name (Resource Name)|The resource name is used as the name of the load balancer. Cannot be longer than 32 characters|`string`|Yes||
|`subnetNames`|The names of the subnets. You can specify only one subnet per Availability Zone. Must provide at least two|`[]string`|Yes||
|`securityGroupNames`|The names of the security groups for the load balancer|`[]string`|Yes||
|`scheme`|The scheme for the load balancer. Valid values: `internal`,`internet-facing`|`string`|No|`internet-facing`|
|`type`|The type of the load balancer. Valid values: `application`,`network`|`string`|No|`application`|
|`listeners`|The listeners attached to the load balancer. See [LBListener](./load_balancer_listener.md) section for more details. |`[]LBListener`|No|`[]`|
|`attributes`|The attributes of the load balancer. See [Attributes](#attributes) section for more details. |`map[string]string`|No|`{}`|
|`tags`|A key value map of resource tags to be applied to this resource. The `GlobalTags` are always applied, any matching keys are overriden from `tags`|`map[string]string`|No|`{}`|
|`dependsOn`|The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this|`[]string`|No|`[]`|

## Attributes
> Attributes differs based on the type of load balancer

| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|`access_logs.s3.enabled`|Indicates whether access logs are enabled |`string`|No|`false`|
|`access_logs.s3.prefix`|The prefix for the location in the S3 bucket for the access logs |`string`|No|`""`|
|`access_logs.s3.bucket`|The name of the S3 bucket for the access logs. This attribute is required if access logs are enabled |`string`|No|`""`|
|`deletion_protection.enabled`|Indicates whether deletion protection is enabled |`string`|No|`false` for `network`, `true` for `application`|
|`load_balancing.cross_zone.enabled`|Indicates whether cross-zone load balancing is enabled |`string`|No|`false` for `network`, `true` for `application`|
|`idle_timeout.timeout_seconds`|Only for `application` type. The idle timeout value, in seconds. The valid range is 1-4000 seconds |`string`|No|`60`|
|`client_keep_alive.seconds`|Only for `application` type. The client keep alive value, in seconds. The valid range is 60-604800 seconds |`string`|No|`3600`|
|`routing.http.desync_mitigation_mode`|Only for `application` type. Determines how the load balancer handles requests that might pose a security risk to your application. The possible values are `monitor` , `defensive` , and `strictest` |`string`|No|`defensive`|
|`routing.http.drop_invalid_header_fields.enabled`|Only for `application` type. Indicates whether HTTP headers with invalid header fields are removed by the load balancer (`true`) or routed to targets (`false`) |`string`|No|`false`|
|`routing.http.preserve_host_header.enabled`|Only for `application` type. Indicates whether the Application Load Balancer should preserve the Host header in the HTTP request and send it to the target without any change |`string`|No|`false`|
|`routing.http.x_amzn_tls_version_and_cipher_suite.enabled`|Only for `application` type. Indicates whether the two headers (`x-amzn-tls-version` and `x-amzn-tls-cipher-suite` ), are added to the client request before sending it to the target |`string`|No|`false`|
|`routing.http.xff_client_port.enabled`|Only for `application` type. Indicates whether the X-Forwarded-For header should preserve the source port that the client used to connect to the load balance |`string`|No|`false`|
|`routing.http.xff_header_processing.mode`|Only for `application` type. Enables you to modify, preserve, or remove the X-Forwarded-For header in the HTTP request before the Application Load Balancer sends the request to the target. Valid values: `append`, `preserve`, `remove` |`string`|No|`append`|
|`routing.http2.enabled`|Only for `application` type. Indicates whether HTTP/2 is enabled |`string`|No|`true`|
|`waf.fail_open.enabled`|Only for `application` type. Indicates whether to allow a WAF-enabled load balancer to route requests to targets if it is unable to forward the request to WAF |`string`|No|`false`|
|`connection_logs.s3.prefix`|Only for `application` type. The prefix for the location in the S3 bucket for the connection logs |`string`|No|`""`|
|`connection_logs.s3.enabled`|Only for `application` type. Indicates whether connection logs are enabled |`string`|No|`false`|
|`connection_logs.s3.bucket`|Only for `application` type. The name of the S3 bucket for the connection logs. This attribute is required if connection logs are enabled |`string`|No|`""`|
|`dns_record.client_routing_policy`|Only for `network` type. Indicates how traffic is distributed among the load balancer Availability Zones. Valid values: `availability_zone_affinity`, `partial_availability_zone_affinity`, `any_availability_zone`|`string`|No|`any_availability_zone`|


Attributes of the load balancer. Check out AWS documentation for load balancer attributes [here](https://docs.aws.amazon.com/cli/latest/reference/elbv2/modify-load-balancer-attributes.html). 

Example:

```yaml 

resources:
  load-balancer:
    example-web-lb:
      attributes:
        access_logs.s3.enabled: true
        access_logs.s3.prefix: example-web-lb
        access_logs.s3.bucket: core-elb-access-logs
      subnetNames:
        - example-public-0-subnet
        - example-public-1-subnet
        - example-public-2-subnet
      securityGroupNames:
        - example-web-lb-sg
      listeners:
        - name: http:80
          protocol: HTTP
          port: 80
          ifNoRulesMatch:
            then:
              - do: redirect-https
        - name: http:443
          protocol: HTTPS
          port: 443
          certificates:
            - my.example.com
          ifNoRulesMatch:
            then:
              - do: fixed-response
                fixedContentType: "text/plain"
                fixedMessageBody: "Target not found"
                fixedStatusCode: 404
          rules:
            - if:
                - the: host-header
                  is:
                    - my.example.com
                - the: path-pattern
                  is:
                    - /*
              then:
                - do: forward
                  forwardStickiness: 0
                  forwardTargetGroups:
                    - name: example-web-waf-tg
                      weight: 100
```

