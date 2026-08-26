package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"

	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/resource"
	"github.com/TouchBistro/buildit/util"

	"github.com/TouchBistro/buildit/config"
	"github.com/TouchBistro/goutils/color"
	"github.com/TouchBistro/goutils/fatal"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// Set by goreleaser when a release build is created
var version string

// Execute this command
func Execute() {
	// Listen for SIGINT to do a graceful abort
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	abort := make(chan os.Signal, 1)
	signal.Notify(abort, os.Interrupt)
	go func() {
		<-abort
		cancel()
	}()

	var c config.Container
	rootCmd := newRootCommand(ctx, &c)
	err := rootCmd.Execute()

	if err := client.Statsd().Flush(); err != nil {
		log.Warnf("Error occurred while flushing statsd client: %v", err)
	}
	exiter := fatal.Exiter{PrintDetailed: true}
	if errors.Is(err, context.Canceled) {
		exiter.PrintAndExit(&fatal.Error{
			Msg:  "\nOperation cancelled",
			Code: 130,
		})
	}

	if err != nil {
		// print stack trace for `DEBUG` or higher
		if log.GetLevel() >= log.TraceLevel {
			exiter.PrintAndExit(err)
		}

		// handling print minimal & exit here
		log.Error(err.Error())
		os.Exit(-1)
	}
}

// newRootCommand builds rootCommand
func newRootCommand(ctx context.Context, c *config.Container) *cobra.Command {
	// Set version if built from source
	if version == "" {
		version = "source"
		if info, available := debug.ReadBuildInfo(); available {
			version = info.Main.Version
		}
	}

	var opts config.RootOptions
	rootCmd := &cobra.Command{
		Use:               "buildit",
		Version:           version,
		Short:             "buildit is a CLI for creating AWS resources in the TouchBistro ecosystem",
		SilenceErrors:     true, // cobra prints errors returned from RunE by default. Disable that since we handle errors ourselves.
		SilenceUsage:      true, // cobra prints command usage by default if RunE returns an error.
		PersistentPreRunE: getRootPersistentPreRunE(&opts, c),
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "/tmp"
	}

	const mvPathHelpMsg = "The value for this parameter can be a list of file or directory paths, or glob patterns. " +
		"Multiple values can be supplied by comma-separating them, or use the flag more than once for each value "
	persistentFlags := rootCmd.PersistentFlags()
	persistentFlags.StringSliceVar(&opts.ConfigPath, "path", []string{".buildit/"}, mvPathHelpMsg+
		"This value is used for locating the buildit config files. If it resolves to multiple config files, each file in independently parsed & merged section by section with configs from other files."+
		"When a directory path is supplied, only files with extension .yml or .yaml are loaded")

	persistentFlags.StringSliceVar(&opts.OverridePath, "override", []string{}, mvPathHelpMsg+
		"This value is used for locating the override config file. See documentation for more details on how overrides are processed.")

	persistentFlags.StringSliceVar(&opts.IncludePath, "include", []string{}, mvPathHelpMsg+
		"The value is used for locating the include files that are prepended to every config file before they are parsed. "+
		"When a directory path is supplied, only files with extension .yml or .yaml are loaded")

	persistentFlags.StringSliceVar(&opts.TemplatesPath, "templates", []string{}, mvPathHelpMsg+
		"The value is used for locating shared template files that are parsed so they can be referenced from the config files using go text/template action tags. See documentation for more details. "+
		"hen a directory path is supplied, only files with extension .tmpl are loaded")

	// variables
	persistentFlags.StringSliceVar(&opts.VariablesPath, "variables-from", []string{}, mvPathHelpMsg+
		"The value is used for locating variable files. See documentation for more details. "+
		"When a directory path is supplied, only files with extension .vars are loaded")
	persistentFlags.StringSliceVar(&opts.Variables, "variables", nil, "A comma-separated list of variables to interpolate into the config file. Each variable must be supplied in key=value format")
	persistentFlags.BoolVar(&opts.UseEnvars, "use-envvars", false, "use env vars for interpolating into the config file")
	persistentFlags.BoolVar(&opts.NoAuditTags, "no-audit", false, "do not add buildit audit tags for created or updated resources")
	persistentFlags.StringSliceVar(&opts.Targets, "targets", nil, "A comma-separated list of buildit resource identifiers to target for this run. The target must be formatted at provider::resource. When non-empty, only the supplied resources will be built or destroyed")

	// default provider setup
	persistentFlags.StringVar(&opts.DefaultProvider.ProviderType, "default-provider-type", "default", `The default provider type for buildit; Allowed values are "env" | "shared" | "default". This is how buildit will fetch default provider credentials`)
	persistentFlags.StringVar(&opts.DefaultProvider.ConfigFile, "config-file", fmt.Sprintf("%v/.aws/config", homeDir), "The config file path when `shared` buildit provider type is specified")
	persistentFlags.StringVar(&opts.DefaultProvider.CredsFile, "creds-file", fmt.Sprintf("%v/.aws/credentials", homeDir), `The credentials file path when "shared" buildit provider type is specified`)
	persistentFlags.StringVar(&opts.DefaultProvider.Profile, "profile", "default", "The profile name when `shared` buildit provider type is specified")
	persistentFlags.StringVar(&opts.DefaultProvider.AccessKeyIdEnvVarName, "access-key-id-env-name", "AWS_ACCESS_KEY_ID", "The env var name for access key id, when `env` buildit provider type is specified")
	persistentFlags.StringVar(&opts.DefaultProvider.SecretAccessKeyVarName, "secret-access-key-env-name", "AWS_SECRET_ACCESS_KEY", "The env var name for secret access key, when env buildit provider type is specified")
	persistentFlags.StringVar(&opts.DefaultProvider.SessionTokenVarName, "session-token-env-name", "AWS_SESSION_TOKEN", "The env var name for session token, when env buildit provider type is specified")
	persistentFlags.StringVar(&opts.DefaultProvider.AccountId, "account-id", "", `If supplied, the aws account id part of the role to assume. value for "role-name" must also be provided`)
	persistentFlags.StringVar(&opts.DefaultProvider.RoleName, "role", "", `[DEPRECATED, use role-name] If supplied, the AWS IAM role name part of the role to assume. Value for "account-id" must also be provided`)
	persistentFlags.StringVar(&opts.DefaultProvider.RoleName, "role-name", "", "If supplied, the aws iam role name part of the role to assume. Value for account-id must also be provided")
	persistentFlags.StringVar(&opts.DefaultProvider.RoleArn, "role-arn", "", "If supplied, the role to assume. The role arn is preferred & overrides if account-id & role-name are also supplied")
	persistentFlags.StringVar(&opts.DefaultProvider.Region, "region", "us-east-1", "the AWS region for the default provider")

	// misc.
	persistentFlags.StringVar(&opts.LogLevel, "log-level", "info", `Sets the buildit logger level. Allowed values are "trace" | "debug" | "info" | "warn" | "error" | "fatal" | "panic"`)
	persistentFlags.BoolVar(&opts.Timestamp, "timestamp", false, `Enable timestamp in buildit log`)
	persistentFlags.IntVar(&opts.Retries, "retries", 3, `Sets the maximum retries for AWS API calls`)
	persistentFlags.IntVar(&opts.Backoff, "backoff", 20, `Sets the maximum backoff (in seconds) between retries for AWS API calls`)

	if err = persistentFlags.MarkDeprecated("override", "this feature was experimental & will not be supported, Currently only a single override file must be supplied."); err != nil {
		os.Exit(-1)
	}

	if err = persistentFlags.MarkDeprecated("include", "use --template flag instead, See documentation for more details."); err != nil {
		os.Exit(-1)
	}

	rootCmd.AddCommand(
		newApplyCommand(c),
		newAuditCommand(c),
		newDestroyCommand(c),
		newPlanCommand(c),
		newValidateCommand(c),
	)
	rootCmd.SetContext(ctx)

	return rootCmd
}

// getRootPersistentPreRunE returns a persistentPreRunE handler for root command
func getRootPersistentPreRunE(opts *config.RootOptions, c *config.Container) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {

		var err error
		ctx := cmd.Context()

		if opts.Retries > 5 || opts.Retries < 2 {
			return &fatal.Error{
				Msg: fmt.Sprintf(color.Red("❌ retries must be in range (2,5). Provided: %v"), opts.Retries),
			}
		}

		if opts.Backoff > 60 || opts.Backoff < 5 {
			return &fatal.Error{
				Msg: fmt.Sprintf(color.Red("❌ backoff must be in range (5,60). Provided: %v"), opts.Backoff),
			}
		}

		if len(opts.IncludePath) > 0 && len(opts.TemplatesPath) > 0 {
			return &fatal.Error{
				Msg: fmt.Sprint(color.Red("❌ either use includes or templates, cannot use both simultaneiously")),
			}
		}

		ctx = context.WithValue(ctx, util.RETRY, opts.Retries)
		ctx = context.WithValue(ctx, util.BACKOFF, opts.Backoff)
		cmd.SetContext(ctx)

		// set log level & formatter..
		configureLogger(opts.LogLevel, opts.Timestamp)

		// configure DD agent for traces
		// TODO: check if this is a nice way of doing this ??
		if ddAgentHost, ok := os.LookupEnv("DD_AGENT_HOST"); ok {
			if err := client.Statsd().Init(ddAgentHost); err != nil {
				return &fatal.Error{
					Msg: "failed to initialize StatsD client",
					Err: err,
				}
			}
			log.Debug("Initialized StatsD client")
		} else {
			log.Debug("DD_AGENT_HOST not set, StatsD client disabled")
		}

		log.Debugf("no-audit %v", opts.NoAuditTags)
		log.Debugf("timestamp %v", opts.Timestamp)
		log.Debugf("use-envvars %v", opts.UseEnvars)

		// load variables from all sources into viper singleton context
		var tokens map[string]any
		if tokens, err = util.LoadVariables(opts.VariablesPath, opts.UseEnvars, opts.Variables...); err != nil {
			log.Error(err)
			return err
		}

		c.RootOpts = opts
		var internal_config *config.InternalConfig
		if internal_config, err = config.Parse(ctx, *opts, tokens); err != nil {
			return &fatal.Error{
				Msg: "failed to parse buildit config",
				Err: err,
			}
		}

		c.Targets = resource.NewKeys(opts.Targets)
		c.ResourceGraph = internal_config.Graph()
		c.Tags = internal_config.Tags()
		c.Providers = internal_config.Providers()

		return nil
	}
}

// configureLogger configures the logger level & formaating
// set the supplied log level, else default `info`
func configureLogger(level string, timestamp bool) {
	var logLevel log.Level

	switch level {
	case "trace":
		logLevel = log.TraceLevel
	case "debug":
		logLevel = log.DebugLevel
	case "info":
		logLevel = log.InfoLevel
	case "warn":
		logLevel = log.WarnLevel
	case "error":
		logLevel = log.ErrorLevel
	case "fatal":
		logLevel = log.FatalLevel
	case "panic":
		logLevel = log.PanicLevel
	default:
		logLevel = log.InfoLevel
	}

	// set log level & formatter..
	log.SetLevel(logLevel)
	log.SetFormatter(&log.TextFormatter{
		DisableTimestamp: !timestamp,
	})
}
