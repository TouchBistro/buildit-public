# S3 Bucket `s3-bucket`

Creates an S3 bucket.

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

Check out AWS documentation for S3 Buckets [here](https://docs.aws.amazon.com/AmazonS3/latest/API/API_CreateBucket.html).

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| Name (Resource Name) | The buildit resource name | `string` | Yes | |
| `eventbridge` | Enable EventBridge notifications for the bucket | `bool` | No | `false` |
| `forceDestroy` | (Future) Force delete bucket by emptying it first | `bool` | No | `false` |
| `tags` | Tags to apply to the bucket | `map[string]string` | No | |

Example:
```yaml
resources:
  s3-bucket:
    # The resource name is used as the bucket name
    example-errors-bucket:
      forceDestroy: false
      eventbridge: true
      tags:
        Environment: production
```

