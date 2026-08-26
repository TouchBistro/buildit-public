package cmd

import (
	"github.com/TouchBistro/buildit/config"
	"github.com/TouchBistro/goutils/fatal"
	"github.com/spf13/cobra"
)

func newDestroyCommand(c *config.Container) *cobra.Command {
	return &cobra.Command{
		Use:   "destroy",
		Args:  cobra.NoArgs,
		Short: "Remove resources from AWS",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := c.ResourceGraph.DestroyTargets(cmd.Context(), c.Targets)
			if err != nil {
				return &fatal.Error{
					Msg: "failed to destroy resources",
					Err: err,
				}
			}
			return nil
		},
	}
}
