


## Buildit Commands

The following top-level commands are available for the **buildit** CLI

|Command| Description|
|----:|:---|
| `apply` | Create or update resources as defined in the supplied config |
| `destroy` | Destroys all resources in the supplied config |
| `plan` | Builds and displays a plan of expected changes when `apply` is run |
| `validate` | Checks validity of the supplied buildit config |
| `help` | Displays the CLI documentation for all supported commands & flags |

> <br/>
>
> For an up-to-date documentation of the **buildit** CLI, use: 
>
>```cmd
>% buildit -h
>```
>
>OR 
>```cmd
>% buildit --help
>```
> </br>

## Flags:

The following top-level flags apply to **buildit** root command only.

| Flag| Default| Description|
|---:|---|---|
| `help` |  | Prints a help message for buildit commands & flags.|
| `version` |  | Prints the embedded version info. |
| `log-level` | `info` | Sets the logger break-level. (allowed values: `trace` \| `debug` \| `info` \| `warn` \| `error` \| `fatal` \| `panic` )|
| `retries` | `3` | Sets the maximum retries for AWS API calls|
| `backoff` | `20` | Sets the maximum backoff (in seconds) between retries for AWS API calls|
| `timestamp` | | nable timestamp in buildit log|

The following top-level flags are used by the `apply`, `destroy`, `plan` & `validate` commands.

| Flag| Default| Description|
|---:|---|---|
| `path` | [`.buildit/`] | Path points to buildit config file(s). This can be a fully-qualified or relative path of the buildit YAML config file. If the path points to a directory instead, then all files with `.yml` or `.yaml` extensions are read and merged in an alphabetical order. The path parameter can also be a glob pattern. In that case all files that match the pattern are merged to read the buildit config. see section [The `--path` parameter](#the---path-parameter) for more details on usage.|
| `include` |  | **DEPRECATED** A fully qualified or relative file name, directory. or a glob pattern that specifies the file(s) to be included in each of the buildit YAML spec files that are read before merging. This allows separation of anchors or common segments of a buildit spec in a separate include file to avoid repetition |
| `template` |  | A fully qualified or relative file name, directory. or a glob pattern that specifies the file(s) to be compiled as go text/template compliant templates, and then can be included in the buildit config files to be referenced for reuse|
| `override` |  | **DEPRECATED** The fully-qualified or relative path of the `override` YAML config file. Can be a directory or glob pattern as well, however must only result in a single file after |
| `variables` |  | A comma-separated list of variable names & values, formatted as `name=value`. Variable tags defined in the buildit config yaml as `${name}` will be substituted with the supplied values before parsing begins. **buildit** will report an error if a \${name} tag exists in the config, but a value for it is not supplied here |
| `variables-from` |  | A list of files, directories or glob patterns representing yaml encoded files that can be referenced as variable values for substitution of tokens or action tags. See [Variables](variables.md) for more details.|
| `use-envvars` |  | when supplied, env variables with a case-insensitive `BUILDIT_` prefix can be used for value subsitution of tokens. see [Variables](variables.md) for more details.|
| `targets` |  | A comma-separate list of resource names from the supplied config to proces (apply, plan or destroy). If this flag is not supplied, all resources are processed. If invalid resource names are supplied, they are ignored.|
| `no-audit` |  | when supplied, **buildit** does not add its `audit` tag to created or updated resources. See [Reserved tag keys](./config.md#reserved-tag-keys).|

The following top-level flags are used by the `apply`, `destroy` & `plan` commands. 

These flags define how the `default` buildit AWS provider is configured. For more details on **buildit** provider see [Providers](./providers.md)

| Flag| Default| Description|
|---:|---|---|
| `default-provider-type` |`default`| Defines the `default` provider's type. If set to any value other than default, additional configuration may be required (allowed values: `default` \| `env` \| `shared` \| `role`)|
| `access-key-id-env-name` |`AWS_ACCESS_KEY_ID`| The env variable name to use for reading AWS Access Key ID, when `default-provider-type` is set to `env`|
| `secret-access-key-env-name` |`AWS_SECRET_ACCESS_KEY`| The env variable name to use for reading AWS Access Key ID, when `default-provider-type` is set to `env`|
| `session-token-env-name` |`AWS_SESSION_TOKEN`|The env variable name to use for reading the AWS Session Token, when `default-provider-type` is set to `env`|
|`config-file`|`~/.aws/config`| The fully-qualified or relatve path for the AWS config file, when `default-provider-type` is set to `shared`|
|`creds-file`|`~/.aws/credentials`| The fully-qualified or relatve path for the AWS credentials file, when `default-provider-type` is set to `shared`|
|`profile`|`default`| The name of the AWS shared credentials profile, when `default-provider-type` is set to `shared`|
|`account-id`| Required* (`role-name` must also be supplied)| [*Deprecated: Use `role-arn`*] The AWS account ID part of the role ARN to assume, when `default-provider-type` is set to `role`|
|`role-name`|Required* (`account-id` must also be supplied)|[*Deprecated: Use `role-arn`*] The AWS IAM Role name part of the role ARNto assume, when `default-provider-type` is set to `role`.|
|`role-arn`| Required*| The AWS IAM Role ARN to assume, when `default-provider-type` is set to `role`. <br> <br>The base credentials to assume this role are taken from the AWS SDK's Default Credentials Chain. Make sure those are set as intended & have the required permissions to assume the supplied role. Also the role supplied here must have a trust-policy defined to allow role assumption by the base credential source.|
|`region`| `us-east-1`| The AWS Region to use with the SDK |
## Commands:


More details on commands & command-level flags below:

### `apply` Command

The apply command instructs **buildit** to create or update the resources defined in the file indicated by `path`. If `target` flag is supplied, then only the targetted resources will be applied.

```cmd
% buildit apply --path /loc/of/buildit.yml \
                --variables tag=latest \
                --targets securitygroup1,securitygroup2
```

### `destroy` Command

The destroy command instructs **buildit** to destroy (or purge) the resources defined in the file indicated by `path`. If `target` flag is supplied, then only the targetted resources will be destroyed.

```cmd
% buildit destroy --path /loc/of/buildit.yml \
                --variables tag=latest \
                --targets securitygroup1,securitygroup2
```
### `plan` Command

The plan command instructs **buildit** to review & print a plan of action for applying the resources defined in the file indicated by `path`. If `target` flag is supplied, then only the targetted resources will be planned.

```cmd
% buildit plan --path /loc/of/buildit.yml \
                --variables tag=latest \
                --targets securitygroup1,securitygroup2
```

Plan-only flag:

| Flag | Default | Description |
|---:|---|---|
| `ignore-tags` |  | A comma-separated list of tag keys to ignore while computing plan diffs. This only affects `plan` output and does not change `apply` behavior. Useful for reading a plan around an expected tag change, e.g. `--ignore-tags buildit:resource-id` when rolling that tag out across existing resources. |

### `validate` Command

The validate command instructs **buildit** to check if the supplied buildit config file is well-formed. Variable substituion is required. If the target config yaml file has variable tags, but values are not supplied, the validation will not run.

```cmd
% buildit validate --path /loc/of/buildit.yml \
                --variables tag=latest \
                --targets securitygroup1,securitygroup2
```


## The `--path` parameter 

**buildit** can read its config (or override config, include, template or variable values ) from one or more files. For the buildit config files, if multiple files are supplied by way of a *directory path* or a *glob pattern*, each file must be a valid yaml file on its own. They can contain one or more of the 3 top-level buildit yaml config sections, i.e `providers`, `resources` & `globalTags` For details on the buildit YAML config DSL see [config](./config.md). Similarly the other types of buildit files, overrides, include, templates or variables have their own corresponding parsing rules. 

See [paths & templates](./paths_templates.md) for detailed rules on how files are parsed when includes or templates are supplied.

When a *directory path* or *glob pattern* is supplied that resolves to more than one file, the files are merged section by section. This means each file is individually unmarshalled from YAML & validated, then the corresponding sections are merged. While merging sections, if the same object is defined in 2 or more files, only the last one will remain.  For instance, if two files contain the definition of a provider `production`, then the one defined last (in the alphabetical order of file names) will be included in the spec. This behavior is identifcal to defining the same named resource within a single file.

Failure to parse any of the included files will causes buildit to exit with an error.

The value for this parameter is of type `[]string` so a single or a comma-separated list of paths can be supplied. If special non alpha-numeric characters are used, then it is safe to quote the value. For ease of use, multiple values for the path parameter can be supplied by using the flag more than once.

A value for `--path` parameter can be supplied in 3 ways:

### File path 
A file path is a fully-qualified, or a relative path to a buildit YAML file. There are no restrictions on the file name or its format. If the file does not exist, or cannot be read, buildit will exit with an error.

```
% buildit plan \
         --path /etc/buildit/my-service.yml \
         --variables ver=123
```

### Directory path 
A directory path is one that points to a directory name. If a directory name is supplied for the `path` prameters, all files contained in it that have a `.yml` or `.yaml` extension are read in the alphabetical order and merged to form the final config. If the directory has no files that match, an error is returned by buildit.

```
% buildit plan \
         --path /etc/buildit \ 
         --variables ver=123
```


### Glob pattern
If the path parameter contains a glob pattern, then it is used to read & merge the resulting files. If not files match the pattern an error is returned. The following example means "read all files in the `/etc/buildit/` directory that match a pattern `service00[0|1|2]*.yml`.

```
% buildit plan \
          --path /etc/buildit/service00[012]*.yml \
          --variables ver=123
```


Use the `--log-level debug` option to see the list of files that buildit is parsing as a result of the supplied `path` parameter, 
