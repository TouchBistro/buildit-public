package resource

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	FixedResponseAction    = string(elbv2types.ActionTypeEnumFixedResponse)    // "fixed-response"
	ForwardAction          = string(elbv2types.ActionTypeEnumForward)          // "forward"
	RedirectAction         = string(elbv2types.ActionTypeEnumRedirect)         // "redirect"
	RedirectHttpsAction    = "redirect-https"                                  // buildit only
	AuthenticateOidcAction = string(elbv2types.ActionTypeEnumAuthenticateOidc) // "authenticate-oidc"
)

const (
	HostHeaderCondition        = "host-header"
	PathPatternCondition       = "path-pattern"
	HttpHeaderCondition        = "http-header"
	HttpRequestMethodCondition = "http-request-method"
	HttpQueryStringCondition   = "query-string"
	HttpSourceIpCondition      = "source-ip"
)

const (
	DefaultSessionCookieName string = "AWSELBAuthSessionCookie"
)

const (
	DefaultSessionTimeoutSec int64 = 604800
)

// LBTargetGroupRef represents the target group forward for a rule
type LBTargetGroupRef struct {
	Name   string `yaml:"name"`
	Weight *int32 `yaml:"weight"`
}

// AuthenticateOidcConfig represents config parameters for LBAction = `authenticate-oidc“
type AuthenticateOidcConfig struct {
	Issuer                   string            `yaml:"issuer"`
	AuthrEndpoint            string            `yaml:"authorizationEndpoint"`
	TokenEndpiont            string            `yaml:"tokenEndpoint"`
	UserInfoEndpoint         string            `yaml:"userInfoEndpoint"`
	ClientId                 string            `yaml:"clientId"`
	ClientSecret             string            `yaml:"clientSecret"`
	SessionCookieName        *string           `yaml:"sessionCookie"`         // defaults to AWSELBAuthSessionCookie
	SessionTimeout           *int64            `yaml:"sessionTimeoutSeconds"` // defaults to 604800
	Scope                    string            `yaml:"scope"`
	Params                   map[string]string `yaml:"params"`
	OnUnauthenticatedRequest string            `yaml:"onUnauthenticatedRequest"`
}

// LBAction represents the Actions (then)
type LBAction struct {
	ActionType             string                  `yaml:"do"` //"redirect, forward, fixed-response", "authenticate-oidc"
	Order                  int32                   `yaml:"-"`
	FxContentType          string                  `yaml:"fixedContentType"`
	FxMessageBody          string                  `yaml:"fixedMessageBody"`
	FxStatusCode           string                  `yaml:"fixedStatusCode"`
	FwStickiness           int32                   `yaml:"forwardStickinessDuration"` //0 disabled, +ve value seconds
	FwTargetGroups         []LBTargetGroupRef      `yaml:"forwardTargetGroups"`
	ReHost                 string                  `yaml:"redirectHost"`
	RePath                 string                  `yaml:"redirectPath"`
	RePort                 string                  `yaml:"redirectPort"`
	ReProtocol             string                  `yaml:"redirectProtocol"`
	ReQuery                string                  `yaml:"redirectQuery"`
	ReStatusCode           string                  `yaml:"redirectStatusCode"`
	AuthenticateOidcConfig *AuthenticateOidcConfig `yaml:"authenticateOidcConfig"`
}

// compares this to the provided *elbv2.Action
func (a LBAction) equals(ctx context.Context, rctx Context, action *elbv2types.Action) bool {

	//empty/nil
	if action == nil {
		return a.ActionType == ""
	}

	//order
	if a.Order != *action.Order {
		return false
	}

	//fixed-response
	switch a.ActionType {
	case FixedResponseAction: //fixed-response
		if action.Type != elbv2types.ActionTypeEnumFixedResponse {
			return false
		}
		if fx := action.FixedResponseConfig; fx == nil ||
			a.FxContentType != *fx.ContentType ||
			a.FxMessageBody != *fx.MessageBody ||
			a.FxStatusCode != *fx.StatusCode {
			return false
		}
	case ForwardAction: //forward
		fwd := action.ForwardConfig

		if fwd == nil {
			return false
		}

		if len(a.FwTargetGroups) != len(fwd.TargetGroups) {
			return false
		}

		//check target groups
		tgMap := make(map[string]struct{})
		for _, tgr := range a.FwTargetGroups {
			arn, err := targetGroupARNFromName(ctx, rctx.ProviderName, tgr.Name)
			if err != nil {
				return false //TODO: should we return error ?
			}
			//fill map with tg tuple -> tg ref
			k := fmt.Sprintf("%v:%v", *arn, util.CoalesceComparable(tgr.Weight, 101))
			tgMap[k] = struct{}{}
		}
		for _, tgt := range fwd.TargetGroups {
			key := fmt.Sprintf("%v:%v", *tgt.TargetGroupArn, util.CoalesceComparable(tgt.Weight, 101))
			if _, ok := tgMap[key]; ok {
				delete(tgMap, key)
			} else {
				return false //we found an existing tg tuple that doesn't match
			}
		}
		//after all done, we see if any new tg refs were unmatched
		if len(tgMap) > 0 {
			return false
		}
		//stickiness
		if a.FwStickiness > 0 && //require(enabled)
			(fwd.TargetGroupStickinessConfig == nil || //found(disabled), OR
				!*fwd.TargetGroupStickinessConfig.Enabled ||
				a.FwStickiness != *fwd.TargetGroupStickinessConfig.DurationSeconds) { //found(enabled), but differnt config
			return false
		}
		if a.FwStickiness == 0 && //require(disabled)
			(fwd.TargetGroupStickinessConfig != nil && *fwd.TargetGroupStickinessConfig.Enabled) { //found(enabled)
			return false
		}
	case RedirectAction: //redirect
		if r := action.RedirectConfig; r == nil ||
			a.ReHost != *r.Host || a.RePath != *r.Path || a.RePort != *r.Port ||
			!strings.EqualFold(a.ReProtocol, *r.Protocol) || a.ReQuery != *r.Query || a.ReStatusCode != string(r.StatusCode) {
			return false
		}
	case RedirectHttpsAction: //https-redirect
		if r := action.RedirectConfig; r == nil ||
			*r.Host != "#{host}" || *r.Path != "/#{path}" || *r.Port != "443" ||
			HTTPS != *r.Protocol || *r.Query != "#{query}" || string(r.StatusCode) != "HTTP_301" {
			return false
		}
	case AuthenticateOidcAction: //authenticate-oidc
		r := action.AuthenticateOidcConfig
		c := a.AuthenticateOidcConfig
		if r == nil && c == nil {
			return true // equal
		} else if r == nil && c != nil || c == nil && r != nil {
			return false
		} else {
			// both r & c not nil
			if util.Coalesce(r.Issuer, "") != c.Issuer ||
				util.Coalesce(r.AuthorizationEndpoint, "") != c.AuthrEndpoint ||
				util.Coalesce(r.TokenEndpoint, "") != c.TokenEndpiont ||
				util.Coalesce(r.UserInfoEndpoint, "") != c.UserInfoEndpoint ||
				util.Coalesce(r.ClientId, "") != c.ClientId ||
				util.Coalesce(r.Scope, "") != c.Scope ||
				util.Coalesce(r.SessionCookieName, DefaultSessionCookieName) != util.Coalesce(c.SessionCookieName, DefaultSessionCookieName) ||
				util.CoalesceComparable(r.SessionTimeout, DefaultSessionTimeoutSec) != util.CoalesceComparable(c.SessionTimeout, DefaultSessionTimeoutSec) ||
				!util.StringMap(r.AuthenticationRequestExtraParams).Equals(c.Params) ||
				r.OnUnauthenticatedRequest != elbv2types.AuthenticateOidcActionConditionalBehaviorEnum(c.OnUnauthenticatedRequest) {
				return false
			}

			if util.Coalesce(r.ClientSecret, "") != c.ClientSecret {
				log.Warn(color.Yellow("when using 'authenticate-oidc' action, existing client secret is not returned by aws, a diff is always shown"))
				return false
			}
		}
	}

	return true
}

// converts this to an elbv2.Action ptr
func (a LBAction) toAwsAction(ctx context.Context, rctx Context) (*elbv2types.Action, error) {
	switch a.ActionType {
	case FixedResponseAction:
		return &elbv2types.Action{
			Type:  elbv2types.ActionTypeEnumFixedResponse, //aws.String(elbv2.ActionTypeEnumFixedResponse),
			Order: aws.Int32(a.Order),
			FixedResponseConfig: &elbv2types.FixedResponseActionConfig{
				ContentType: aws.String(a.FxContentType),
				MessageBody: aws.String(a.FxMessageBody),
				StatusCode:  aws.String(a.FxStatusCode),
			},
		}, nil
	case ForwardAction:

		var stickinessConfig *elbv2types.TargetGroupStickinessConfig = nil
		if a.FwStickiness > 0 {
			stickinessConfig = &elbv2types.TargetGroupStickinessConfig{
				Enabled:         aws.Bool(true),
				DurationSeconds: aws.Int32(a.FwStickiness),
			}
		}

		var targetGroupTuples []elbv2types.TargetGroupTuple
		for _, t := range a.FwTargetGroups {
			targetGroupArn, err := targetGroupNametoArn(ctx, rctx, t.Name)
			if err != nil {
				return nil, errors.Errorf("cannot find target group %v", t.Name)
			}
			targetGroupTuples = append(targetGroupTuples, elbv2types.TargetGroupTuple{
				TargetGroupArn: aws.String(targetGroupArn),
				Weight:         t.Weight,
			})
		}

		return &elbv2types.Action{
			Type:  elbv2types.ActionTypeEnumForward, //aws.String(elbv2.ActionTypeEnumForward),
			Order: aws.Int32(a.Order),
			ForwardConfig: &elbv2types.ForwardActionConfig{
				TargetGroupStickinessConfig: stickinessConfig,
				TargetGroups:                targetGroupTuples,
			},
		}, nil
	case RedirectAction:
		return &elbv2types.Action{
			Type:  elbv2types.ActionTypeEnumRedirect, //aws.String(elbv2.ActionTypeEnumRedirect),
			Order: aws.Int32(a.Order),
			RedirectConfig: &elbv2types.RedirectActionConfig{
				Host:       aws.String(a.ReHost),
				Path:       aws.String(a.RePath),
				Port:       aws.String(a.RePort),
				Protocol:   aws.String(strings.ToUpper(a.ReProtocol)),
				Query:      aws.String(a.ReQuery),
				StatusCode: elbv2types.RedirectActionStatusCodeEnum(a.ReStatusCode),
			},
		}, nil
	case RedirectHttpsAction:
		return &elbv2types.Action{
			Type:  elbv2types.ActionTypeEnumRedirect, //aws.String(elbv2.ActionTypeEnumRedirect),
			Order: aws.Int32(a.Order),
			RedirectConfig: &elbv2types.RedirectActionConfig{
				Host:       aws.String("#{host}"),
				Path:       aws.String("/#{path}"),
				Port:       aws.String("443"),
				Protocol:   aws.String(HTTPS),
				Query:      aws.String("#{query}"),
				StatusCode: elbv2types.RedirectActionStatusCodeEnumHttp301,
			},
		}, nil
	case AuthenticateOidcAction:
		if a.AuthenticateOidcConfig == nil {
			return nil, errors.Errorf("authenticate oidc config not supplied")
		}
		c := a.AuthenticateOidcConfig
		return &elbv2types.Action{
			Type:  elbv2types.ActionTypeEnumAuthenticateOidc,
			Order: aws.Int32(a.Order),
			AuthenticateOidcConfig: &elbv2types.AuthenticateOidcActionConfig{
				AuthorizationEndpoint:            aws.String(c.AuthrEndpoint),
				ClientId:                         aws.String(c.ClientId),
				ClientSecret:                     aws.String(c.ClientSecret),
				UseExistingClientSecret:          aws.Bool(false), // we'll always be supplying this value
				Issuer:                           aws.String(c.Issuer),
				TokenEndpoint:                    aws.String(c.TokenEndpiont),
				UserInfoEndpoint:                 aws.String(c.UserInfoEndpoint),
				AuthenticationRequestExtraParams: c.Params,
				OnUnauthenticatedRequest:         elbv2types.AuthenticateOidcActionConditionalBehaviorEnum(c.OnUnauthenticatedRequest),
				Scope:                            aws.String(c.Scope),
				SessionCookieName:                c.SessionCookieName,
				SessionTimeout:                   c.SessionTimeout,
			},
		}, nil

	}
	return nil, errors.Errorf("cannot convert %v to action", a.ActionType)
}

// LBCondition represents the Rule conditions (if)
type LBCondition struct {
	ConditionType string   `yaml:"the"` //host-header, path-pattern, http-header
	HeaderName    string   `yaml:"named"`
	Values        []string `yaml:"is"`
}

// checks if this is equal to the specified *elbv2.RuleCondition
func (c LBCondition) equals(condition *elbv2types.RuleCondition) bool {

	// diff unsupported rule first
	if condition.Field == nil || *condition.Field == HttpQueryStringCondition {
		return false
	}

	//nil
	if condition == nil {
		return c.ConditionType == ""
	}

	//type
	if c.ConditionType != *condition.Field {
		return false
	}

	//http header condition
	if c.ConditionType == HttpHeaderCondition &&
		c.HeaderName != *condition.HttpHeaderConfig.HttpHeaderName {
		return false
	}

	var conditionValues = condition.Values

	switch *condition.Field {

	// Path Pattern condition values can be either in PathPatternConfig.values or values
	case PathPatternCondition:
		if len(condition.PathPatternConfig.Values) > len(condition.Values) {
			conditionValues = condition.PathPatternConfig.Values
		}
		// Host Header condition value can be either in HostHeaderConfig.values or values
	case HostHeaderCondition:
		if len(condition.HostHeaderConfig.Values) > len(condition.Values) {
			conditionValues = condition.HostHeaderConfig.Values
		}

	case HttpHeaderCondition:
		conditionValues = condition.HttpHeaderConfig.Values

	case HttpRequestMethodCondition:
		conditionValues = condition.HttpRequestMethodConfig.Values

	case HttpSourceIpCondition:
		conditionValues = condition.SourceIpConfig.Values

	case HttpQueryStringCondition:
		//Not Supporte dnow
		//conditionValues = condition.QueryStringConfig.
	default:
		conditionValues = condition.Values
	}

	return util.StringSliceEquals(c.Values, conditionValues)
}

// converts this to an elbv2.RuleCondition ptr
func (c LBCondition) toAwsRuleCondition() (*elbv2types.RuleCondition, error) {
	switch c.ConditionType {
	case HostHeaderCondition:
		return &elbv2types.RuleCondition{
			Field:  aws.String(HostHeaderCondition),
			Values: c.Values,
		}, nil
	case PathPatternCondition:
		return &elbv2types.RuleCondition{
			Field:  aws.String(PathPatternCondition),
			Values: c.Values,
		}, nil
	case HttpHeaderCondition:
		return &elbv2types.RuleCondition{
			Field: aws.String(HttpHeaderCondition),
			HttpHeaderConfig: &elbv2types.HttpHeaderConditionConfig{
				HttpHeaderName: aws.String(c.HeaderName),
				Values:         c.Values,
			},
		}, nil
	case HttpSourceIpCondition:
		return &elbv2types.RuleCondition{
			Field: aws.String(HttpSourceIpCondition),
			SourceIpConfig: &elbv2types.SourceIpConditionConfig{
				Values: c.Values,
			},
		}, nil
	}
	return nil, errors.Errorf("cannot convert to rule condition, %v not supported", c.ConditionType)
}

// LBRule represents a load balancer rule, either default action or configured
type LBRule struct {
	Priority         int32             `yaml:"-"`
	Actions          []LBAction        `yaml:"then"`
	Conditions       []LBCondition     `yaml:"if"`
	ListenerArn      string            `yaml:"-"`
	ListenerName     string            `yaml:"-"`
	LoadBalancerName string            `yaml:"-"`
	RuleArn          string            `yaml:"-"`
	GlobalTags       map[string]string `yaml:"-"`
}

// equals checks if the LBRule is equal to the elbv2.Rule
func (l LBRule) equals(ctx context.Context, rctx Context, rule *elbv2types.Rule) bool {

	//check contidions
	if len(l.Conditions) != len(rule.Conditions) {
		return false
	}

	// there is no ordering for LB rule conditions.
	// which means we need match new rules with all
	// existing rule)
	//
	for _, new_rc := range l.Conditions {
		var found bool
		for _, existing_rc := range rule.Conditions {
			found = new_rc.ConditionType == *existing_rc.Field &&
				new_rc.equals(&existing_rc)
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}

	//check actions
	if len(l.Actions) != len(rule.Actions) {
		return false
	}

	// order existing action by Order
	sort.Slice(rule.Actions, func(i, j int) bool {
		return *rule.Actions[i].Order < *rule.Actions[j].Order
	})

	for i := 0; i < len(l.Actions); i++ {
		l.Actions[i].Order = int32(i + 1)
		if !l.Actions[i].equals(ctx, rctx, &rule.Actions[i]) {
			return false
		}
	}

	return true
}

func (l *LBRule) Normalize(ctx context.Context, rctx Context) {

	for _, a := range l.Actions {
		if a.ActionType == AuthenticateOidcAction {
			if a.AuthenticateOidcConfig == nil {
				continue
			}

			id := a.AuthenticateOidcConfig.ClientId
			val, err := awsw.NewSecretsManager(ctx, rctx.ProviderName).GetValueBySecretId(ctx, id)
			if err != nil {
				log.Warnf("error fetching client secret from %v", id)
				panic(err)
			}
			a.AuthenticateOidcConfig.ClientId = *val

			sec := a.AuthenticateOidcConfig.ClientSecret
			val, err = awsw.NewSecretsManager(ctx, rctx.ProviderName).GetValueBySecretId(ctx, sec)
			if err != nil {
				log.Warnf("error fetching client secret from %v", sec)
				panic(err)
			}
			a.AuthenticateOidcConfig.ClientSecret = *val
		}
	}

}

// Identifier represents the unique id of a listener
func (l LBRule) Identifier() string {
	return strconv.FormatInt(int64(l.Priority), 10)
}

// Validate the listener
func (l LBRule) Validate(ctx context.Context) error {
	var errMessages []string
	for _, c := range l.Conditions {
		switch c.ConditionType {
		case HostHeaderCondition, PathPatternCondition, HttpHeaderCondition, HttpRequestMethodCondition, HttpSourceIpCondition:
			//no-op
		default:
			errMessages = append(errMessages,
				fmt.Sprintf("Rule Condition of type %v currently not supported by buildit; Rule %v", c, l.Identifier()))
		}
	}
	for _, a := range l.Actions {
		switch a.ActionType {
		case FixedResponseAction, ForwardAction, RedirectAction, RedirectHttpsAction:
			//no-op
		case AuthenticateOidcAction:

		default:
			errMessages = append(errMessages,
				fmt.Sprintf("Rule Action of type %v currently not supported by buildit; Rule %v", a, l.Identifier()))
		}
	}

	return &ValidationError{
		ResourceIdentifier: l.Identifier(),
		ResourceType:       "Load Balancer Listener",
		Messages:           errMessages,
	}
}

// update an existing rule
func (l LBRule) update(ctx context.Context, rctx Context, existing *elbv2types.Rule) error {

	actions, err := l.toAwsActions(ctx, rctx)
	if err != nil {
		return errors.Wrap(err, "error parsing rule actions")
	}

	conditions, err := l.toAwsRuleConditions()
	if err != nil {
		return errors.Wrap(err, "error parsing rule condition")
	}

	_, err = client.ELB(ctx, rctx.ProviderName).ModifyRule(ctx, &elbv2.ModifyRuleInput{
		RuleArn:    existing.RuleArn,
		Actions:    actions,
		Conditions: conditions,
	})

	if err != nil {
		return errors.Wrapf(err, "error updating rule for at priority %v", *existing.Priority)
	}

	log.WithFields(log.Fields{
		"LoadBalancer": l.LoadBalancerName,
		"Listener":     l.ListenerName,
		"Priority":     *existing.Priority,
	}).Info(color.Yellow("Load balancer rule updated"))

	return nil

}

// Apply makes a load balancer listener
func (l LBRule) Apply(ctx context.Context, rctx Context) error {

	client := client.ELB(ctx, rctx.ProviderName)

	var actions []elbv2types.Action
	var conditions []elbv2types.RuleCondition
	var err error

	actions, err = l.toAwsActions(ctx, rctx)
	if err != nil {
		return errors.Wrap(err, "error parsing rule actions")
	}

	conditions, err = l.toAwsRuleConditions()
	if err != nil {
		return errors.Wrap(err, "error parsing rule condition")
	}

	var tags []elbv2types.Tag
	for k, v := range l.GlobalTags {
		tag := elbv2types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		}
		tags = append(tags, tag)
	}

	_, err = client.CreateRule(ctx, &elbv2.CreateRuleInput{
		Actions:     actions,
		Conditions:  conditions,
		ListenerArn: aws.String(l.ListenerArn),
		Priority:    aws.Int32(l.Priority),
		Tags:        tags,
	})

	if err != nil {
		return errors.Wrap(err, "error creating load balancer listener")
	}

	//ruleArn := out.Rules[0].RuleArn
	log.WithFields(log.Fields{
		"LoadBalancer": l.LoadBalancerName,
		"Listener":     l.ListenerName,
		"Priority":     l.Priority,
	}).Info(color.Green("load balancer rule created"))

	return nil
}

// Destroy removes the rule
func (l LBRule) Destroy(ctx context.Context, rctx Context) error {
	_, err := client.ELB(ctx, rctx.ProviderName).DeleteRule(ctx, &elbv2.DeleteRuleInput{
		RuleArn: &l.RuleArn,
	})
	if err != nil {
		return errors.Wrapf(err, "error removing listener rule %v", l.Priority)
	}
	log.WithFields(log.Fields{
		"LoadBalancer": l.LoadBalancerName,
		"Listener":     l.ListenerName,
		"Priority":     l.Priority,
	}).Info(color.Red("load balancer rule deleted"))

	return nil
}

// toAwsActions converts a []LBActions to []*elbv2.Action
func (l LBRule) toAwsActions(ctx context.Context, rctx Context) ([]elbv2types.Action, error) {
	var actions []elbv2types.Action
	for n, a := range l.Actions {
		a.Order = int32(n + 1)                  //set order before converting
		action, err := a.toAwsAction(ctx, rctx) //TODO: return value instead of ptr from toAwsAction
		if err != nil {
			return nil, errors.Wrap(err, "error parsing rule actions")
		}
		actions = append(actions, *action)
	}
	return actions, nil
}

// toAwsRuleConditions converts a []LBCondition to []*elbv2.RuleCondition
func (l LBRule) toAwsRuleConditions() ([]elbv2types.RuleCondition, error) {
	//conditions := make([]*elbv2.RuleCondition, len(l.Conditions))
	var conditions []elbv2types.RuleCondition
	for _, c := range l.Conditions {
		condition, err := c.toAwsRuleCondition() //TODO: return value instead of ptr from toAwsRuleCondition
		if err != nil {
			return nil, errors.Wrap(err, "error parsing rule condition")
		}
		conditions = append(conditions, *condition)
	}
	return conditions, nil
}

// targetGroupNametoArn returns the target group arn for name
func targetGroupNametoArn(ctx context.Context, rctx Context, name string) (string, error) {
	client := client.ELB(ctx, rctx.ProviderName)

	out, err := client.DescribeTargetGroups(ctx, &elbv2.DescribeTargetGroupsInput{
		Names: []string{name},
	})
	if err != nil {
		return "", errors.Wrap(err, "error looking up target group arn")
	}

	if len(out.TargetGroups) == 0 {
		return "", errors.New("no target group found for provided name")
	}

	return *out.TargetGroups[0].TargetGroupArn, nil
}
