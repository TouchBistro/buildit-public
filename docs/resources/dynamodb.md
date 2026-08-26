# DynamoDB Table `dynamodb`

Create, update or destroy a DynamoDB table with support for global secondary indexes and global tables. Check out AWS documentation for DynamoDB [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/dynamodb/create-table.html).

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

> **Warning:** Global secondary index creation currently only works with `PAY_PER_REQUEST` billing mode. If `provisionedThroughput` is provided, global secondary indexes are not supported.

| Field                    | Description                                                                                                                                                                                                                                                                                           | DataType                          | Required | Default |
| ------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------- | -------- | ------- |
| Name (Resource Name)     | The resource name is used as the name of the DynamoDB table                                                                                                                                                                                                                                           | `string`                          | Yes      |         |
| `attributeDefinitions`   | An array of attributes that describe the key schema for the table and indexes. See [AttributeDefinition](#attributedefinition)                                                                                                                                                                        | `[]AttributeDefinition`           | Yes      |         |
| `keySchema`              | Specifies the attributes that make up the primary key for a table or an index. See [KeySchema](#keyschema)                                                                                                                                                                                            | `[]KeySchema`                     | Yes      |         |
| `provisionedThroughput`  | Provisioned throughput settings for the table. If specify the billing mode is set to `PROVISIONED`. Otherwise billing mode will be `PAY_PER_REQUEST`. **Note:** Global secondary indexes are not supported when using `PROVISIONED` billing mode. See [ProvisionedThroughput](#provisionedthroughput) | `ProvisionedThroughput`           | No       |         |
| `deletionProtection`     | Indicates whether deletion protection is to be enabled (true) or disabled (false) on the table                                                                                                                                                                                                        | `bool`                            | No       | `true`  |
| `globalTableRegions`     | List of AWS regions for global table replication. Automatically enables DynamoDB Streams with `NEW_AND_OLD_IMAGES` view type                                                                                                                                                                          | `[]string`                        | No       | `[]`    |
| `globalSecondaryIndexes` | A key-value map for one or more global secondary indexes to be created on the table. See [GlobalSecondaryIndex](#globalsecondaryindex)                                                                                                                                                                | `map[string]GlobalSecondaryIndex` | No       |         |
| `serverSideEncryption`   | Enable or disable server-side encryption                                                                                                                                                                                                                                                              | `bool`                            | No       | `true`  |
| `resourcePolicy`         | The resource-based policy document that is attached to the table. See [IAMPolicyDocument](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_elements.html) for more details                                                                                                         | `IAMPolicyDocument`               | No       |         |
| `updateTimeout`          | The maximum time in seconds to wait for each table update operation to complete                                                                                                                                                                                                                       | `int`                             | No       | `900`   |
| `ttlAttribute`           | The name of the Time To Live attribute for items in the table. When specified, DynamoDB will automatically delete items after the TTL expires                                                                                                                                                         | `string`                          | No       |         |
| `tags`                   | A key value map of resource tags to be applied to this resource. The `GlobalTags` are always applied, any matching keys are overriden from `tags`                                                                                                                                                     | `map[string]string`               | No       | `{}`    |
| `dependsOn`              | The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destroyed after this                                                                                                    | `[]string`                        | No       | `[]`    |

Example:

```yaml
resources:
  dynamodb-table:
    users-table:
      attributeDefinitions:
        - name: userId
          type: S
        - name: email
          type: S
        - name: createdAt
          type: N
      keySchema:
        - name: userId
          type: HASH
        - name: createdAt
          type: RANGE
      deletionProtection: true
      serverSideEncryption: true
      ttlAttribute: expiresAt
      globalTableRegions:
        - us-west-2
        - eu-west-1
      globalSecondaryIndexes:
        email-index:
          keySchema:
            - name: email
              type: HASH
          projection:
            type: ALL
        created-index:
          keySchema:
            - name: createdAt
              type: HASH
          projection:
            type: KEYS_ONLY
      resourcePolicy:
        Version: "2012-10-17"
        Statement:
          - Effect: Allow
            Principal:
              AWS: "arn:aws:iam::123456789012:role/buildit/example-kitchen-sink-role"
            Action:
              - dynamodb:BatchWriteItem
              - dynamodb:Query
              - dynamodb:Scan
      tags:
        Environment: production
        Team: backend
```

---

## AttributeDefinition

An attribute definition specifies the name and data type of an attribute that will be used in the table's key schema or global secondary index key schema.

| Field  | Description                                                                             | DataType | Required | Default |
| ------ | --------------------------------------------------------------------------------------- | -------- | -------- | ------- |
| `name` | The name of the attribute                                                               | `string` | Yes      |         |
| `type` | The data type for the attribute. Valid values: `S` (String), `N` (Number), `B` (Binary) | `string` | Yes      |         |

---

## KeySchema

Specifies the attributes that make up the primary key for a table or an index. The attributes must be defined in the `attributeDefinitions` array.

| Field  | Description                                                                                            | DataType | Required | Default |
| ------ | ------------------------------------------------------------------------------------------------------ | -------- | -------- | ------- |
| `name` | The name of a key attribute                                                                            | `string` | Yes      |         |
| `type` | The role that this key attribute will assume. Valid values: `HASH` (partition key), `RANGE` (sort key) | `string` | Yes      |         |

---

## ProvisionedThroughput

Represents the provisioned throughput settings for a specified table or index. Required when `billingMode` is set to `PROVISIONED`.

| Field                | Description                                                                                                       | DataType | Required | Default |
| -------------------- | ----------------------------------------------------------------------------------------------------------------- | -------- | -------- | ------- |
| `readCapacityUnits`  | The maximum number of strongly consistent reads consumed per second before DynamoDB returns a ThrottlingException | `int64`  | Yes      |         |
| `writeCapacityUnits` | The maximum number of writes consumed per second before DynamoDB returns a ThrottlingException                    | `int64`  | Yes      |         |

---

## GlobalSecondaryIndex

Represents the properties of a global secondary index. The key is the index name.

| Field        | Description                                                                                                                                             | DataType      | Required | Default |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------- | -------- | ------- |
| `keySchema`  | The complete key schema for a global secondary index, which consists of one or more pairs of attribute names and key types. See [KeySchema](#keyschema) | `[]KeySchema` | Yes      |         |
| `projection` | Represents attributes that are copied (projected) from the table into the global secondary index. See [Projection](#projection)                         | `Projection`  | Yes      |         |

---

## Projection

Represents attributes that are copied (projected) from the table into the global secondary index.

| Field              | Description                                                                                                                                                                                              | DataType   | Required | Default |
| ------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------- | -------- | ------- |
| `type`             | The set of attributes that are projected into the index. Valid values: `ALL` (all attributes), `KEYS_ONLY` (only the index and primary keys), `INCLUDE` (specific attributes listed in nonKeyAttributes) | `string`   | Yes      |         |
| `nonKeyAttributes` | Represents the non-key attribute names which will be projected into the index. Required when `type` is `INCLUDE`, must be omitted for other projection types                                             | `[]string` | No       |         |

---
