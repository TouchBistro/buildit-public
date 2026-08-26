package lambda

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/resource"
	"github.com/TouchBistro/buildit/util"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/pkg/errors"
)

// PolicyStatement is lambda functions resource policy statement
type PolicyStatement struct {
	StatementId   string                       `yaml:"sid" json:"Sid"`
	PrincipalJson interface{}                  `yaml:"-" json:"Principal"`
	Principal     map[string]string            `yaml:"principal"`
	Effect        string                       `yaml:"effect" json:"Effect"`
	Action        string                       `yaml:"action" json:"Action"`
	Condition     map[string]map[string]string `yaml:"condition" json:"Condition"`
	// Resource      string                       `yaml:"resource" json:"Resource"`
}

// Policy is lambda function's resource policy
type Policy struct {
	Statements []PolicyStatement `yaml:"statement" json:"Statement"`
}

func unmarshalPolicy(doc *string) (*Policy, error) {
	if doc == nil {
		return nil, errors.New("nil policy document supplied")
	}

	var policy Policy
	err := json.Unmarshal([]byte(*doc), &policy)
	if err != nil {
		return nil, err
	}

	for n, p := range policy.Statements {
		switch p.PrincipalJson.(type) {
		case map[string]interface{}:
			prMap := p.PrincipalJson.(map[string]interface{})
			if len(prMap) != 1 {
				return nil, errors.Errorf("lambda function policy contains more than 1 principal for statement %v", p.StatementId)
			}
			for k, v := range p.PrincipalJson.(map[string]interface{}) {
				switch vt := v.(type) {
				case string:
					policy.Statements[n].Principal = map[string]string{k: vt}
				default:
					return nil, errors.Errorf("lambda function policy contains invalid value for principal value %v", v)
				}
			}

		case string:
			policy.Statements[n].Principal = map[string]string{p.PrincipalJson.(string): ""}
		}
	}

	return &policy, nil
}

// canonicalPrincipal returns the principal map with its type keys folded to AWS's canonical
// casing, so a lowercase key in config (e.g. `service:`) compares equal to the canonicalized
// form GetPolicy returns.
func canonicalPrincipal(p map[string]string) map[string]string {
	if p == nil {
		return nil
	}
	out := make(map[string]string, len(p))
	for k, v := range p {
		out[resource.CanonicalPrincipalKey(k)] = v
	}
	return out
}

// equals compares this policy with the supplied other
func (d *Policy) equals(other *Policy) ([]string, resource.EqualsResult) {
	var msgs []string

	if d == nil && other != nil {
		return nil, resource.LeftZero
	} else if d != nil && other == nil {
		return nil, resource.RightZero
	} else if d != nil && other != nil {
		if len(d.Statements) != len(other.Statements) {
			msgs = append(msgs, fmt.Sprintf("number of policy statements do not match %v -> %v", len(other.Statements), len(d.Statements)))
		} else {
			for n, st := range d.Statements {
				oSt := other.Statements[n]
				if st.StatementId != oSt.StatementId {
					msgs = append(msgs, fmt.Sprintf("sids for statement %v do not match %v -> %v", n+1, oSt.StatementId, st.StatementId))
				}
				if !util.StringMap(canonicalPrincipal(st.Principal)).Equals(canonicalPrincipal(oSt.Principal)) {
					msgs = append(msgs, fmt.Sprintf("principals for statement %v do not match %v -> %v", n+1, oSt.Principal, st.Principal))
				}
				if st.Effect != oSt.Effect {
					msgs = append(msgs, fmt.Sprintf("effects for statement %v do not match %v -> %v", n+1, oSt.Effect, st.Effect))
				}
				if st.Action != oSt.Action {
					msgs = append(msgs, fmt.Sprintf("actions for statement %v do not match %v -> %v", n+1, oSt.Action, st.Action))
				}
				// if st.Resource != oSt.Resource {
				// 	msgs = append(msgs, fmt.Sprintf("resources for statement %v do not match %v -> %v\n", n+1, oSt.Resource, st.Resource))
				// 	equal = false
				// }
				if len(st.Condition) != len(oSt.Condition) {
					msgs = append(msgs, fmt.Sprintf("number of conditions for statement %v do not match %v -> %v", n+1, len(oSt.Condition), len(st.Condition)))
				}
				for ck, cv := range st.Condition {
					if ov, ok := oSt.Condition[ck]; !ok {
						msgs = append(msgs, fmt.Sprintf("missing condition key for statement %v  %v", n+1, ck))
					} else {
						if !util.StringMap(cv).Equals(ov) {
							msgs = append(msgs, fmt.Sprintf("conditions don't match for statement %v %v -> %v", n+1, ov, cv))
						}
					}
				}
			}
		}
	}

	if len(msgs) == 0 {
		return nil, resource.Equal
	}
	return msgs, resource.NotEqual
}

// apply the policy for the
func (d *Policy) apply(ctx context.Context, rctx resource.Context, functionName string, qualifer *string) error {
	client := client.Lambda(ctx, rctx.ProviderName)

	for _, s := range d.Statements {

		// principal
		var principal *string
		for k, v := range s.Principal {
			if k == "*" {
				principal = &k
			} else {
				principal = &v
			}
		}

		// conditions
		var sourceAccount *string
		var sourceArn *string
		var authType *string
		var orgId *string
		for _, c := range s.Condition {
			for k, v := range c {
				switch k {
				case "AWS:SourceAccount":
					sourceAccount = &v
				case "AWS:SourceArn":
					sourceArn = &v
				case "lambda:FunctionUrlAuthType":
					authType = &v
				case "aws:PrincipalOrgID":
					orgId = &v
				}
			}
		}

		_, err := client.AddPermission(ctx, &lambda.AddPermissionInput{
			StatementId:         aws.String(s.StatementId),
			Action:              aws.String(s.Action),
			FunctionName:        aws.String(functionName),
			Principal:           principal,
			Qualifier:           qualifer,
			SourceArn:           sourceArn,
			SourceAccount:       sourceAccount,
			PrincipalOrgID:      orgId,
			FunctionUrlAuthType: types.FunctionUrlAuthType(util.Coalesce(authType, "")),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// destroy the policies for the supplied lambda function & qualifier
func (d *Policy) destroy(ctx context.Context, rctx resource.Context, functionName string, qualifier *string) error {
	client := client.Lambda(ctx, rctx.ProviderName)

	for _, s := range d.Statements {
		_, err := client.RemovePermission(ctx, &lambda.RemovePermissionInput{
			FunctionName: aws.String(functionName),
			Qualifier:    qualifier,
			StatementId:  aws.String(s.StatementId),
		})
		if err != nil {
			return errors.Wrapf(err, "error destroying policy statement for function %v and qualifier %v", functionName, util.Coalesce(qualifier, "$LATEST"))
		}
	}
	return nil
}
