

`buildit` has a concept of named *providers*. The *providers* are used to configure credentials for the AWS SDK. These provider names are then used to specify where a resource is located, created, updated & deleted from. When a provider is not supplied, it is assumed the intention is to use `main`. 

> Note: `buildit` supports provisioning resources in AWS only, so the providers configuration is specific to AWS.

There are two reserved provider names, `default` & `main`. If not explicitly configured, the `main` provider is just a clone of the `default`.

### Provider Scoping

In a `buildit` spec, a resource is always scoped with a `provider` to indicate where this resource will be built. For instance a security group `test-sg` is built in the `main` provider's scope. The fully-qualified version of this resource name is `main::test-sg`. Here `main` is assumed. It is recommended to explicitly scope all resource names & references.

The scoping can be defined in one of 2 ways:

- `<provider_name>` followed by a double colon `::` then the resource name (or the resource identifier). There may be some [exceptions](#provider-scoping-rule-exceptions) to this rule due to legacy/compatibility reasons.
- Supply the provider name in the `provider` field for any resource. 

#### Example: 


```yaml 

  security-group:
    production::test-sg:
      description: a test security group
      vpcName: test-vpc
      outboundRules: [...]
      inboundRules: [...]
      tags:
        Name: test-sg

```

is equivalent to:

```yaml 
  security-group:
    test-sg:
      provider: production
      description: a test security group
      vpcName: test-vpc
      outboundRules: [...]
      inboundRules: [...]
      tags:
        Name: test-sg
```

### Notes:

> - When using `main` the resource scope can be omitted, but recommended for clarity.
> - `CAUTION` Provider must be supplied using ONLY one of the above wayS; supplying IN both places may result in error in some cases.
> - All non-scoped resource references within a resource definition use the same provider, unless otherwise configured where allowed. 


## A side effect of scoping:

A side-effect of the `provider::id` usage is the ability to allow the same resource name to be built in 2 AWS accounts *(pointed by the different providers)* within the same buildit context (yaml file). As we know, YAML doesn't allow using the same key for two fields under the same stanza. For instance, if we wish to build a security group `test-sg` in two different accounts within the same buildit context, we can use `main::test-sg` & `other::test-sg` to define both without a conflict.

This can also be used to create a resource with the same name (in some special cases where resource name can be a duplicate) in the same AWS account. For instance, an `A`  `route53-record` record for `www.example.com` & `www.example2.com` can then be defined without needing to use a psuedo resource name. 

### Example;

Using same provider, we have to use a pseudo-name & `recordName` field for the 2 DNS records: 

```yaml 

 providers:
    main:
      type: role
      accountId: 123456789012
      roleName: role-name

 resources: 

  route53-record:

    www-example:
      recordName: main/www
      hostedZone: example.com
      aliasType: ...

    www-example2:
      recordName: main/www
      hostedZone: example2.com
      aliasType: ...

```

Using different providers to define the same can simplify the definition of the above as:

```yaml 
 providers:
    main_1:
      type: role
      accountId: 123456789012
      roleName: role-name
    main_2:  # main_2 same a main_1 but a different provider name
      type: role
      accountId: 123456789012
      roleName: role-name

 resources: 

  route53-record:

     # provider scoped resource name, no need to use recordName field 
    main_1::www:
      hostedZone: example.com
      aliasType: ...

    main_2::www:
      hostedZone: example2.com
      aliasType: ...
```

## Important Notes:

> - If not explicitly qualified, all resources provisioned, destroyed, updated; or any searches for referenced resources are done using credentials from the `main` provider.
> - A provider reference is also required in the `dependsOn` section of any resource. If not supplied `main` is assumed. 
> - A provider reference can also be supplied to the `--targets` command-line flag, If omitted, `main` is assumed.

## Provider Scoping Rule **Exceptions** :

Due to legacy usage & backward compatibility requirements, the following exceptions to the provider scoping exists. 

### Definitions:

The following resource definitions can use both the `::` or legacy `/` separator for provider scoping. These 2 resource types support the old `/` separator for backward-compatibility reasons. They should be converted to use the new `::` method eventually.

- The `route53-record` resource. For instance, both `prod/cloud` and `prod::cloud` are valid fully-qualified resource names.
- The`sqs-queue` resource. For instance, both `prod/test-queue` and `prod::test-queue` are valid fully-qualified resource names.

### References:

- When referencing a security group in the `inboundRules->securityGroups[].value` or `outboundRules->securityGroups[].value` field of a `security-group` resource definition, we use `provider/id` format to reference a security group in other AWS account/provider. e.g `production/other-sg`
When referencing a `dnsValidationDomainName` for an ACM `certificate` resource, we still use the `provider/id` format to reference a DNS Hosted Zone in another account/provider.
- When referencing a secret in the `taskDef` resource `secrets` section, we still use the `provider/secret_fqn` where example of this could be `EMAIL_PASSWORD: "production/secret_name:key::"` indicating a secret `secret_name`, json key `key` & version `latest` (Default) to from the `production` provider/account to be injected here.



<br/>
<br/>

For more details the various provider names, reserved names & types, see below:
<br/>
<br/>



# `default` Provider

A default provider is configured using `buildit` command-line arguments. This is the starting point for `buildit`. If no configuration is supplied, then the `default` provider is configured using the precendence defined by the AWS SDK default credentials chain:

- Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`)
- Shared credentials file (`~/.aws/config`, `~/.aws/credentials` &  profile name is `default`)
- If buildit is running as an ECS task or RunTask API operation, IAM role for tasks.
- If buildit is running on an Amazon EC2 instance, IAM role for Amazon EC2.

Example: 

```
% buildit apply \ 
         --default-provider-type default \  
         buildit.yaml

```

or simply 

```
% buildit apply buildit.yaml
```

To configure the `default` provider using specific credentials sources, you can use the `buildit` command-line.

## Environment Variables

To use environment variables for AWS credentials, set the value for the `--default-provider-type` command-line flag to `env`. Additionally, if you wish to use non-default AWS environment variables, you can provide `--access-key-id-env-name`, `--secret-access-key-env-name` & `--session-token-env-name` to supply the environment variable names for reading the access_key_id, secret_access_key & session_token.

Example:

```
% buildit apply \ 
         --default-provider-type env \  
         --access-key-id-env-name MY_ACCESS_KEY_ID \
         --secret-access-key-env-name MY_SECRET_ACCESS_KEY \
         --session-token-env-name MY_SECRET_ACCESS_KEY \
         buildit.yaml
          
```

## Shared Credentials

To use shared credentils file set the value for the `--default-provider-type` command-line flag to `shared`. This instructs `buildit` to use `~/.aws/config` & `~/.aws/credentials` files & the `default` profile. To override any of these defaults, you can supply the additional flags on buildit command-line, e.g `--config-file` & `--creds-file` to point to the AWS config & credentials files & `--profile` to specify the profile to use.

Example:

```
% buildit apply \ 
          --default-provider-type shared \  
          --config-file /tmp/config \
          --creds-file /tmp/credentials \ 
          --profile provisioner \ 
          buildit.yaml

```
## Assuming a role

If you wish the `default` provider to assume an IAM Role, set the value for `--default-provider-type` command-line flag to `role` and supply additional values:
- the "arn" of the role to be assumed using `--role-arn` flag, or 
- the AWS account ID & role name using `--account-id` & `--role-name` flags

> Note: The credentials used to assume the supplied role (aka base credentials) will be retrieved using AWS SDK default credentials chain. This means you cannot enforce the base credentials to be `env`
 or `shared` type.


Example: 

```
% buildit apply \ 
          --default-provider-type role \  
          --role-arn arn:aws:iam::123456789012:role/provisioning-role \
          buildit.yaml
```

or a combination of the role name & AWS account ID using `--role-name` & `--account-id` flags.

```
% buildit apply \ 
          --default-provider-type role \  
          --account-id 123456789012
          --role-name provisioning-role \
          buildit.yaml
```



# `main` Provider

`buildit` creates a clone of the `default` provider and saves it as `main`. All AWS API actions performed by `buildit` use the `main` provider.  If for some reason we need a different role for these operations you can either configure the `default` provider with those credentials, or explicitly configure the `main` provider by specifying a `role` to assume. 

> Note: All non-default providers, including `main` when explicitly defined, are done so in the buildit config YAML files.

> Note: All non-default providers, including `main` when explicitly defined, can only be configured with an IAM Role to assume using the `default` provider as the base. This means the buildit `default` provider must have the correct IAM permissions to assume that role. 

Example of overriding the `main` provider by assuming the IAM role identified by `accountId` & `roleName`

```yaml
providers: 
  main:
    type: role
    accountId: 123456789012
    roleName: buildit-role

```

Alternatively, this can also be written as: 

```yaml
providers: 
  main:
    type: role
    roleArn: arn:aws:iam::123456789012:role/buildit-role
```


> Note: All resources are provisioned using the credentials represented by the `main` provider. There are some exceptions but thost will be defined in those respective resource documentations


# Other providers

You can configure additional providers for `buildit` to allow searching or referencing resources while provisioning a resource. A common example is security group references for a security group inbound (ingress) rule. In case you want to specify a security group that is in a different AWS account, you can add a named provider say `otheraccount` & supply the details for that provider in the `buildit` YAML file.

The same rules & restrictions that apply to `main` also apply to all named providers 

For example, if we are provisioning a new security group called `test-sg` in AWS account `123456789012` and want to allow another security grouop `bastion-sg` in the account `210987654321` as an ingress reference, we will do the following. Notice how the value of the securityGroup is `other/bastion-sg`. The `other/` syntax, where supported indicates that the resource is to be looked-up using the `other` credentials (provider).

```yaml
providers: 
  main:
    type: role
    accountId: 123456789012
    roleName: buildit-role

  other:
    type: role
    accountId: 210987654321
    roleName: other-role
   
resources:
  test-sg:
    description: security group for test
    vpcName: my-vpc
    inboundRules:
    - portRange: 8080 
      ipProtocol: tcp 
      securityGroups:
      - value: other/bastion-sg
        description: allow inbound traffic from the bastion sg from other account

```

> Note: In `buildit` almost all resources are built & referenced using human readable names. So we need to convert the references to AWS-specific Ids or ARNs before using them. The primary role of all named providers, other than `default` and `main` is to allow searching the referenced resources so we can fetch those AWS Ids or ARNs. Some resources can also be provisioned using non-`main` credentials, e.g `route53-record`. These will be explicitly documented.



# Advanced

Although not recommended, but if `main` or other named-providers need to be configured independent of `default` with assume role, it can be done by using an `env` or `shared` provider types. However, be careful when setting up AWS config environment variables, or credentials file to not interfere with the `default` provider itself. Non-default AWS environment variables or shared credential files can be used.

```yaml
providers: 
  main:
    type: env
    accessKeyIdEnvVar: MY_ACCESS_KEY_ID
    secretAccessKeyEnvVar: MY_SECRET_ACCESS_KEY
    sessionTokenEnvVar: MY_SESSION_TOKEN

  other:
    type: shared
    configFile: /tmp/config
    credsFile: /tmp/credentials
    profile: profile1
  

```


 ## References:

Under the hood, `buildit` uses [awesome](https://github.com/TouchBistro/awesome#readme), which is an open-source package, maintained by Touchbistro to simplify AWS credentials configuration.



