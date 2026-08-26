package resource

import (
	"encoding/json"
	"fmt"

	"github.com/TouchBistro/buildit/util"
)

// This file defines the structs to map the task overrides (for runtask) API
// This is required for triggers like standalone task or scheduled (Eventbridge) ECS task targets.
// Specifically for the eventbridge task target, the AWS SDK doesnot have the corresponding fields/parameters
// and a JSON representation of the overrides, schema below from AWS Doc has to be supplied as the eventbridge
// target's input override
//
//	"overrides": {
//		"containerOverrides": [
//		   {
//			  "command": [ "string" ],
//			  "cpu": number,
//			  "environment": [
//				 {
//					"name": "string",
//					"value": "string"
//				 }
//			  ],
//			  "environmentFiles": [
//				 {
//					"type": "string",
//					"value": "string"
//				 }
//			  ],
//			  "memory": number,
//			  "memoryReservation": number,
//			  "name": "string",
//			  "resourceRequirements": [
//				 {
//					"type": "string",
//					"value": "string"
//				 }
//			  ]
//		   }
//		],
//		"cpu": "string",
//		"ephemeralStorage": {
//		   "sizeInGiB": number
//		},
//		"executionRoleArn": "string",
//		"inferenceAcceleratorOverrides": [
//		   {
//			  "deviceName": "string",
//			  "deviceType": "string"
//		   }
//		],
//		"memory": "string",
//		"taskRoleArn": "string"
//	 },
//
// TaskOverrides represents the task overrides for a runtask operation
type TaskOverrides struct {
	ContainerOverrides []TaskContainerOverrides `yaml:"containerOverrides" json:"containerOverrides,omitempty"`
	CPU                *int32                   `yaml:"cpu" json:"cpu,omitempty"`
	Memory             *int32                   `yaml:"memory" json:"memory,omitempty"`
}

// TaskContainerOverrides defines the container overrides
type TaskContainerOverrides struct {
	Name              string       `yaml:"name" json:"name"`
	Command           []string     `yaml:"command" json:"command,omitempty"`
	Environment       []TaskEnvVar `yaml:"environment" json:"environment,omitempty"`
	CPU               *int32       `yaml:"cpu" json:"cpu,omitempty"`
	Memory            *int32       `yaml:"memory" json:"memory,omitempty"`
	MemoryReservation *int32       `yaml:"memoryReservation" json:"memoryReservation,omitempty"`
	// ExecutionRoleArn string not supported
	// TaskRoleArn string not supported
}

// TaskEnvVar defines the key/value for environtment variable overrides
type TaskEnvVar struct {
	Name  string `yaml:"name" json:"name"`
	Value string `yaml:"value" json:"value"`
}

// toJsonString converts this TaskOverrides struct to Json string
func (t TaskOverrides) toJsonString() (string, error) {
	var bytes []byte
	var err error
	if bytes, err = json.Marshal(t); err != nil {
		return "", err
	}
	return string(bytes), nil
}

// parseJson parses & populates this object from the supplied overrideJson string
func (t *TaskOverrides) parseJson(overrideJson string) error {
	bytes := []byte(overrideJson)
	if err := json.Unmarshal(bytes, &t); err != nil {
		return err
	}
	return nil
}

// equals returns a boolean value, true if this is equal to the
// other TaskOverrides supplied, else false
func (t TaskOverrides) equals(other *TaskOverrides) (bool, []string) {

	this := t
	msgs := make([]string, 0)
	diff := false

	var that TaskOverrides
	if other != nil {
		that = *other
	}

	if util.CoalesceComparable(this.CPU, 0) != util.CoalesceComparable(that.CPU, 0) {
		diff = true
		msgs = append(msgs, "override task CPU is not the same")
	}

	if util.CoalesceComparable(this.Memory, 0) != util.CoalesceComparable(that.Memory, 0) {
		diff = true
		msgs = append(msgs, "override task memory is not the same")
	}

	if len(this.ContainerOverrides) != len(that.ContainerOverrides) {
		diff = true
		msgs = append(msgs, "override container settings are not the same")
	} else {

		// check all container overrides
		cMap := make(map[string]TaskContainerOverrides)
		for _, tco := range this.ContainerOverrides {
			cMap[tco.Name] = tco
		}
		for _, oco := range that.ContainerOverrides {
			if tco, ok := cMap[oco.Name]; !ok {
				diff = true
				msgs = append(msgs, fmt.Sprintf("override container name '%v' does not exist", oco.Name))
			} else {

				if util.CoalesceComparable(tco.CPU, 0) != util.CoalesceComparable(oco.CPU, 0) {
					diff = true
					msgs = append(msgs, fmt.Sprintf("override container CPU is not the same for container '%v'", oco.Name))
				}

				if util.CoalesceComparable(tco.Memory, 0) != util.CoalesceComparable(oco.Memory, 0) {
					diff = true
					msgs = append(msgs, fmt.Sprintf("override container memory is not the same for container '%v'", oco.Name))
				}

				if util.CoalesceComparable(tco.MemoryReservation, 0) != util.CoalesceComparable(oco.MemoryReservation, 0) {
					diff = true
					msgs = append(msgs, fmt.Sprintf("override container memory reservation is not the same for container '%v'", oco.Name))
				}

				// container command
				if len(tco.Command) != len(oco.Command) {
					diff = true
					msgs = append(msgs, fmt.Sprintf("override container commands are not the same for container '%v'", oco.Name))
				} else {
					for n, cmd := range tco.Command {
						if oco.Command == nil {
							diff = true
							msgs = append(msgs, fmt.Sprintf("override container commands are not the same for container '%v'", oco.Name))
						}
						if cmd != oco.Command[n] {
							diff = true
							msgs = append(msgs, fmt.Sprintf("override container command '%v' is not the same for container '%v'", cmd, oco.Name))
						}
					}
				}

				// container env
				if len(tco.Environment) != len(oco.Environment) {
					diff = true
					msgs = append(msgs, fmt.Sprintf("override container environment variables are not the same for container '%v'", oco.Name))
				} else {
					for n, env := range tco.Environment {
						if oco.Environment == nil {
							diff = true
							msgs = append(msgs, fmt.Sprintf("override container environment variables are not the same for container '%v'", oco.Name))
						}
						if env != oco.Environment[n] {
							diff = true
							msgs = append(msgs, fmt.Sprintf("override container environment variable '%v' is not the same for container '%v'", env, oco.Name))
						}
					}
				}

			}
		}
	}

	if diff {
		return false, msgs
	}

	return true, nil

}
