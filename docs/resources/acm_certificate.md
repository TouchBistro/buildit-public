# ACM Certificate `certificate`

Requests an ACM certificate for use with other Amazon Web Services services. Check out AWS documentation for ACM certificate [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/acm/request-certificate.html).

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

> The resource name is the certificate's identity: it is written to the built-in
> `buildit:resource-id` tag, and it doubles as the certificate's domain name unless a separate
> `domainName` is supplied. AWS does not allow `*` in a tag value, so a wildcard value is recorded
> with the `*` replaced by `_` — a certificate named `*.example.com` carries
> `buildit:resource-id: _.example.com`. See [Reserved tag keys](../config.md#reserved-tag-keys).

> **Same-CN certificates.** Because the resource name and the domain are separate, several certificates with
> the same domain can live in one config scope under distinct names (same pattern as
> [`route53-record`](./route53_record.md)'s `recordName`). Each one's `buildit:resource-id` is its
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
| `dnsValidationDomainName` | One hosted zone that receives every DNS validation record, in `[provider/]zone` or `provider::zone` format. Omit it to have buildit discover the zone for every validation record — see [DNS validation](#dns-validation) | `string` | No | Discovered per record |
| `dnsValidationZones` | Hosted zone per domain: a map from the CN and each SAN, exactly as written above, to a `[provider/]zone` or `provider::zone`. Every domain on the certificate must be listed. Cannot be combined with `dnsValidationDomainName` — see [DNS validation](#dns-validation) | `map[string]string` | No | Discovered per record |
|`tags`|A key value map of resource tags to be applied to this resource. The `GlobalTags` are always applied, any matching keys are overriden from `tags`|`map[string]string`|No|`{}`|
|`dependsOn`|The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this|`[]string`|No|`[]`|
## DNS validation

ACM proves ownership of the CN and every SAN through a CNAME per domain
(`_<token>.<domain>`). buildit writes each of those records into a Route53 hosted zone and
waits for the certificate to be issued; on destroy it removes them again.

**Discovery (default).** With `dnsValidationDomainName` omitted, buildit lists the hosted zones
in the certificate's own provider once per run and, for each validation record, picks the
**public zone whose name is the longest suffix of the record**. That is the zone public DNS
resolves through, so a record for `uuid.service.example.com` lands in `service.example.com`
when that zone exists and in `example.com` otherwise. Resolution is per record, so a
certificate whose CN and SANs live in different zones (`api.example.com` + `api.example.io`)
validates fine. `plan` performs the same lookup for the CN and each SAN and fails, before
anything is requested from ACM, when a zone cannot be settled:

- **No public zone matches** — the zone may live in another account; set
  `dnsValidationDomainName` with a provider prefix (`dns::example.com`). Discovery only ever
  searches the certificate's own provider.
- **Only private zones match** — ACM validates over public DNS, so a private zone cannot
  complete validation; set `dnsValidationDomainName` to the public zone that owns the record.
- **Several public zones share the winning name** — ambiguous; the error lists the hosted zone
  ids. Set `dnsValidationDomainName` explicitly.

**Explicit zone.** When `dnsValidationDomainName` is set, every validation record (CN and all
SANs) is written to that one zone, in that provider, exactly as before discovery existed —
existing configs keep working unchanged. Two consequences follow: the value must be the hosted
zone that owns the record, not the CN (`example.com`, not `api.example.com`), and all domains
on the certificate must resolve under that single zone. A certificate spanning several zones
needs discovery (same provider) or `dnsValidationZones` (any provider).

**Explicit zone per domain.** `dnsValidationZones` names the hosted zone for each domain
individually: keys are the CN and SANs exactly as written on the certificate (a wildcard SAN is
keyed literally, `"*.api.example.com"`), values use the same `[provider/]zone` or
`provider::zone` format. Each record goes to its own zone in its own provider, so one
certificate can span zones in different accounts. The map is all-or-nothing: `plan` rejects a
map that misses a domain, names a domain that is not on the certificate, or is combined with
`dnsValidationDomainName` — a partial map would silently mix explicit placement with
discovery. Precedence per certificate is `dnsValidationZones`, then `dnsValidationDomainName`,
then discovery.

**Cross-provider zones.** Discovery never crosses providers: it searches the certificate's own
provider only. When the certificate's provider does not own the public zones (the common layout
where certificates live in a workload account and DNS lives in a separate account), name the
zone explicitly — `dnsValidationDomainName: dns::example.com` when one zone owns every domain,
`dnsValidationZones` when the CN and SANs need zones in **different** providers
(`api.example.com` in the `dns` provider, `api.example.io` in `other`). Leaving discovery to
find them fails the plan with "no hosted zone in this account".

Records already written for an issued certificate are never moved: switching an existing
certificate between an explicit zone and discovery only affects where records are created
or removed from that point on, so keep the placement stable for a certificate's lifetime.

Example: zone discovered per record.
```yaml
resources:
  certificate:
    api.example.com:
      san:
        - api2.example.com
        - api.example.io   # validated in the example.io zone
```

Example: explicit zone in another provider.
```yaml
resources:
  certificate:
    api.example.com:
      san:
        - api2.example.com
      dnsValidationDomainName: dns::example.com
```

Example: explicit zone per domain, across providers. Every domain on the certificate is listed.
```yaml
resources:
  certificate:
    api.example.com:
      san:
        - "*.api.example.com"
        - api.example.io
        - api.example.net
      dnsValidationZones:
        api.example.com: dns::example.com
        "*.api.example.com": dns::example.com   # same record as the CN; keyed as written
        api.example.io: other::example.io       # zone in a different provider
        api.example.net: example.net            # bare zone = the main provider
```

Example: two certificates sharing a CN, distinguished by resource name. Consumers reference each by
its resource name (e.g. a listener's `certificates: [example-api-cert-blue]`).
```yaml
resources:
  certificate:
    example-api-cert-blue:
      domainName: api.example.com
    example-api-cert-green:
      domainName: api.example.com
```
