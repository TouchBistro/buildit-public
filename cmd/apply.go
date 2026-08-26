package cmd

import (
	"github.com/TouchBistro/buildit/config"
	"github.com/TouchBistro/goutils/fatal"
	"github.com/spf13/cobra"
)

func newApplyCommand(c *config.Container) *cobra.Command {
	return &cobra.Command{
		Use:     "apply",
		Aliases: []string{"bootstrap"},
		Args:    cobra.NoArgs,
		Short:   "Create resources on AWS",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := c.ResourceGraph.ApplyTargets(cmd.Context(), c.Targets)
			if err != nil {
				return &fatal.Error{
					Msg: "failed to create resources",
					Err: err,
				}
			}
			return nil
		},
	}
}
