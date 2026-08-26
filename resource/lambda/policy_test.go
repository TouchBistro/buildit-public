package lambda

import (
	"strings"
	"testing"

	"github.com/TouchBistro/buildit/resource"
)

func TestPolicyEquals(t *testing.T) {
	tests := []struct {
		name       string
		left       *Policy
		right      *Policy
		wantResult resource.EqualsResult
		wantMsg    string
	}{
		{
			name:       "both nil",
			left:       nil,
			right:      nil,
			wantResult: resource.Equal,
		},
		{
			name:       "left nil",
			left:       nil,
			right:      basePolicy(),
			wantResult: resource.LeftZero,
		},
		{
			name:       "right nil",
			left:       basePolicy(),
			right:      nil,
			wantResult: resource.RightZero,
		},
		{
			name:       "equal",
			left:       basePolicy(),
			right:      basePolicy(),
			wantResult: resource.Equal,
		},
		{
			name:       "statement count mismatch",
			left:       basePolicy(),
			right:      policyWithSecondStatement(),
			wantResult: resource.NotEqual,
			wantMsg:    "number of policy statements do not match",
		},
		{
			name:       "statement id mismatch",
			left:       basePolicy(),
			right:      policyWithStatementID("other-sid"),
			wantResult: resource.NotEqual,
			wantMsg:    "sids for statement 1 do not match",
		},
		{
			name:       "principal mismatch",
			left:       basePolicy(),
			right:      policyWithPrincipal(map[string]string{"Service": "sns.amazonaws.com"}),
			wantResult: resource.NotEqual,
			wantMsg:    "principals for statement 1 do not match",
		},
		{
			name:       "principal type key casing is insignificant",
			left:       basePolicy(),
			right:      policyWithPrincipal(map[string]string{"service": "events.amazonaws.com"}),
			wantResult: resource.Equal,
		},
		{
			name:       "different principal type keys are not equal",
			left:       basePolicy(),
			right:      policyWithPrincipal(map[string]string{"Federated": "events.amazonaws.com"}),
			wantResult: resource.NotEqual,
			wantMsg:    "principals for statement 1 do not match",
		},
		{
			name:       "effect mismatch",
			left:       basePolicy(),
			right:      policyWithEffect("Deny"),
			wantResult: resource.NotEqual,
			wantMsg:    "effects for statement 1 do not match",
		},
		{
			name:       "action mismatch",
			left:       basePolicy(),
			right:      policyWithAction("lambda:InvokeFunctionUrl"),
			wantResult: resource.NotEqual,
			wantMsg:    "actions for statement 1 do not match",
		},
		{
			name:       "condition count mismatch",
			left:       policyWithAdditionalCondition(),
			right:      basePolicy(),
			wantResult: resource.NotEqual,
			wantMsg:    "number of conditions for statement 1 do not match",
		},
		{
			name:       "missing condition key",
			left:       basePolicy(),
			right:      policyWithDifferentConditionKey(),
			wantResult: resource.NotEqual,
			wantMsg:    "missing condition key for statement 1",
		},
		{
			name:       "condition value mismatch",
			left:       basePolicy(),
			right:      policyWithConditionValue("arn:aws:events:us-east-1:123456789012:rule/other"),
			wantResult: resource.NotEqual,
			wantMsg:    "conditions don't match for statement 1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			msgs, result := test.left.equals(test.right)

			if result != test.wantResult {
				t.Fatalf("expected result %v, got %v", test.wantResult, result)
			}

			if test.wantMsg == "" {
				if len(msgs) != 0 {
					t.Fatalf("expected no messages, got %v", msgs)
				}
				return
			}

			found := false
			for _, msg := range msgs {
				if strings.Contains(msg, test.wantMsg) {
					found = true
					break
				}
			}

			if !found {
				t.Fatalf("expected message containing %q, got %v", test.wantMsg, msgs)
			}
		})
	}
}

func basePolicy() *Policy {
	return &Policy{
		Statements: []PolicyStatement{
			{
				StatementId: "sid-1",
				Principal: map[string]string{
					"Service": "events.amazonaws.com",
				},
				Effect: "Allow",
				Action: "lambda:InvokeFunction",
				Condition: map[string]map[string]string{
					"ArnLike": {
						"AWS:SourceArn": "arn:aws:events:us-east-1:123456789012:rule/myrule",
					},
				},
			},
		},
	}
}

func policyWithSecondStatement() *Policy {
	policy := basePolicy()
	policy.Statements = append(policy.Statements, PolicyStatement{
		StatementId: "sid-2",
		Principal: map[string]string{
			"AWS": "arn:aws:iam::123456789012:root",
		},
		Effect: "Allow",
		Action: "lambda:InvokeFunction",
		Condition: map[string]map[string]string{
			"StringEquals": {
				"AWS:SourceAccount": "123456789012",
			},
		},
	})
	return policy
}

func policyWithStatementID(statementID string) *Policy {
	policy := basePolicy()
	policy.Statements[0].StatementId = statementID
	return policy
}

func policyWithPrincipal(principal map[string]string) *Policy {
	policy := basePolicy()
	policy.Statements[0].Principal = principal
	return policy
}

func policyWithEffect(effect string) *Policy {
	policy := basePolicy()
	policy.Statements[0].Effect = effect
	return policy
}

func policyWithAction(action string) *Policy {
	policy := basePolicy()
	policy.Statements[0].Action = action
	return policy
}

func policyWithAdditionalCondition() *Policy {
	policy := basePolicy()
	policy.Statements[0].Condition["StringEquals"] = map[string]string{
		"AWS:SourceAccount": "123456789012",
	}
	return policy
}

func policyWithDifferentConditionKey() *Policy {
	policy := basePolicy()
	policy.Statements[0].Condition = map[string]map[string]string{
		"StringEquals": {
			"AWS:SourceAccount": "123456789012",
		},
	}
	return policy
}

func policyWithConditionValue(value string) *Policy {
	policy := basePolicy()
	policy.Statements[0].Condition["ArnLike"]["AWS:SourceArn"] = value
	return policy
}
