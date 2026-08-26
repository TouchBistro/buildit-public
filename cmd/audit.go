package cmd

import (
	"fmt"
	"strings"

	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/config"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newAuditCommand(c *config.Container) *cobra.Command {
	return &cobra.Command{
		Use:   "audit",
		Args:  cobra.NoArgs,
		Short: "Output a list of resources that have the incorrect audit tag",
		RunE: func(cmd *cobra.Command, args []string) error {
			audit := c.Tags["audit"]
			tagFilters := []types.TagFilter{}
			for key, value := range c.Tags {
				if key == "audit" {
					continue
				}
				tagFilters = append(tagFilters, types.TagFilter{
					Key:    &key,
					Values: []string{value},
				})
			}

			// TaskDefinition snowflake

			for _, p := range c.Providers {
				taskDefinitions := map[string]bool{}
				log.Info("Auditing provider's resources\t", fmt.Sprintf("%v=%v", color.Cyan("provider"), p.Name))
				// Initialize an AWS config and instantiate a resourcegroupstaggingapi client
				rgtaClient := client.ResourceGroupsTaggingAPI(cmd.Context(), p.Name)
				ecsClient := client.ECS(cmd.Context(), p.Name)

				var nextToken *string
				for {
					resp, err := rgtaClient.GetResources(cmd.Context(),
						&resourcegroupstaggingapi.GetResourcesInput{
							TagFilters:      tagFilters,
							PaginationToken: nextToken,
						},
					)
					if err != nil {
						log.Error("Unable to fetch resources", err)
						continue
					}
					if len(resp.ResourceTagMappingList) > 0 {
						log.Info("Found resources to validate\t", fmt.Sprintf("%v=%v", color.Cyan("count"), len(resp.ResourceTagMappingList)))
					} else {
						log.Info("No resources found to validate")
					}

					for _, mapping := range resp.ResourceTagMappingList {
						// Get audit tag
						pass := false
						for _, tag := range mapping.Tags {
							if tag.Key != nil && tag.Value != nil && *tag.Key == "audit" && *tag.Value == audit {
								pass = true
								break
							}
						}
						if strings.Contains(*mapping.ResourceARN, ":task-definition/") {
							// Check if we have 1 active, tagged task definition as we could have several from the same family.
							baseArn := *mapping.ResourceARN
							lastColonIndex := strings.LastIndex(baseArn, ":")
							if lastColonIndex != -1 {
								baseArn = baseArn[:lastColonIndex]
							}
							if !taskDefinitions[baseArn] {
								taskDefinitions[baseArn] = pass
							}
							continue
						} else if strings.Contains(*mapping.ResourceARN, ":task/") {
							// Check if this task is running
							// arn:aws:ecs:us-east-1:123456789012:task/example-cluster/00000000000000000000000000000000
							parts := strings.Split(*mapping.ResourceARN, "/")
							if len(parts) > 2 {
								clusterName := parts[1]
								taskId := parts[2]

								taskResp, err := ecsClient.DescribeTasks(cmd.Context(), &ecs.DescribeTasksInput{
									Tasks:   []string{taskId},
									Cluster: &clusterName,
								})
								if err == nil && len(taskResp.Tasks) > 0 && *taskResp.Tasks[0].LastStatus == "STOPPED" {
									continue
								}
							}
						}
						if !pass {
							log.Warn("Found resource to audit\t", fmt.Sprintf("%v=%v", color.Yellow("arn"), *mapping.ResourceARN))
						}
					}
					if resp.PaginationToken == nil || *resp.PaginationToken == "" {
						break
					}
					nextToken = resp.PaginationToken
				}
				for k, v := range taskDefinitions {
					if !v {
						log.Warn("Found resource to audit\t", fmt.Sprintf("%v=%v", color.Yellow("arn"), k))
					}
				}

			}
			return nil
		},
	}
}
