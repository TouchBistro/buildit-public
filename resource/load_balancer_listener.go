package resource

import (
	"context"
	"fmt"
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

// LBListener represents a load balancer listener
type LBListener struct {
	Name                    string            `yaml:"name"`
	Certificates            []string          `yaml:"certificates"` // first one is considered default
	Protocol                string            `yaml:"protocol"`     // HTTP, HTTPS TCP, TLS
	SslPolicy               *string           `yaml:"sslPolicy"`    // uses default ELBSecurityPolicy-FS-1-2-2019-08 when HTTPS/TLS
	Port                    int32             `yaml:"port"`         // listener port
	DefaultRule             LBRule            `yaml:"ifNoRulesMatch"`
	Rules                   []LBRule          `yaml:"rules"`
	LoadBalancerName        string            `yaml:"-"`
	LoadBalancerARN         string            `yaml:"-"`
	LoadBalancerListenerArn string            `yaml:"-"`
	GlobalTags              map[string]string `yaml:"-"` // pre-stripped of buildit's reserved keys by the parent load balancer, so rules can inherit it as-is
}

// Identifier represents the unique id of a listener
func (l LBListener) Identifier() string {
	return l.Name
}

func (l *LBListener) Normalize(ctx context.Context, rctx Context) {

	// upper case the protocol; aws-sdk-v2 doesn't allow lower case protocol names..
	l.Protocol = strings.ToUpper(l.Protocol)

	if l.SslPolicy == nil {
		l.SslPolicy = aws.String("ELBSecurityPolicy-TLS13-1-1-2021-06") //TODO: this should be configurable...
	}

	for _, r := range l.Rules {
		r.Normalize(ctx, rctx)
	}
}

// Validate the listener
func (l LBListener) Validate(ctx context.Context) error {

	var errMessages []string

	proto := l.Protocol
	if proto != HTTP && proto != HTTPS && proto != TCP && proto != TLS && proto != UDP {
		errMessages = append(errMessages,
			fmt.Sprintf("invalid protocol `%s` specified for listener`%s`; only HTTP, HTTPS, TCP, TLS OR UDP expected", l.Protocol, l.Identifier()))
	}

	if (proto == HTTPS || proto == TLS) && len(l.Certificates) == 0 {
		errMessages = append(errMessages,
			"one or more certificates must be provided when protocol is HTTPS  or TLS")
	}

	if len(l.DefaultRule.Actions) == 0 {
		errMessages = append(errMessages,
			fmt.Sprintf("no default action specified for listener `%s`", l.Identifier()))
	}

	for _, rule := range l.Rules {
		err := rule.Validate(ctx)
		if err != nil {
			t, ok := err.(*ValidationError)
			if ok {
				errMessages = append(errMessages, t.Messages...)
			}
		}
	}

	if errMessages == nil {
		return nil
	}

	return &ValidationError{
		ResourceIdentifier: l.Identifier(),
		ResourceType:       "Load Balancer Listener",
		Messages:           errMessages,
	}
}

// Destroy delete the listener
func (l LBListener) Destroy(ctx context.Context, rctx Context) error {

	client := client.ELB(ctx, rctx.ProviderName)
	_, err := client.DeleteListener(ctx, &elbv2.DeleteListenerInput{
		ListenerArn: aws.String(l.LoadBalancerListenerArn),
	})

	if err != nil {
		return errors.Wrapf(err, "error deleting listener %v", l.Identifier())
	}

	log.WithFields(log.Fields{
		"Loadbalancer": l.LoadBalancerName,
		"Name":         l.Identifier(),
	}).Infof("%s", color.Red("loadbalancer listener deleted"))

	return nil
}

// LoadBalancerListenerDiff represetns diffs between lb listener definition & AWS representation
type LoadBalancerListenerDiff struct {
	BaseResourceDiff

	listenerArn        string
	defaultCertsDiff   bool
	defaultCerts       []elbv2types.Certificate
	certsDiff          bool
	certsToAdd         map[string]bool
	certsToDelete      map[string]bool
	policyDiff         bool
	defaultActionsDiff bool
	defaultActions     []elbv2types.Action
	rulesDiff          bool
	existingRules      []*elbv2types.Rule //existing rule n, nil implies no updated needed
	rulesToUpdate      []*LBRule          // new rule n, nil implies no updated needed
	rulesToAdd         []LBRule
	rulesToDelete      []LBRule
	tagsDiff           bool
	tagDiff            util.TagDiffResult
}

// equals fetches the existing load balancer listener, and if it exists
// checks if this resource is equal to the corresponding AWS LB listener
func (l LBListener) equals(ctx context.Context, rctx Context, existing *elbv2types.Listener) (ResourceDiff, error) {

	diffs := &LoadBalancerListenerDiff{
		BaseResourceDiff: BaseResourceDiff{
			Resource: existing,
		},
		listenerArn: *existing.ListenerArn,
	}

	var diff bool

	//TODO: use a custom set type for this
	oMap := make(map[string]bool)
	nMap := make(map[string]bool)

	//certificates (ALB & HTTPs only)
	if strings.ToUpper(l.Protocol) == HTTPS || strings.ToUpper(l.Protocol) == TLS {

		//default certificate
		certArn, err := certificateArnByCertificateIDOrDomain(ctx, rctx, l.Certificates[0])
		if err != nil {
			return nil, errors.Wrapf(err, "cannot lookup certificate for %v", l.Certificates[0])
		}
		if certArn != *existing.Certificates[0].CertificateArn {
			diffs.Messages = append(diffs.Messages, "default certificate does not match")
			diff = true
			diffs.defaultCertsDiff = true

			var certs []elbv2types.Certificate
			certs = append(certs, elbv2types.Certificate{
				CertificateArn: aws.String(certArn),
			})
			diffs.defaultCerts = certs
		}

		// extra certificates
		// fetch additional certificates
		respCerts, err := client.ELB(ctx, rctx.ProviderName).DescribeListenerCertificates(ctx, &elbv2.DescribeListenerCertificatesInput{
			ListenerArn: existing.ListenerArn,
			PageSize:    aws.Int32(26),
		})
		if err != nil {
			return nil, errors.Wrapf(err, "error listing certificates for listener %v", l.Name)
		}
		newCerts := l.Certificates[1:]
		oldCerts := respCerts.Certificates

		for _, cert := range oldCerts {
			if !*cert.IsDefault {
				oMap[*cert.CertificateArn] = true
			}
		}
		for _, cert := range newCerts {
			arn, err := certificateArnByCertificateIDOrDomain(ctx, rctx, cert)
			if err != nil {
				return nil, errors.Wrapf(err, "cannot lookup certificate for %v", cert)
			}
			nMap[arn] = true
		}
		for arn := range nMap {
			if _, ok := oMap[arn]; ok {
				delete(nMap, arn)
				delete(oMap, arn)
			}
		}

		diffs.certsDiff = len(oMap) > 0 || len(nMap) > 0
		if diffs.certsDiff {
			diff = true
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("%v certificate(s) need to be added", len(nMap)))
			diffs.certsToAdd = nMap
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("%v certificate(s) need to be removed", len(oMap)))
			diffs.certsToDelete = oMap
		}

		// ssl policy
		diffs.policyDiff = util.Coalesce(existing.SslPolicy, "") != util.Coalesce(l.SslPolicy, "")
		if diffs.policyDiff {
			diff = true
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("ssl policy is different %v -> %v", util.Coalesce(existing.SslPolicy, ""), util.Coalesce(l.SslPolicy, "")))
		}

	}

	// default action
	newDefaultAction := l.DefaultRule.Actions
	oldDefaultAction := existing.DefaultActions
	defaultDiff := len(newDefaultAction) != len(oldDefaultAction)
	if !defaultDiff {
		for i := 0; i < len(newDefaultAction) && !defaultDiff; i++ {
			newDefaultAction[i].Order = int32(i + 1)
			if !newDefaultAction[i].equals(ctx, rctx, &oldDefaultAction[i]) {
				defaultDiff = true
			}
		}
	}
	if defaultDiff {
		diff = true
		diffs.defaultActionsDiff = true
		diffs.Messages = append(diffs.Messages, "default actions are not the same")
		actions, err := toAwsElbv2Actions(ctx, rctx, l.DefaultRule.Actions)
		if err != nil {
			return nil, errors.Wrap(err, "error parsing default rule action")
		}
		diffs.defaultActions = actions
	}

	// rules
	// for listener rules we will compare new & existing rules index by index.
	// if the set of old and existing is not same, that rule will be modified.
	// if there are more new rules, then all the extra ones will be added
	// if there are few new rules, then all the extra existing ones will be deleted

	// find existing rules..
	// `DescribeRules()` API will return all rules + default. So we need to ignore
	// the default and use the reamining for comparisons
	respRules, err := client.ELB(ctx, rctx.ProviderName).DescribeRules(ctx, &elbv2.DescribeRulesInput{
		ListenerArn: existing.ListenerArn,
		PageSize:    aws.Int32(100),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error listing rules for listener %v", l.Name)
	}

	existingRules := respRules.Rules[0 : len(respRules.Rules)-1]

	limit := len(l.Rules)
	if limit > len(existingRules) {
		limit = len(existingRules)
	}
	//for the first n rules where n = min(len(old),len(existin)) just
	//compare and update rules as needed
	priorityStr := "1"
	for i := 0; i < limit; i++ {
		existing := existingRules[i]
		priorityStr = *existing.Priority
		if !l.Rules[i].equals(ctx, rctx, &existing) {
			diff = true
			diffs.rulesDiff = true
			diffs.Messages = append(diffs.Messages,
				fmt.Sprintf("load balancer rule with priority %v for listener %v is not the same", priorityStr, l.Identifier()))
			l.Rules[i].ListenerName = l.Name
			l.Rules[i].LoadBalancerName = l.LoadBalancerName
			diffs.rulesToUpdate = append(diffs.rulesToUpdate, &l.Rules[i]) //add the LBRule to update
			diffs.existingRules = append(diffs.existingRules, &existing)   //existing AWS rule for comparison
		} else {
			diffs.rulesToUpdate = append(diffs.rulesToUpdate, nil)
			diffs.existingRules = append(diffs.existingRules, nil)
		}
	}
	// rule priority is always in ascending order, however there may be gaps e.g 2,4,8,10,20....
	// this happens when rules are added/removed from the AWS console. For this reason, during
	// update(), when adding new rules, we have to start with last updated rule's priority+1 and
	// increment thereon
	priority, err := strconv.Atoi(priorityStr)
	if err != nil {
		return nil, errors.Wrapf(err, "error converting rule priority to integer: %v", priorityStr)
	}

	//new rules are more then old, add the rest from new
	if len(l.Rules) > limit {
		diff = true
		diffs.rulesDiff = true
		for i := limit; i < len(l.Rules); i++ {
			priority++
			l.Rules[i].ListenerArn = *existing.ListenerArn
			l.Rules[i].ListenerName = l.Name
			l.Rules[i].LoadBalancerName = l.LoadBalancerName
			l.Rules[i].Priority = int32(priority)
			l.Rules[i].GlobalTags = l.GlobalTags
			diffs.rulesToAdd = append(diffs.rulesToAdd, l.Rules[i])
		}
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("%v rules to be added to listener %v", len(diffs.rulesToAdd), l.Name))
	} else if len(existingRules) > limit { //delete the rest from existing
		diff = true
		diffs.rulesDiff = true
		for i := limit; i < len(existingRules); i++ {
			priority, err := strconv.Atoi(*existingRules[i].Priority)
			if err != nil {
				return nil, errors.Wrap(err, "non numeric priority value received")
			}
			temp := LBRule{
				RuleArn:          *existingRules[i].RuleArn,
				Priority:         int32(priority),
				ListenerName:     l.Name,
				LoadBalancerName: l.LoadBalancerName,
			}
			diffs.rulesToDelete = append(diffs.rulesToDelete, temp)
		}
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("%v rules to be deleted from listener %v", len(diffs.rulesToDelete), l.Name))
	}

	// tags
	awsTags, err := awsw.NewELB(ctx, rctx.ProviderName).GetResourceTags(ctx, *existing.ListenerArn)
	if err != nil {
		return nil, err
	}
	if tagDiff := TagDiffForContext(ctx, awsTags, l.GlobalTags); tagDiff.HasChanges() {
		diff = true
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, TagDiffSummary(awsTags, diffs.tagDiff)...)
	}

	if !diff {
		return nil, nil
	}

	return diffs, nil
}

// apply makes a load balancer listener
func (l LBListener) apply(ctx context.Context, rctx Context) error {

	elbClient := client.ELB(ctx, rctx.ProviderName)
	actions, err := toAwsElbv2Actions(ctx, rctx, l.DefaultRule.Actions)
	if err != nil {
		return errors.Wrap(err, "error parsing default rule action")
	}

	//Protocol
	switch strings.ToUpper(l.Protocol) {
	case HTTP, HTTPS, TCP, TLS:
	default:
		return errors.Errorf(
			"invalid protocol specified for load balancer %v", l.Protocol)
	}

	//SSL certs for https listeners
	var certs []elbv2types.Certificate //only one can be specified here (default)
	var sslPolicy *string = nil
	if strings.ToUpper(l.Protocol) == HTTPS || strings.ToUpper(l.Protocol) == TLS {
		certArn, err := certificateArnByCertificateIDOrDomain(ctx, rctx, l.Certificates[0])
		if err != nil {
			return errors.Wrap(err, "error creating listener due to missing ssl certificate")
		}
		certs = append(certs, elbv2types.Certificate{
			CertificateArn: aws.String(certArn),
		})
		sslPolicy = aws.String(*l.SslPolicy)
	}

	//tags
	tags := make([]elbv2types.Tag, 0)
	for k, v := range l.GlobalTags {
		tag := elbv2types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		}
		tags = append(tags, tag)
	}

	out, err := elbClient.CreateListener(ctx, &elbv2.CreateListenerInput{
		DefaultActions:  actions,
		Certificates:    certs,
		LoadBalancerArn: aws.String(l.LoadBalancerARN),
		Port:            aws.Int32(l.Port),
		Protocol:        elbv2types.ProtocolEnum(l.Protocol),
		SslPolicy:       sslPolicy,
		Tags:            tags,
	})

	if err != nil {
		return errors.Wrap(err, "error creating load balancer listener")
	}

	listenerArn := out.Listeners[0].ListenerArn
	log.WithFields(log.Fields{
		"Name":         l.Name,
		"Loadbalancer": l.LoadBalancerName,
	}).Info(color.Green("load balancer listener created"))

	//attach additional certificates...
	err = addAdditionalCertsToListener(ctx, rctx, listenerArn, l.Certificates)
	if err != nil {
		return errors.Wrap(err, "error adding additional certificates to load balancer listener")
	}

	// Add listener rules
	for n, rule := range l.Rules {
		rule.ListenerArn = *listenerArn
		rule.ListenerName = l.Name
		rule.LoadBalancerName = l.LoadBalancerName
		rule.Priority = int32(n + 1)
		rule.GlobalTags = l.GlobalTags
		err = rule.Apply(ctx, rctx)
		if err != nil {
			return errors.Wrap(err, "error creating load balancer rules")
		}
	}
	return nil
}

// update existing listener
func (l LBListener) applyDiffs(ctx context.Context, rctx Context, diffs ResourceDiff) error {

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": l.Identifier(),
		}).Info("no updates required for load balancer listener")
		return nil
	}

	listDiffs, ok := diffs.(*LoadBalancerListenerDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	input := &elbv2.ModifyListenerInput{
		ListenerArn: aws.String(listDiffs.listenerArn),
	}

	if listDiffs.certsDiff {
		input.Certificates = listDiffs.defaultCerts
	}

	if listDiffs.defaultActionsDiff {
		input.DefaultActions = listDiffs.defaultActions
	}

	// SSL policy to update
	if listDiffs.policyDiff {
		input.SslPolicy = aws.String(*l.SslPolicy)
	}

	// rules to update
	if listDiffs.rulesDiff {
		for n, urule := range listDiffs.rulesToUpdate {
			// only update rules that are non-nil (updated)
			if urule != nil {
				err := urule.update(ctx, rctx, listDiffs.existingRules[n])
				if err != nil {
					return errors.Wrapf(err, "error updating load balancer rule")
				}
			}
		}
		// rules to add
		for _, arule := range listDiffs.rulesToAdd {
			err := arule.Apply(ctx, rctx)
			if err != nil {
				return errors.Wrap(err, "error creating load balancer rules")
			}
		}
		// rules to delete
		for _, drule := range listDiffs.rulesToDelete {
			err := drule.Destroy(ctx, rctx)
			if err != nil {
				return errors.Wrap(err, "error creating load balancer rules")
			}
		}
	}

	// certificates to add ...
	if listDiffs.certsDiff {
		if len(listDiffs.certsToAdd) > 0 {
			log.WithFields(log.Fields{
				"Name":         l.Name,
				"Loadbalancer": l.LoadBalancerName,
			}).Infof("%v certificates added to listener", len(listDiffs.certsToAdd))

			for arn := range listDiffs.certsToAdd {
				//one certificate at a time can be added...
				_, err := client.ELB(ctx, rctx.ProviderName).AddListenerCertificates(ctx, &elbv2.AddListenerCertificatesInput{
					ListenerArn: aws.String(listDiffs.listenerArn),
					Certificates: []elbv2types.Certificate{{
						CertificateArn: aws.String(arn),
					}},
				})
				if err != nil {
					return errors.Wrap(err, "error adding additional certificates to load balancer listener")
				}
			}
		}

		if len(listDiffs.certsToDelete) > 0 {
			log.WithFields(log.Fields{
				"Name":         l.Name,
				"Loadbalancer": l.LoadBalancerName,
			}).Infof("%v certificates removed from listener", len(listDiffs.certsToDelete))

			for arn := range listDiffs.certsToDelete {
				_, err := client.ELB(ctx, rctx.ProviderName).RemoveListenerCertificates(ctx, &elbv2.RemoveListenerCertificatesInput{
					ListenerArn: aws.String(listDiffs.listenerArn),
					Certificates: []elbv2types.Certificate{{
						CertificateArn: aws.String(arn),
					}},
				})
				if err != nil {
					return errors.Wrap(err, "error deleting  certificates from load balancer listener")
				}
			}
		}
	}

	elbClient := client.ELB(ctx, rctx.ProviderName)

	_, err := elbClient.ModifyListener(ctx, input)
	if err != nil {
		return errors.Wrap(err, "error updating load balancer listener")
	}

	// tags
	if listDiffs.tagsDiff {
		upserts := listDiffs.tagDiff.Upserts()
		if len(upserts) > 0 {
			if err := awsw.NewELB(ctx, rctx.ProviderName).AddResourceTags(ctx, listDiffs.listenerArn, upserts); err != nil {
				return errors.Wrapf(err, "error updating load balancer listener tags for %v", l.Identifier())
			}
		}
		if len(listDiffs.tagDiff.Deleted) > 0 {
			if err := awsw.NewELB(ctx, rctx.ProviderName).DeleteResourceTags(ctx, listDiffs.listenerArn, listDiffs.tagDiff.Deleted); err != nil {
				return errors.Wrapf(err, "error deleting load balancer listener tags for %v", l.Identifier())
			}
		}
	}

	log.WithFields(log.Fields{
		"Name":         l.Name,
		"Loadbalancer": l.LoadBalancerName,
	}).Info(color.Yellow("load Balancer listener updated"))

	return nil
}

// toAwsElbv2Actions converts an []LBAction to []elbv2types.Action
func toAwsElbv2Actions(ctx context.Context, rctx Context, lbActions []LBAction) ([]elbv2types.Action, error) {
	var actions []elbv2types.Action
	for n, a := range lbActions {
		a.Order = int32(n + 1) //set order before converting
		action, err := a.toAwsAction(ctx, rctx)
		if err != nil {
			return nil, errors.Wrap(err, "error parsing default rule action")
		}
		actions = append(actions, *action)
	}
	return actions, nil
}

// addCertsToListener adds additional listeners [1:] from the domains array
func addAdditionalCertsToListener(ctx context.Context, rctx Context, listenerArn *string, domains []string) error {

	if len(domains) <= 1 {
		return nil
	}

	for i := 1; i < len(domains); i++ { //skip the first one (that's added as default cert...)
		certArn, err := certificateArnByCertificateIDOrDomain(ctx, rctx, domains[i])
		if err != nil {
			return errors.Wrap(err, "error creating listener due to missing ssl certificate")
		}
		_, err = client.ELB(ctx, rctx.ProviderName).AddListenerCertificates(ctx, &elbv2.AddListenerCertificatesInput{
			ListenerArn: listenerArn,
			Certificates: []elbv2types.Certificate{{
				CertificateArn: aws.String(certArn),
			}},
		})
		if err != nil {
			return errors.Wrap(err, "error adding additional certificates to load balancer listener")
		}

	}

	return nil
}

// certificateArnByCertificateIDOrDomain resolves an ACM certificate identifier (full ARN,
// certificate id, or domain name) against the resource provider's account and region —
// listener certificates must live in the load balancer's own region.
func certificateArnByCertificateIDOrDomain(ctx context.Context, rctx Context, certDomainOrID string) (string, error) {
	arn, err := awsw.NewACM(ctx, rctx.ProviderName).CertificateArnForIdentifier(ctx, certDomainOrID)
	if err != nil {
		return "", errors.Wrapf(err, "error resolving certificate %q", certDomainOrID)
	}
	return aws.ToString(arn), nil
}
