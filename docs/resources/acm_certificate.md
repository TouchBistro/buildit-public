# ACM Certificate `certificate`

Requests an ACM certificate for use with other Amazon Web Services services. Check out AWS documentation for ACM certificate [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/acm/request-certificate.html).

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

> The resource name is the certificate's domain name, and it is also written to the built-in
> `buildit:resource-id` tag. AWS does not allow `*` in a tag value, so a wildcard domain is recorded
> with the `*` replaced by `_` — a certificate named `*.example.com` carries
> `buildit:resource-id: _.example.com`. See [Reserved tag keys](../config.md#reserved-tag-keys).

| Field | Description | DataType | Required | Default |
|-------|-------------|----------|----------|---------|
| `san` | List of subject alternative names for the CSR | `string` | No |  |
| `dnsValidationDomainName` | Domain to use for validation in `[provider/]domain-name` format | `string` | Yes |  |
|`tags`|A key value map of resource tags to be applied to this resource. The `GlobalTags` are always applied, any matching keys are overriden from `tags`|`map[string]string`|No|`{}`|
|`dependsOn`|The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this|`[]string`|No|`[]`|
Example:
```yaml
resources:
  certificate:
    api.example.io:
      san:
        - api2.example.io
      dnsValidationDomainName: default/example.io
```
