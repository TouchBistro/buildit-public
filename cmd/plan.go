package cmd

import (
	"github.com/TouchBistro/buildit/config"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/fatal"
	"github.com/spf13/cobra"
)

func newPlanCommand(c *config.Container) *cobra.Command {
	var ignoreTags []string

	command := &cobra.Command{
		Use:   "plan",
		Args:  cobra.NoArgs,
		Short: "Output a plan for buildit bootstrap command",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			normalizedIgnoreTags := util.NormalizeStringKeys(ignoreTags)
			if len(normalizedIgnoreTags) > 0 {
				ctx = util.WithIgnoreTags(ctx, normalizedIgnoreTags)
			}

			err := c.ResourceGraph.PlanTargets(ctx, c.Targets)
			if err != nil {
				return &fatal.Error{
					Msg: "failed to create plan",
					Err: err,
				}
			}
			return nil
		},
	}

	command.Flags().StringSliceVar(&ignoreTags, "ignore-tags", nil, "A comma-separated list of tag keys to ignore while computing plan diffs (plan output only; apply still evaluates all tags)")
	return command
}
