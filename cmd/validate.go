package cmd

import (
	"github.com/TouchBistro/buildit/config"
	"github.com/TouchBistro/goutils/color"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newValidateCommand(c *config.Container) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Args:  cobra.NoArgs,
		Short: "Validates the buildit config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Loading the config already failed on any validation error or dependency
			// cycle, so a config that got this far is valid.
			log.Infof(color.Green("✅ %s is valid"), c.RootOpts.ConfigPath)
			return nil
		},
	}
}
