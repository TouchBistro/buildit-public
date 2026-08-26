package config

import (
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/resource"
)

type RootOptions struct {
	ConfigPath      []string // configPath includes a file, directory (.yml & .yaml included) or a glob pattern
	OverridePath    []string // overridePath includes a file, directory (.yml & .yaml included) or a glob pattern; must resolve to a single file
	IncludePath     []string // includePath includes a file, directory (.yml & .yaml included) or a glob pattern
	VariablesPath   []string // variablesPath includes a file, directory (.var & .vars included) or a glob pattern
	TemplatesPath   []string // templatesPathincludes a file, directory (.tmpl included) or a glob pattern
	Variables       []string
	UseEnvars       bool
	NoAuditTags     bool
	Targets         []string
	DefaultProvider client.AwsProvider
	LogLevel        string
	Timestamp       bool
	Retries         int
	Backoff         int
}

// container stores all the dependencies that can be used by commands.
type Container struct {
	ResourceGraph *Graph
	Tags          map[string]string
	Providers     map[string]*client.AwsProvider
	RootOpts      *RootOptions
	Targets       []resource.Key // TODO do we need these & below?
}
