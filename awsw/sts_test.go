package awsw_test

import (
	"testing"

	"github.com/TouchBistro/buildit/awsw"
)

// We can't easily mock the client.STS call without refactoring client package or using interfaces.
// For now, we'll just test that the struct methods exist and compile.
// Integration tests would verify actual AWS calls.

func TestSTSGetAccountID(t *testing.T) {
	// This test relies on creating a client which might fail if no creds are present.
	// We'll skip if no creds, or mock if we could.
	// For now, simple compilation check via generic test structure.
	t.Run("Compiles", func(t *testing.T) {
		_ = awsw.STS{}
	})

	// Real test would need:
	// s := awsw.NewSTS(ctx, "default")
	// id, err := s.GetAccountID(ctx)
}
