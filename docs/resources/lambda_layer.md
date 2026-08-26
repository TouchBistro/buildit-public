
# Lambda Layer `lambda-layer` 

This resources creates & publishes a lambda layer using the `main` provider. 

Check out AWS documentation for lambda [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/lambda/index.html#description). 


> Since AWS does not provide an API endpoint to fetch the existing layer content/code, `buildit` cannot perform a diff between the supplied & existing layer content. Everytime there is a change in the layer attributes, or the `publish` flag is set, a new version for the layer is pushed.


| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|Name (Resource Name)|The resource name is used as the name of the lambda function|`string`|Yes||
|`description`|The description of the lambda layer |`string`|No|""|
|`code`|supply the content/code for the layer. see [Code](#code-code) section for more details. |`code{}`|Yes||
|`publish`| Forces a new layer version even if all the other attributes are the same. |`bool`|No|`false`|
|`compatibleArchitectures`|A list of all instruction-set architectures this layer would be compatible with. Values can be `arm64` & `x86_64`|`[]string`|No|`[]`|
|`comptabileRuntimes`|A list of all compatible runtimes that this layer is compatible with; See details of currently supported values [here](https://docs.aws.amazon.com/lambda/latest/dg/lambda-runtimes.html). |`[]string`|Yes||
|`dependsOn`|The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this|`[]string`|No|`[]`|


## Code `code`
This section configures how the content/code for the lambda layer is packaged and supplied. The location of the package can be supplied using the S3 bucket name, object key & version (optional). `buildit` only supports configuring lambda layer content using an S3 location. 

| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|`image`|The container image repository URI pointing to the image to be used as the code package when the `type` of the lambda is `Image`. This field is required when `type` attribute is `Image` |`string`|Yes*||
|`bucket`|The S3 bucket name for the `zip` package. Required when lambda function `type` attributes is `Zip` (not supported yet)|`string`|Yes*||
|`key`|The S3 key for the `zip` package. Required when lambda function `type` attributes is `Zip` (not supported yet)|`string`|No||
|`version`|The S3 version name for the `zip` package (not supported yet)|`string`|No||


Example: Lambda function with code provided as a Zip package from S3.

```yaml 

resources:
  test-layer:
    description: a test layer
    compatibleRuntimes: [ python3.8 ]
    compatibleArchitectures: [x86_64]
    license: MIT
    publish: true 
    code:
        bucket: lambda-zip-artifacts
        key: layer/test-layer/${gitSHA}.zip
```

