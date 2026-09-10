# ACM Certificate `certificate`

Requests an ACM certificate for use with other Amazon Web Services services. Check out AWS documentation for ACM certificate [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/acm/request-certificate.html).

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

> The resource name is the certificate's identity: it is written to the built-in
> `buildit:resource-id` tag, and it doubles as the certificate's domain name unless a separate
> `domainName` is supplied. AWS does not allow `*` in a tag value, so a wildcard value is recorded
> with the `*` replaced by `_` — a certificate named `*.example.com` carries
> `buildit:resource-id: _.example.com`. See [Reserved tag keys](../config.md#reserved-tag-keys).

> **Same-CN twins.** Because the resource name and the domain are separate, two certificates with
> the same domain can live in one config scope under distinct names (same pattern as
> [`route53-record`](./route53_record.md)'s `recordName`). Each twin's `buildit:resource-id` is its
> own resource name, which is how their lifecycles stay apart. Note that a certificate's domain is
> immutable: changing `domainName` under the same resource name fails the plan — destroy the
> certificate first, or rename the resource.

> **Lookup.** A domain name is not unique in ACM — several certificates can share a CN. Everywhere
> buildit resolves a certificate (this resource's own compare/destroy, a
> [`cloudfront-distribution`](./cloudfront_distribution.md) `certificate` reference, a
> [`load-balancer` listener](./load_balancer_listener.md) `certificates` entry), a full ARN is used
> as-is, a certificate id (UUID) matches exactly, and anything else is matched as a domain name
> against all certificates in the account — when several match, the one carrying the referenced
> `buildit:resource-id` tag wins, and when none match as a domain, the identifier is tried as a
> buildit resource name via that tag. A collision the tag cannot settle (none tagged, or several
> tagged alike) fails the lookup rather than guess — manage the certificate with buildit, tag it,
> or reference it by ARN, certificate id, or its buildit resource name.

| Field | Description | DataType | Required | Default |
|-------|-------------|----------|----------|---------|
| `domainName` | The certificate's domain name (CN). Supply it when the resource name should differ from the domain — required to define two certificates with the same CN in one scope | `string` | No | Resource Name |
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

Example: two certificates sharing a CN, distinguished by resource name. Consumers reference each by
its resource name (e.g. a listener's `certificates: [example-api-cert-blue]`).
```yaml
resources:
  certificate:
    example-api-cert-blue:
      domainName: api.example.com
      dnsValidationDomainName: default/example.com
    example-api-cert-green:
      domainName: api.example.com
      dnsValidationDomainName: default/example.com
```
