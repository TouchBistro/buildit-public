# Route53 Record `route53-record` 

Create, update or destroy a Route53 DNS record in a supplied Hosted Zone. 

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name. 

The following DNS record types are supported `A`,`AAAA`, `CNAME`, `CAA`, `MX`, `NS`, `SOA` & `TXT`.

Route53 Alias record-sets for **Load Balancer** or **Cloudfront distribution** targets are also supported.

| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|Name (Resource Name)|The resource name is used as the DNS record name. This can be qualified with the provider name using the `provider::name` syntax. The fully-qualified domain name for this record can be supplied here as well. The hosted zone's domain name part will be stripped off. |`string`|Yes||
|`recordName`| An optional field indicating the DNS name of the record. When supplied, this value is used instead of the route54-record's resource name. *Optionally the provider name can be prefixed here using `provider/name` syntax for legacy support* |`string`|No|`<nil>`|
|`hostedZone`| A required field that specifies the name of the hosted zone for this record. |`string`|Yes||
|`recordType`| The DNS record type, Allowed values are `A`,`AAAA`, `CNAME`, `CAA`, `MX`, `NS`, `SOA` & `TXT`.|`string`|No|`A`|
|`aliasType`| Alias type specifies if this record is an alias. Allowed values are `load-balancer` or `cloudfront-distribution`. For `load-balancer` alias, the `destination` must contain only a single destination with the value in the format `provider/load-balancer-name`. For `cloudfront-distribution` the destination must include the Cloudfront distribution's domain name. |`string`|No|`<nil>`|
|`routingPolicy`| An optional routing policy for this record. For more detail see [Routing Policy](#routing-policy-routingpolicy)|`routingPolicy{}`|No|`<nil>`|
|`destinations`| A list of values for this DNS record. Multiple values can be supplied for most record types. For `alias` records, only a single value is allowed.  |`[]string`|Yes||
|`dependsOn`|The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this|`[]string`|No|`[]`|



## Routing Policy `routingPolicy`
This section supplies a routing policy for the record. Only `weighted` routing is supported. 

| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|`weight`| Supply the relative weight value (`0`-`255`) for this record. |`int`|Yes||
|`identiier`| A required field that uniquely identifies a weighted record for the same fqdn (name & domain name) |`string`|Yes||

## Important Note:

> `buildit` will update a route53 record in place where possible. However, when the `recordType` is modified, for example from `A` to `CNAME`, `buildit` will first **destroy**, then create the new record. Similarly when `routingPolicy` is updated, i.e. from `simple` to `weighted` or vice-versa, the existing record(s) will be **destroyed** before the new ones are created. This can cause a momentary `NXDOMAIN` (Non-existent domain) DNS answer for that particular record.

### Example 1: 

This example shows how the record name can be supplied in various ways. The examples are in order of recommendation, from hightest to least.

```yaml 
 route53-record:
   production::test.example.com:
     hostedZone: example.com
     recordType: A
     destinations:
       - 10.10.0.1
       - 10.10.0.2
```

is the same as:

```yaml 
 route53-record:
   production::test:
     hostedZone: example.com
     recordType: A
     destinations:
       - 10.10.0.1
       - 10.10.0.2
```
or 

```yaml
 route53-record:
   test.example.com:
     provider: production
     hostedZone: example.com
     recordType: A
     destinations:
       - 10.10.0.1
       - 10.10.0.2
```

or the legacy ways, which is not recommended for use.

```yaml
 route53-record:
   test_1:
     recordName: produciton/test
     hostedZone: example.com
     recordType: A
     destinations:
       - 10.10.0.1
       - 10.10.0.2
```
and

```yaml 
   production/test2:
     hostedZone: example.com
     recordType: A
     destinations:
       - 10.10.0.1
       - 10.10.0.2
```

Example 2:

Shows an alias & weighted policy distributing DNS traffic to a load balancer & a cloudfront distribution

```yaml
  route53-record:
   production::test.example.com_1:
      recordName: test.example.com
      hostedZone: example.net
      aliasType: load-balancer
      routingPolicy:
        weight: 1
        identifier: test_1
      destinations:
      - test/test-lb 
      dependsOn:
      - test::test-lb

   production::test.example.com_2:
      recordName: test.example.com
      hostedZone: example.net
      aliasType: cloudfront-distribution
      routingPolicy:
        weight: 1
        identifier: test_2
      destinations:
      - d123xxxxxxxx.cloudfront.net 
      dependsOn: []
```