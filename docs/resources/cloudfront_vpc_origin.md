# CloudFront VPC Origin `cloudfront-vpc-origin`

Creates an [AWS CloudFront VPC origin](https://docs.aws.amazon.com/AmazonCloudFront/latest/APIReference/API_CreateVpcOrigin.html) — a private connection that lets a CloudFront distribution route traffic to an ALB/NLB inside a VPC without exposing the load balancer to the internet.

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

The buildit **resource name** is used as the VPC origin's `Name`. VPC origins have no name-based lookup API, so buildit matches an existing VPC origin by listing and comparing names; the resource name must remain stable across runs (renaming it is treated as a different VPC origin).

A `cloudfront-distribution` references a VPC origin through an `origins` entry of `type: vpc` whose `target` is the VPC origin's name or id (`vo_...`). Use `dependsOn` on the distribution so the VPC origin is applied first. Deletes are rejected by AWS while any distribution still references the VPC origin.

Creates and updates wait inline for the VPC origin to reach the `Deployed` state (typically a few minutes, up to a 20-minute timeout). The wait cannot be deferred: AWS refuses to associate a still-deploying VPC origin with a distribution, so dependents must observe the `Deployed` state.

## Top-level fields

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| Name (Resource Name) | The buildit resource name; used as the VPC origin `Name` | `string` | Yes | |
| `target` | ARN or name of the ALB/NLB the VPC origin points at. A buildit `load-balancer` resource name works directly (buildit load balancer names are the AWS names) — add it to `dependsOn` so it is created first. May carry a `provider::` prefix to resolve the load balancer via a different provider | `string` | Yes | |
| `httpPort` | HTTP port of the endpoint | `int32` | No | `80` |
| `httpsPort` | HTTPS port of the endpoint | `int32` | No | `443` |
| `originProtocolPolicy` | `http-only` \| `https-only` \| `match-viewer` | `string` | No | `https-only` |
| `originSslProtocol` | SSL/TLS protocol for origin connections: `SSLv3` \| `TLSv1` \| `TLSv1.1` \| `TLSv1.2` (case-insensitive). AWS accepts exactly one protocol for VPC origins | `string` | No | `TLSv1.2` |
| `tags` | Tags to apply to the VPC origin | `map[string]string` | No | |
| `dependsOn` | Resource dependencies | `[]string` | No | |

## Example

```yaml
resources:
  load-balancer:
    internal-alb:
      # ... internal ALB definition ...

  cloudfront-vpc-origin:
    my-internal-alb-origin:
      # references the load-balancer resource above by name
      target: internal-alb
      dependsOn:
        - internal-alb
      httpPort: 80
      httpsPort: 443
      originProtocolPolicy: https-only
      originSslProtocol: TLSv1.2
      tags:
        Environment: production

  cloudfront-distribution:
    my-distribution:
      dependsOn:
        - my-internal-alb-origin
      origins:
        - name: primary
          type: vpc
          target: my-internal-alb-origin
      defaultCacheBehavior:
        targetOriginId: primary
```
