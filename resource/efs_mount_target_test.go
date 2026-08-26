package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEFSMountTarget_validate(t *testing.T) {
	tests := []struct {
		name     string
		mt       EFSMountTarget
		wantMsgs []string
	}{
		{
			name: "valid",
			mt:   EFSMountTarget{SubnetName: "private-a"},
		},
		{
			name: "valid with ip",
			mt:   EFSMountTarget{SubnetName: "private-a", IpAddress: "10.0.1.50"},
		},
		{
			name:     "missing subnet name",
			mt:       EFSMountTarget{},
			wantMsgs: []string{"subnetName is required"},
		},
		{
			name:     "bad ip address",
			mt:       EFSMountTarget{SubnetName: "private-a", IpAddress: "999.1.1.1"},
			wantMsgs: []string{"not a valid IP address"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msgs := tc.mt.validate()
			if len(tc.wantMsgs) == 0 {
				assert.Empty(t, msgs)
				return
			}
			for _, want := range tc.wantMsgs {
				assert.Contains(t, joinMsgs(msgs), want)
			}
		})
	}
}

func TestMountTargetSGChanged(t *testing.T) {
	// empty desired leaves security groups unmanaged
	assert.False(t, mountTargetSGChanged([]string{"sg-1"}, nil))
	// order-independent equality means no change
	assert.False(t, mountTargetSGChanged([]string{"sg-1", "sg-2"}, []string{"sg-2", "sg-1"}))
	// a genuine difference is a change
	assert.True(t, mountTargetSGChanged([]string{"sg-1"}, []string{"sg-2"}))
}

func TestStringSetEqual(t *testing.T) {
	assert.True(t, stringSetEqual(nil, nil))
	assert.True(t, stringSetEqual([]string{"a", "b"}, []string{"b", "a"}))
	assert.False(t, stringSetEqual([]string{"a"}, []string{"a", "b"}))
	assert.False(t, stringSetEqual([]string{"a", "a"}, []string{"a", "b"}))
}

func joinMsgs(msgs []string) string {
	out := ""
	for _, m := range msgs {
		out += m + "\n"
	}
	return out
}
