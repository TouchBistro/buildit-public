
# Templates & Includes

Templates make it easier to structure & reuse buildit spec. In situations where there are a lot of similar resource definitions, a lot of that configuration can be extracted into anchors, a YAML feature or just snippets of text that can be replaced into a templated file to create a final runtime configuration. 

## Includes

**Buildit** supports an `--include` option that allows passing a single of set of files, directories, or glob patterns to read in a list of files. All files in scope are read & merged in that order to build a single include byte stream. 

Later when config files are read, each of those files is prepended by the include files bytes & then parsed for buildit config. This is useful when every file read by buildit requires common content to be included at the top of the file. This helps in some cases, but is restricted & inefficient. 

## Templates

**Buildit** also supports go text/template style actions. Shared templates or snippets can be defined in files that can be included for parsing using the `--template` flag. Similar to include, these can be a single or multiple file, directory (.yml, .yaml only) or globs. All files in scope are read & compiled as templates. Any `{{ define name}}` .. `{{ end }}` tags within will create more named templates in the shared context.  

Later when config files are read, they can reference one or more named templates for inclusion using text template actions `{{template . "name"}}`. 

Templates in **buildit** support all data evaluation & control structure operations out of the box defined here: [text/template](https://pkg.go.dev/text/template) engine.

Both the *includes* & *templates* allow variable subsitution using the `token` or template `action` tag notations. see [variables](variables.md) for more details.
