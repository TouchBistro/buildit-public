package buildit

import (
	"fmt"

	"github.com/TouchBistro/buildit/config"
	"github.com/pkg/errors"
)

// Option configures NewConfig.
type Option func(*options)

type options struct {
	root       config.RootOptions
	globalTags map[string]string
}

// validate rejects option combinations that cannot mean anything, before any file is
// read or provider touched.
func (o options) validate() error {
	if len(o.root.ConfigPath) == 0 {
		switch {
		case len(o.root.OverridePath) > 0:
			return errors.New("WithOverridePath requires WithConfigPath")
		case len(o.root.TemplatesPath) > 0:
			return errors.New("WithTemplates requires WithConfigPath")
		case len(o.root.VariablesPath) > 0:
			return errors.New("WithVariablesFrom requires WithConfigPath")
		case len(o.root.Variables) > 0:
			return errors.New("WithVariable requires WithConfigPath")
		case o.root.UseEnvars:
			return errors.New("WithEnvVariables requires WithConfigPath")
		}
	}
	return nil
}

// WithGlobalTags adds tags to apply to every resource, as the `globalTags:` section of a
// buildit.yml does. Repeated keys across calls take the later value.
func WithGlobalTags(tags map[string]string) Option {
	return func(o *options) {
		for k, v := range tags {
			WithGlobalTag(k, v)(o)
		}
	}
}

// WithGlobalTag adds one tag to apply to every resource.
func WithGlobalTag(key, value string) Option {
	return func(o *options) {
		if o.globalTags == nil {
			o.globalTags = make(map[string]string)
		}
		o.globalTags[key] = value
	}
}

// WithoutAuditTags skips the `audit` global tag, which buildit otherwise derives from
// the config file contents. A Config built in Go has no file content to hash, so the
// derived value is only meaningful when config files are merged in.
func WithoutAuditTags() Option {
	return func(o *options) {
		o.root.NoAuditTags = true
	}
}

// WithConfigPath merges buildit.yml files into the Config, in addition to the resources
// built in Go. Each path is a file, a directory (.yml and .yaml files) or a glob.
func WithConfigPath(paths ...string) Option {
	return func(o *options) {
		o.root.ConfigPath = append(o.root.ConfigPath, paths...)
	}
}

// WithTemplates renders the config files with these text/template files (.tmpl), the
// way the --templates flag does. Each path is a file, a directory or a glob.
func WithTemplates(paths ...string) Option {
	return func(o *options) {
		o.root.TemplatesPath = append(o.root.TemplatesPath, paths...)
	}
}

// WithOverridePath applies an override config file to the merged config, the way the
// --override flag does. The path must resolve to a single file, and the option can be
// given once: a second call makes NewConfig fail, as merging overrides is not supported.
//
// Deprecated: override configs were experimental and are kept only for existing users;
// prefer templates and variables.
func WithOverridePath(path string) Option {
	return func(o *options) {
		o.root.OverridePath = append(o.root.OverridePath, path)
	}
}

// WithVariable sets a variable for the config files' ${name} substitutions, the way
// --variables name=value does. It takes precedence over WithVariablesFrom and
// WithEnvVariables.
func WithVariable(name, value string) Option {
	return func(o *options) {
		o.root.Variables = append(o.root.Variables, fmt.Sprintf("%s=%s", name, value))
	}
}

// WithVariablesFrom loads variables from .vars files for the config files' ${name}
// substitutions. Each path is a file, a directory or a glob.
func WithVariablesFrom(paths ...string) Option {
	return func(o *options) {
		o.root.VariablesPath = append(o.root.VariablesPath, paths...)
	}
}

// WithEnvVariables loads variables from BUILDIT_-prefixed environment variables for the
// config files' ${name} substitutions, the way --use-envvars does.
func WithEnvVariables() Option {
	return func(o *options) {
		o.root.UseEnvars = true
	}
}
