# Security Group `security-group` 

This resources creates a security group. 

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

Check out AWS documentation for security group [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/ec2/create-security-group.html) and [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/ec2/modify-security-group-rules.html). 


| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|Name (Resource Name)|The resource name is used as the name of the security group|`string`|Yes||
|`description`|A description for the security group|`string`|No||
|`vpcName`|The name of the VPC|`string`|Yes||
|`inboundRules`|A list of inbound security group rule. More details at [SecurityGroupRule](#securitygrouprule)|`[]SecurityGroupRule`|No||
|`outboundRules`|A list of outbound security group rule. More details at [SecurityGroupRule](#securitygrouprule)|`[]SecurityGroupRule`|No||
|`tags`|A key value map of resource tags to be applied to this resource. The `GlobalTags` are always applied, any matching keys are overriden from `tags`|`map[string]string`|No|`{}`|
|`dependsOn`|The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this|`[]string`|No|`[]`|

## SecurityGroupRule
Represents a security group rule. 
> Only the first instance of duplicated security group rules is applied

| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|`ipProtocol`|The IP protocol name `tcp`, `udp`, `icmp`, `icmpv6` or number (see [Protocol Numbers](https://www.iana.org/assignments/protocol-numbers/protocol-numbers.xhtml)). Use `-1` to specify all protocols|`string`|Yes||
|`portRange`|The port range in format `n` or `n - m`|`string`|Yes||
|`cidrBlocks`|List of CIDR sources. Specify using a list key value pair: `Value` for the cidr block and `Description` for the description |`[]SecurityGroupRuleSource`|No||
|`securityGroups`|List of security group sources. Specify using a list key value pair: `Value` for the security group name and `Description` for the description |`[]SecurityGroupRuleSource`|No||

Example:

```yaml 
resources:
  security-group:  
    example-pos-api-lb-sg:
      description: security group for pos-api load balancer
      vpcName: example-vpc
      outboundRules:
        - portRange: 3000-3002
          ipProtocol: tcp
          securityGroups:
            - value: example-pos-api-svc-sg
              description: allow from example-pos-api load balancer              
      inboundRules:
        - portRange: 80
          ipProtocol: tcp
          cidrBlocks:
            - value: 0.0.0.0/0
              description: Allow http from the world
        - portRange: 443
          ipProtocol: tcp
          cidrBlocks:
            - value: 0.0.0.0/0
              description: allow https from the world
      tags:
        Name: example-pos-api-lb-sg
```
