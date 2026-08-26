# CloudFront Distribution `cloudfront-distribution`

Creates an [AWS CloudFront distribution](https://docs.aws.amazon.com/AmazonCloudFront/latest/APIReference/API_CreateDistribution.html).

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

The buildit **resource name** is used as the distribution's `CallerReference` — an immutable, unique identifier. buildit matches an existing distribution by `CallerReference`, so the resource name must remain stable across runs (renaming it is treated as a different distribution). It is limited to 128 characters.

`Normalize` applies opinionated defaults (mirroring wafer): HTTPS-only viewer protocol, the AWS-managed `CachingDisabled` cache policy and `AllViewerAndCloudFrontHeaders-2022-06` origin-request policy, the full method set with `GET,HEAD` cached, `PriceClass_100`, `http2and3`, `TLSv1.2_2021` / `sni-only`. Override any of them explicitly.

All enum-valued fields (`viewerProtocolPolicy`, `priceClass`, `httpVersion`, HTTP methods, origin `type`, etc.) are **case-insensitive**: buildit normalizes them to the exact constants CloudFront expects.

Plan/apply diff output renders resolved identifiers as the values you wrote in YAML (web ACL names, managed policy names, origin targets, function names) rather than the underlying ARNs/ids; raw values are shown only where a name would be ambiguous (e.g. a web ACL replaced by another with the same name).

## Top-level fields

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| Name (Resource Name) | The buildit resource name; used as the distribution `CallerReference` (≤128 chars) | `string` | Yes | |
| `comment` | Distribution comment | `string` | No | `""` |
| `enabled` | Whether the distribution is enabled | `bool` | No | `true` |
| `defaultRootObject` | Object returned for requests to the root URL | `string` | No | `""` |
| `aliases` | Alternate domain names (CNAMEs). Require `certificate` | `[]string` | No | |
| `certificate` | ACM certificate ARN, certificate id (UUID), **or** domain name (resolved like load balancer listeners). Must be in us-east-1. If unset, the default CloudFront certificate is used | `string` | No | default CloudFront cert |
| `minimumProtocolVersion` | Minimum TLS version (used with a custom `certificate`) | `string` | No | `TLSv1.2_2021` |
| `sslSupportMethod` | `sni-only` \| `vip` \| `static-ip` (used with a custom `certificate`) | `string` | No | `sni-only` |
| `httpVersion` | `http1.1` \| `http2` \| `http3` \| `http2and3` | `string` | No | `http2and3` |
| `isIPV6Enabled` | Enable IPv6 for the distribution | `bool` | No | `false` |
| `priceClass` | `PriceClass_100` \| `PriceClass_200` \| `PriceClass_All` | `string` | No | `PriceClass_100` |
| `webAclName` | Name of the **global** (CloudFront-scope) WAFv2 web ACL to associate, resolved to its ARN via the WAFv2 API (always us-east-1). Empty or unset means no web ACL (an existing association is removed) | `string` | No | |
| `logging` | Access-logging configuration | `object` | No | |
| `origins` | Origins (at least one required) | `[]object` | Yes | |
| `defaultCacheBehavior` | Default cache behavior | `object` | Yes | |
| `customErrorResponses` | Custom error responses | `[]object` | No | |
| `tags` | Tags to apply to the distribution | `map[string]string` | No | |
| `dependsOn` | Resource dependencies | `[]string` | No | |

### `logging`

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| `bucket` | S3 bucket domain, e.g. `my-logs.s3.amazonaws.com` | `string` | Yes (when `logging` set) | |
| `prefix` | Log object prefix | `string` | No | `""` |
| `includeCookies` | Include cookies in logs | `bool` | No | `false` |
| `enabled` | Enable logging | `bool` | No | `true` |

### `origins[]`

An origin is a generic `(type, target)` pair. There is **no `domainName` field** — the domain is always inferred from the target based on the type:

- `vpc` — `target` is a VPC origin id (`vo_...`) or its Name. The domain is the DNS name of the load balancer the VPC origin points at. Only ALB/NLB-backed VPC origins are supported (not EC2-instance endpoints).
- `s3` — `target` is a bucket name (the domain becomes the bucket's regional S3 endpoint, e.g. `my-bucket.s3.us-east-1.amazonaws.com`) or a full `*.amazonaws.com` endpoint used as-is. Access is expected via an origin access control (`originAccessControlId`); the legacy origin access identity is not supported.
- `custom` — `target` is an (internet-facing) load balancer name resolved to its DNS name, or a literal domain name (anything containing a dot) used as-is.

The target may carry a `provider::` prefix (e.g. `uswest::my-cell2-vo`) selecting the provider whose regional clients resolve the underlying load balancer or bucket — useful when a VPC origin's ALB lives in a different region than the distribution's provider.

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| `name` | Unique origin id (referenced by `defaultCacheBehavior.targetOriginId`) | `string` | Yes | |
| `type` | `vpc` \| `s3` \| `custom` | `string` | Yes | |
| `target` | Type-specific identifier the origin domain is inferred from (see above) | `string` | Yes | |
| `path` | Path appended to origin requests | `string` | No | `""` |
| `customHeaders` | Custom headers sent to the origin | `map[string]string` | No | |
| `connectionAttempts` | Connection attempts (1–3); all types | `int32` | No | `3` |
| `connectionTimeout` | Connection timeout in seconds (1–10); all types | `int32` | No | `10` |
| `originReadTimeout` | Response timeout in seconds (1–60); `vpc`/`custom` only | `int32` | No | `30` |
| `originKeepAliveTimeout` | Keep-alive timeout in seconds (1–60); `vpc`/`custom` only | `int32` | No | `5` |
| `responseCompletionTimeout` | Total response completion timeout in seconds (must be ≥ `originReadTimeout`); all types | `int32` | No | unset (disabled) |
| `originAccessControlId` | Origin Access Control id | `string` | No | |
| `httpPort` | Origin HTTP port; `custom` only | `int32` | No | `80` |
| `httpsPort` | Origin HTTPS port; `custom` only | `int32` | No | `443` |
| `originProtocolPolicy` | `http-only` \| `https-only` \| `match-viewer`; `custom` only | `string` | No | `https-only` |
| `originSslProtocols` | SSL protocols for origin connections; `custom` only | `[]string` | No | `["TLSv1.2"]` |

### `defaultCacheBehavior`

Only the default cache behavior is managed. Ordered (path-pattern) cache behaviors and origin groups are **not supported**; any that already exist on the distribution are preserved untouched on update.

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| `targetOriginId` | Origin `name` to route to (must match an origin) | `string` | Yes | |
| `viewerProtocolPolicy` | `allow-all` \| `https-only` \| `redirect-to-https` | `string` | No | `https-only` |
| `allowedMethods` | Allowed HTTP methods; must be exactly `GET,HEAD`, `GET,HEAD,OPTIONS`, or all seven methods (any order; validated at plan time) | `[]string` | No | `GET,HEAD,OPTIONS,PUT,POST,PATCH,DELETE` |
| `cachedMethods` | Cached HTTP methods; must be exactly `GET,HEAD` or `GET,HEAD,OPTIONS`, and a subset of `allowedMethods` (validated at plan time) | `[]string` | No | `GET,HEAD` |
| `cachePolicyId` | Cache policy id, managed policy name, or custom policy name (see below) | `string` | No | `CachingDisabled` (managed) |
| `originRequestPolicyId` | Origin request policy id, managed policy name, or custom policy name (see below) | `string` | No | `AllViewerAndCloudFrontHeaders-2022-06` (managed); `CORS-S3Origin` (managed) when the target origin is `s3` — AWS rejects Host-header-forwarding policies on S3 targets |
| `responseHeadersPolicyId` | Response headers policy id, managed policy name, or custom policy name (see below) | `string` | No | |
| `compress` | Enable automatic compression | `bool` | No | `false` |
| `functionAssociations` | CloudFront Function associations | `[]object` | No | |

**Policy identifiers** (`cachePolicyId`, `originRequestPolicyId`, `responseHeadersPolicyId`) accept, in order of resolution:

1. a **policy id** (UUID), used as-is;
2. an **AWS-managed policy name** as documented for [cache](https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/using-managed-cache-policies.html), [origin request](https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/using-managed-origin-request-policies.html), and [response headers](https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/using-managed-response-headers-policies.html) policies — e.g. `CachingOptimized`, `AllViewer`, `SecurityHeadersPolicy`. Names are case-insensitive and the `Managed-` prefix is optional; they resolve to the well-known ids at plan time with no AWS call;
3. anything else is treated as a **custom policy name** and resolved via the CloudFront API at compare/apply time (case-insensitive; must match exactly one policy).

**`functionAssociations[]`**: `eventType` (`viewer-request` \| `viewer-response`), `functionARN` (the CloudFront Function **ARN or its Name**, resolved to the ARN via the CloudFront API — same identifier pattern as `certificate` / origin `target`). The function must already exist; this resource does not create CloudFront Functions.

### `customErrorResponses[]`

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| `errorCode` | HTTP error code from the origin | `int32` | Yes | |
| `responseCode` | HTTP code returned to the viewer | `string` | No | |
| `responsePagePath` | Path to the custom error page | `string` | No | |
| `errorCachingMinTTL` | Minimum seconds to cache the error | `int64` | No | |

## Example

```yaml
resources:
  cloudfront-distribution:
    my-cdn:
      comment: "CDN for my-service"
      aliases:
        - cdn.example.com
      certificate: arn:aws:acm:us-east-1:111122223333:certificate/abc-123
      webAclName: my-acl # global (CloudFront-scope) web ACL Name; resolved to its ARN
      logging:
        bucket: my-logs.s3.amazonaws.com
        prefix: cloudfront/my-cdn
      origins:
        # VPC origin: domain inferred from the load balancer the VPC origin points at
        - name: primary-cell
          type: vpc
          target: my-service-cell1-vo # VPC origin Name or vo_... id
          customHeaders:
            x-example-stack: my-service
        # VPC origin in another region: provider prefix picks the clients used to
        # resolve the underlying load balancer
        - name: secondary-cell
          type: vpc
          target: uswest::my-service-cell2-vo
        # S3 origin: domain inferred from the bucket's region
        - name: static-assets
          type: s3
          target: my-assets-bucket
          originAccessControlId: E2QWRUHAPOMQZL
      defaultCacheBehavior:
        targetOriginId: primary-cell
        viewerProtocolPolicy: redirect-to-https # case-insensitive
        allowedMethods: [GET, HEAD, OPTIONS]
        cachedMethods: [GET, HEAD]
        cachePolicyId: CachingOptimized # managed policy name or id
        originRequestPolicyId: AllViewer # managed policy name or id
        responseHeadersPolicyId: SecurityHeadersPolicy # managed policy name or id
        functionAssociations:
          - eventType: viewer-request
            functionARN: my-vreq-fn # Name or ARN
      customErrorResponses:
        - errorCode: 404
          responseCode: "200"
          responsePagePath: /index.html
          errorCachingMinTTL: 10
      tags:
        Name: my-cdn
```

## Known limitations

- **Ordered cache behaviors & origin groups are not managed.** Only `defaultCacheBehavior` is configurable; any ordered behaviors or origin groups already on the distribution are preserved untouched on update.
- **Lookup cost:** matching an existing distribution lists all distributions in the account and reads each one's config to compare `CallerReference` (CloudFront's `DistributionSummary` does not expose it, and buildit persists no state). In accounts with many distributions this is rate-limited; a per-run cache may be added later.
- **No output references:** the created distribution's ARN/domain name are not surfaced for other resources in the same run. Route53 alias records to a CloudFront distribution take the distribution's domain name directly.
- **CloudFront Functions** are not managed by this resource; reference existing function ARNs via `functionAssociations`.
