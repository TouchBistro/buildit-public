# Variables

**buildit** allows tokenizing YAML config files for runtime subsitution with values, called variables. This enables customizing configuration and supply specific values at runtime, or enabling reusing of the same config for different usecases, like `production` & `staging` environments. The are two ways to embedding tokens in a config file.

## Tokens

**Buildit** tokens can be placed using `${token}` syntax. These tokens are replaced at runtime using values supplied as variables. The value within the `${` and `}` must match exactly with a key that matches one of the variables sources. If a variable replacement cannot be found, **buildit** will return an error during the parsing stage.

## Actions

With the introduction of go-style templating, the go action tag can be used for basic variable replacement directly. Under the hood, all token style tags i.e `${token}` are replaced to an equivalent action tag `{{getValue . "token"}}`. This utilizes the **buildit** built-in `getValue` template function to ensure a value for `token` is looked up in a case insensitive manner across the various variable value sources following the correct precedence rules, i.e. `--variables key=value` flag, `--variables-from` files or the env vars.

For backward compatibility, and ease of use, the *preferred* method for placing tokens in config, include or template files is the legacy token syntax `${token}`.  

If supplying go action style tags, use one of the following options: 

- `{{.token}}`: only support variables read from variables files; Does not support indexing lists.

- `{{getValue . "token"}}`: supports all variables sources, and extended syntax like a.b.c.0 for lists.


## Variables

During the parsing stage, tokens are replaced with matching variable values supplied in the following order:

- Variable files in yaml format supplied via command-line flag `--variables-from <path>`. Here `path` can be a single, or list of absolute or relative file paths, directories (all .yml or .yaml files are included from those directories) or a glob pattern. All files are read in order they are supplied and listed, where later variables overwrite values from earlier declarations when the same keys are used. The yaml file must have a top-level `variables` key with a map! type. Values can be referenced using a fully qualified or relative to template keys e.g `env.production.key1` 

- Environment variables prefixed with `BUILDIT_` (case-insensitive), when `--use-envvars` flag is supplied on the command-line.

- Values supplied on the command-line in the format key=value using `--variables` flag on the command-line, e.g `--variables token=value`

## Example:

Suppose we supply the following `variables.yml` file

```yaml
# variables.yml
---
variables:
  repo: shared
  region: us-east-1
  env:
    production:
      key1: value1
      key2: 
      field1: val0
      field2: 123
      field3: xyz
      key3:
        - time0
        - item1
        - item2
```

and the following environment variables are set 

```bash 
export BUILDIT_REGION=us-west-2
```

& the following configuration files is supplied to a buildit command-line 

```bash
buildit --path buildit.yml --variables repo=buildit --variables-from variables.yml --use-envvars
```


```yaml 

# buildit.yml 
--- 
globalTags:
  tag_repo: ${repo} # replaced by buildit, overridden from command-line
  tag_region: ${region} # replaced by us-west-2, overridden from environment
  tag_key1: ${env.production.key1}  # replaced by value1
  tag_key2: ${env.production.key2.field2} # replaced by 123 
  tag_key3: ${env.production.key3.1} # replaced by 1

```