package firehose

import (
	"context"
	"fmt"
	"strings"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/aws/aws-sdk-go-v2/aws"
	firehosetypes "github.com/aws/aws-sdk-go-v2/service/firehose/types"
)

// normalize sets default values for processing configuration, particularly for Lambda transformations.
// For Lambda processors, it ensures required parameters are set with sensible defaults.
func (p *FirehoseProcessingConfiguration) normalize(ctx context.Context, providerName string, roleArn string) {
	if p.Enabled == nil {
		p.Enabled = aws.Bool(false)
	}

	// Add default parameters for Lambda processors
	for i := range p.Processors {
		if !strings.EqualFold(p.Processors[i].Type, "Lambda") {
			continue
		}

		// Initialize Parameters map if nil
		if p.Processors[i].Parameters == nil {
			p.Processors[i].Parameters = make(map[string]string)
		}

		// Resolve RoleArn if it looks like a name (no "arn:aws:iam")
		if val, exists := p.Processors[i].Parameters["RoleArn"]; exists && val != "" {
			if !strings.HasPrefix(val, "arn:aws:") {
				arn, err := awsw.NewIAM(ctx, providerName).RoleArnForName(ctx, val)
				if err != nil {
					panic(err)
				}
				if arn != nil {
					p.Processors[i].Parameters["RoleArn"] = *arn
				}
			}
		}

		// Add RoleArn if missing - required for Lambda transformations
		if _, exists := p.Processors[i].Parameters["RoleArn"]; !exists {
			p.Processors[i].Parameters["RoleArn"] = roleArn
		}

		// Add BufferSizeInMBs if missing - default for Lambda transformations
		if _, exists := p.Processors[i].Parameters["BufferSizeInMBs"]; !exists {
			p.Processors[i].Parameters["BufferSizeInMBs"] = "1" // Updated default value
		}

		// Add BufferIntervalInSeconds if missing - default for Lambda transformations
		if _, exists := p.Processors[i].Parameters["BufferIntervalInSeconds"]; !exists {
			p.Processors[i].Parameters["BufferIntervalInSeconds"] = "60" // Updated default value
		}

		// Add NumberOfRetries if missing - default for Lambda transformations
		if _, exists := p.Processors[i].Parameters["NumberOfRetries"]; !exists {
			p.Processors[i].Parameters["NumberOfRetries"] = "3" // Default number of retries
		}
	}
}

func (p *FirehoseProcessingConfiguration) validate() []string {
	var msgs []string
	if aws.ToBool(p.Enabled) {
		if len(p.Processors) == 0 {
			msgs = append(msgs, "processingConfiguration.processors must contain at least one processor when enabled")
		}
		for i, proc := range p.Processors {
			if proc.Type == "" {
				msgs = append(msgs, fmt.Sprintf("processingConfiguration.processors[%d].type is required", i))
			}
			// Add more specific validation for Lambda if needed, but Type is the main one.
		}
	}
	return msgs
}

func (p *FirehoseProcessingConfiguration) toAWS() *firehosetypes.ProcessingConfiguration {
	if p == nil || !aws.ToBool(p.Enabled) {
		return &firehosetypes.ProcessingConfiguration{
			Enabled: aws.Bool(false),
		}
	}

	var processors []firehosetypes.Processor
	for _, proc := range p.Processors {
		var params []firehosetypes.ProcessorParameter
		for k, v := range proc.Parameters {
			params = append(params, firehosetypes.ProcessorParameter{
				ParameterName:  firehosetypes.ProcessorParameterName(k),
				ParameterValue: aws.String(v),
			})
		}
		processors = append(processors, firehosetypes.Processor{
			Type:       firehosetypes.ProcessorType(proc.Type),
			Parameters: params,
		})
	}

	return &firehosetypes.ProcessingConfiguration{
		Enabled:    p.Enabled,
		Processors: processors,
	}
}

func processingFromDescription(desc *firehosetypes.ProcessingConfiguration) *FirehoseProcessingConfiguration {
	if desc == nil {
		return nil
	}

	var processors []FirehoseProcessor
	for _, proc := range desc.Processors {
		params := make(map[string]string)
		for _, p := range proc.Parameters {
			params[string(p.ParameterName)] = aws.ToString(p.ParameterValue)
		}
		processors = append(processors, FirehoseProcessor{
			Type:       string(proc.Type),
			Parameters: params,
		})
	}

	return &FirehoseProcessingConfiguration{
		Enabled:    desc.Enabled,
		Processors: processors,
	}
}

func diffProcessingProcessors(prefix string, existing, new []FirehoseProcessor) []string {
	var diffs []string
	if len(existing) != len(new) {
		diffs = append(diffs, fmt.Sprintf("%s.Processors count: %d -> %d", prefix, len(existing), len(new)))
	}

	maxLen := len(existing)
	if len(new) > maxLen {
		maxLen = len(new)
	}

	for i := 0; i < maxLen; i++ {
		switch {
		case i >= len(existing):
			diffs = append(diffs, fmt.Sprintf("%s.Processors[%d]: added", prefix, i))
			diffs = append(diffs, diffProcessorParameters(prefix, i, nil, new[i].Parameters)...)
		case i >= len(new):
			diffs = append(diffs, fmt.Sprintf("%s.Processors[%d]: removed", prefix, i))
			diffs = append(diffs, diffProcessorParameters(prefix, i, existing[i].Parameters, nil)...)
		default:
			if existing[i].Type != new[i].Type {
				diffs = append(diffs, fmt.Sprintf("%s.Processors[%d].Type: %q -> %q", prefix, i, existing[i].Type, new[i].Type))
			}
			diffs = append(diffs, diffProcessorParameters(prefix, i, existing[i].Parameters, new[i].Parameters)...)
		}
	}

	return diffs
}

func diffProcessorParameters(prefix string, index int, existing, new map[string]string) []string {
	var diffs []string
	if len(existing) != len(new) {
		diffs = append(diffs, fmt.Sprintf("%s.Processors[%d].Parameters count: %d -> %d", prefix, index, len(existing), len(new)))
	}

	for k, v := range existing {
		newVal, exists := new[k]
		if !exists {
			diffs = append(diffs, fmt.Sprintf("%s.Processors[%d].Parameters[%s]: removed", prefix, index, k))
			continue
		}
		if v != newVal {
			diffs = append(diffs, fmt.Sprintf("%s.Processors[%d].Parameters[%s]: %q -> %q", prefix, index, k, v, newVal))
		}
	}

	for k := range new {
		if _, exists := existing[k]; !exists {
			diffs = append(diffs, fmt.Sprintf("%s.Processors[%d].Parameters[%s]: added", prefix, index, k))
		}
	}

	return diffs
}
