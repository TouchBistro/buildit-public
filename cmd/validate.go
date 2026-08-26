package cmd

import (
	"fmt"
	"strings"

	"github.com/TouchBistro/buildit/config"
	"github.com/TouchBistro/goutils/color"
	"github.com/TouchBistro/goutils/fatal"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newValidateCommand(c *config.Container) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Args:  cobra.NoArgs,
		Short: "Validates the buildit config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cycle := c.ResourceGraph.Cycle(); cycle != nil {
				log.Error("Dependency cycle detected between resources")
				var cyclestr []string
				for _, k := range cycle {
					cyclestr = append(cyclestr, k.String())
				}
				log.Error("Cycle: ", strings.Join(cyclestr, " -> "))
				return &fatal.Error{
					Msg: fmt.Sprintf(color.Red("❌ %s is invalid"), c.RootOpts.ConfigPath),
				}
			}

			log.Infof(color.Green("✅ %s is valid"), c.RootOpts.ConfigPath)
			return nil
		},
	}
}
