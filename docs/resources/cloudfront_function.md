# CloudFront Function `cloudfront-function`

Create, update, or destroy an [AWS CloudFront Function](https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/cloudfront-functions.html) — a lightweight JavaScript function that runs at CloudFront edge locations on viewer requests/responses.

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

Check out AWS documentation for CloudFront Functions [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/cloudfront/create-function.html).

The resource name is the function **Name** (unique per AWS account, max 64 characters of `a-zA-Z0-9-_`). Applying always publishes the function to the **LIVE** stage, so the configuration describes what is actually serving traffic — a distribution's `functionAssociations` can only reference LIVE functions. A [`cloudfront-distribution`](./cloudfront_distribution.md) in the same config can reference the function by name; add the function to the distribution's `dependsOn` so it is created first.

`Normalize` defaults `runtime` to `cloudfront-js-2.0`.

## Top-level fields

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| `Name (Resource Name)` | The function name (unique per account) | `string` | Yes | |
| `comment` | Description of the function | `string` | No | `""` |
| `runtime` | Function runtime: `cloudfront-js-1.0` \| `cloudfront-js-2.0` | `string` | No | `cloudfront-js-2.0` |
| `code` | The JavaScript function code, inline (max 10 KB) | `string` | Yes | |
| `tags` | Tags to apply to the function; merged with `globalTags` (resource tags win on conflict) | `map[string]string` | No | `{}` |
| `dependsOn` | Resources this function depends on | `[]Key` | No | |

## Example

```yaml
resources:
  cloudfront-function:
    my-viewer-request-fn:
      comment: adds security headers
      runtime: cloudfront-js-2.0
      tags:
        team: example-team
      code: |
        function handler(event) {
          var request = event.request;
          request.headers['x-custom-header'] = { value: 'my-value' };
          return request;
        }

  cloudfront-distribution:
    my-cdn:
      # ...
      defaultCacheBehavior:
        targetOriginId: primary
        functionAssociations:
          - eventType: viewer-request
            functionARN: my-viewer-request-fn # referenced by name; resolved to the ARN
      dependsOn:
        - my-viewer-request-fn
```

## Known limitations

- **Code is inline-only.** The CloudFront API accepts function code only as an inline blob (10 KB max) — there is no S3 option, unlike Lambda. Use buildit's `--templates`/`--include` interpolation to compose code from files if needed.
- **Always published.** Every create/update publishes the DEVELOPMENT stage to LIVE; there is no way to stage unpublished changes through this resource.
- **KeyValueStore associations are not supported.**
- **Tags-only changes do not republish.** Tag updates are applied via the tagging API without touching the function code or its LIVE stage.
- **Deletion fails while associated.** CloudFront refuses to delete a function still associated with a distribution; remove the association first.
