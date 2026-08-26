package resource

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/pkg/errors"
)

const (
	//HTTP Protocol
	HTTP string = "HTTP"
	//HTTPS Protocol
	HTTPS = "HTTPS"
	//TCP Protocol
	TCP = "TCP"
	//TLS Protocol
	TLS = "TLS"
	//UDP Protocol
	UDP = "UDP"
	//TCPOrUDP Prtocol
	TCPOrUDP = "TCP_UDP"
)

const (
	//Default availability zone for target registration
	availabilityZoneDefault = "all"
)

const (
	//Instance target type
	Instance string = "instance"
	//IP Address target type
	IP = "ip"
	//Lambda Address target type
	Lambda = "lambda"
)

const (
	//LeastOutstandingRequests algorithm for load balancing targets
	LeastOutstandingRequests = "least_outstanding_requests"
	//RoundRobin algorithm for load balancing targes
	RoundRobin = "round_robin"
)

const (
	//StickinessTypeLBCookie method for stickiness, used for ALBs
	StickinessTypeLBCookie = "lb_cookie"
	//StickinessTypeSourceIP method for stickiness, used for NLBs
	StickinessTypeSourceIP = "source_ip"
)

const (
	staticTargetFormatString = "%v:%v@%v"
)

// TrafficPort represents upstream healthcheck port
// for service container, when using dynamic host-port binding
const TrafficPort string = "traffic-port"

// LBHealthCheck represents an LB Healtcheck setup
type LBHealthCheck struct {
	IntervalSeconds  int32  `yaml:"interval"`
	Path             string `yaml:"path"`
	Port             string `yaml:"port"`
	Protocol         string `yaml:"protocol"`
	TimeoutSeconds   int32  `yaml:"timeout"`
	HealthyCount     int32  `yaml:"healthyCount"`
	UnhealthyCount   int32  `yaml:"unhealthyCount"`
	SuccessHTTPCodes string `yaml:"successHttpCodes"` //200,201,302 or 200-209,302
}

// GetDefaultHTTP inits a default HTTP LBHealthcheck
func GetDefaultHTTP(path string) LBHealthCheck {
	return LBHealthCheck{
		30, path, TrafficPort, HTTP,
		5, //Timeout seconds
		5, //Healthy count
		2, //Unhealthy count
		"200",
	}
}

// LBTargetGroup is a target group resource
type LBTargetGroup struct {
	BaseResource `yaml:",inline"`
	Name                string            `yaml:"name"`
	VPCName             string            `yaml:"vpc"`
	Protocol            string            `yaml:"protocol"`
	Port                int32             `yaml:"port"`
	TargetType          string            `yaml:"targetType"`
	Healthcheck         *LBHealthCheck    `yaml:"healthcheck"`
	Sticky              bool              `yaml:"sticky"`              // stickiness.enabled
	Algorithm           string            `yaml:"algorithm"`           //load_balancing.algorithm.type
	DeregistrationDelay *int64            `yaml:"deregistrationDelay"` //deregistration_delay.timeout_seconds
	Targets             []string          `yaml:"targets"`             //targets specified in format ip:port@region
	DependsOn           []Key             `yaml:"dependsOn"`
	GlobalTags          map[string]string `yaml:"-"`
	Tags                map[string]string `yaml:"tags"`
}

// Key returns the unique key for the resource for this buildit context
func (tg LBTargetGroup) Key() Key {
	return NewKey(tg.Context.ProviderName, tg.Identifier())
}

// Identifier returns the unique identifier for this resources
func (t LBTargetGroup) Identifier() string {
	return t.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (t *LBTargetGroup) Normalize(ctx context.Context) {

	if t.Algorithm == "" {
		t.Algorithm = LeastOutstandingRequests
	}

	t.Protocol = strings.ToUpper(t.Protocol)

	if t.DeregistrationDelay == nil {
		t.DeregistrationDelay = aws.Int64(300)
	}

	if t.Healthcheck == nil {
		// TODO can we just inline GetDefaultHTTP?
		hc := GetDefaultHTTP("/ping")
		t.Healthcheck = &hc
	} else {
		t.Healthcheck.Protocol = strings.ToUpper(t.Healthcheck.Protocol)
	}

	if len(t.Targets) > 0 {
		t.Targets = normalizeTargets(t.Targets)
	}

	//Merge globalTags to Target Group tags, if key is not already present
	//later we'll use s.Tags to add/update tags

	if t.Tags == nil {
		t.Tags = make(map[string]string)
	}
	ResourceTags(t.Tags).Merge(t.GlobalTags)
}

// Validate checks that the TargetGroup has a valid configuration.
// It returns a list of validation error strings.
func (t LBTargetGroup) Validate(ctx context.Context) error {
	var errMessages []string

	if len(t.Name) > 32 {
		msg := fmt.Sprintf("name cannot be longer than 32 characters, current length: %d", len(t.Name))
		errMessages = append(errMessages, msg)
	}

	// Healthcheck can be nil, use default in that case
	healthcheck := t.Healthcheck
	if healthcheck == nil {
		hc := GetDefaultHTTP("/ping")
		healthcheck = &hc
	}

	switch strings.ToUpper(healthcheck.Protocol) {
	case HTTP, HTTPS, TLS, TCP, UDP, TCPOrUDP:
	default:
		msg := fmt.Sprintf("invalid protocol specified for healthcheck %q", healthcheck.Protocol)
		errMessages = append(errMessages, msg)
	}

	switch strings.ToUpper(t.Protocol) {
	case HTTP, HTTPS, TLS, TCP, UDP, TCPOrUDP:
	default:
		msg := fmt.Sprintf("invalid protocol specified %q", t.Protocol)
		errMessages = append(errMessages, msg)
	}

	switch strings.ToLower(t.TargetType) {
	case Instance, IP, Lambda:
	default:
		msg := fmt.Sprintf("invalid targetType specified %q", t.TargetType)
		errMessages = append(errMessages, msg)
	}

	if strings.ToLower(t.TargetType) == Lambda {
		errMessages = append(errMessages, "lambda target type not supported yet")
	}

	//when targetGroup.protocol in (HTTP/HTTPS) targetGroup.healthcheck.protocol should be in (HTTP/HTTPS)
	if (strings.ToUpper(t.Protocol) == HTTP || strings.ToUpper(t.Protocol) == HTTPS) &&
		(strings.ToUpper(healthcheck.Protocol) != HTTP && strings.ToUpper(healthcheck.Protocol) != HTTPS) {
		errMessages = append(errMessages,
			"when target group protocol is HTTP or HTTPS, the healthcheck protocol must also be HTTP or HTTPS")

	}

	//when healthcheck protocol in TCP,UDP or TCP_UDP, healthcheck path must not be provided
	if len(healthcheck.Path) > 0 &&
		(strings.ToUpper(healthcheck.Protocol) == TCP ||
			strings.ToUpper(healthcheck.Protocol) == UDP ||
			strings.ToUpper(healthcheck.Protocol) == TCPOrUDP) {
		errMessages = append(errMessages,
			"healthcheck path cannot be specified when healthcheck protocol is TCP, UDP or TCP_UDP")
	}

	//when healthcheck protocol in TCP,UDP or TCP_UDP, healthcheck success codes must not be provided
	if len(healthcheck.SuccessHTTPCodes) > 0 &&
		(strings.ToUpper(healthcheck.Protocol) == TCP ||
			strings.ToUpper(healthcheck.Protocol) == UDP ||
			strings.ToUpper(healthcheck.Protocol) == TCPOrUDP) {
		errMessages = append(errMessages,
			"healthcheck http codes cannot be specified when healthcheck protocol is TCP, UDP or TCP_UDP")
	}

	//when target group protocol in TCP,TLS, healthcheck protocol in HTTP, timeout interval must be 6s
	if (strings.ToUpper(t.Protocol) == TCP || strings.ToUpper(t.Protocol) == TLS) &&
		strings.ToUpper(healthcheck.Protocol) == HTTP && healthcheck.TimeoutSeconds != 6 {
		errMessages = append(errMessages,
			"healthcheck timeout value must be 6 when target group protocol is TCP or TLS and healhtcheck protocol is HTTP")
	}

	//when target group protocol in TCP,TLS, healthcheck protocol in TCP,HTTPS, timeout interval must be 10s
	if (strings.ToUpper(t.Protocol) == TCP || strings.ToUpper(t.Protocol) == TLS) &&
		(strings.ToUpper(healthcheck.Protocol) == TCP || strings.ToUpper(healthcheck.Protocol) == HTTPS) &&
		healthcheck.TimeoutSeconds != 10 {
		errMessages = append(errMessages,
			"healthcheck timeout value must be 10 when target group protocol is TCP or TLS and healhtcheck protocol is TCP or HTTPS")
	}

	//algorithm
	if t.Algorithm != LeastOutstandingRequests && t.Algorithm != RoundRobin {
		errMessages = append(errMessages,
			fmt.Sprintf("invalid algorithm type specified, only '%v' or '%v' allowed", LeastOutstandingRequests, RoundRobin))
	}

	//de-registration delay
	if *t.DeregistrationDelay < 0 || *t.DeregistrationDelay > 3600 {
		errMessages = append(errMessages, "value for deregistration delay must be beween 0-3600 seconds")
	}

	//target registration
	if t.TargetType != IP && len(t.Targets) > 0 {
		errMessages = append(errMessages, "target registration only supported for target group with TargetType = IP")
	}

	if len(t.Targets) > 0 {
		for _, t := range t.Targets {
			if _, _, _, err := parseIPPort(t); err != nil {
				errMessages = append(errMessages, fmt.Sprintf("target is invalid %v", t))
			}
		}
	}

	// No validation errors, yay!
	if errMessages == nil {
		return nil
	}

	return &ValidationError{
		ResourceIdentifier: t.Identifier(),
		ResourceType:       "Target Group",
		Messages:           errMessages,
	}
}

// Apply creates/updates the load balancer target group
func (t LBTargetGroup) Apply(ctx context.Context) error {

	log.Debugf("creating target group %v", t.Identifier())

	diffs, err := t.Compare(ctx)
	if err != nil {
		return err
	}

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": t.Identifier(),
		}).Info("no updates required")
		return nil
	}

	//if diff found & existing resource exists...
	if diffs.AWSResource() != nil {
		log.WithField("Name", t.Identifier()).Info("target group already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}
		err = t.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update target group %v", t.Identifier())
		}
		return nil
	}

	return t.apply(ctx)
}

// Destroy deletes the load balancer target group from AWS.
func (t LBTargetGroup) Destroy(ctx context.Context) error {

	log.Debugf("destroying target group %v", t.Identifier())

	awsTG, err := t.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error finding target group %v", t.Name)
	}

	if awsTG == nil {
		log.WithFields(log.Fields{
			"Name": t.Identifier(),
		}).Info("target group does not exist, nothing to destroy, skippping ")
		return nil
	}

	elbClient := client.ELB(ctx, t.Context.ProviderName)
	_, err = elbClient.DeleteTargetGroup(ctx, &elbv2.DeleteTargetGroupInput{
		TargetGroupArn: awsTG.TargetGroupArn,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to delete target group %s", t.Name)
	}

	log.WithFields(log.Fields{
		"Name": t.Name,
	}).Info(color.Red("target group destroyed"))
	return nil
}

// TargetGroupDiff type  captures diffs between this & AWS representation
type TargetGroupDiff struct {
	BaseResourceDiff

	targetGroupArn string

	vpcDiff         bool
	protocolDiff    bool
	portDiff        bool
	targetTypeDiff  bool
	healthcheckDiff bool
	attrsDiff       bool
	targetsDiff     bool
	targets         []string
	tagsDiff        bool
	tagDiff         util.TagDiffResult
}

// Compare fetches the existing TargetGroup & if it exists, checks if this
// is equal to the AWS resource
func (t LBTargetGroup) Compare(ctx context.Context) (ResourceDiff, error) {

	existing, err := t.fetchExisting(ctx)
	if err != nil {
		return nil, err
	}

	var diff bool
	diffs := &TargetGroupDiff{}

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "target group does not exist")
		return diffs, nil
	}

	diffs.Resource = existing
	diffs.targetGroupArn = *existing.TargetGroupArn

	// vpc
	vpcID, err := awsw.NewEC2(ctx, t.Context.ProviderName).VpcIdByName(ctx, t.VPCName)
	if err != nil {
		return nil, errors.Wrap(err, "error getting vpc id")
	}

	if *vpcID != *existing.VpcId {
		diff = true
		diffs.vpcDiff = true
		diffs.Messages = append(diffs.Messages, "vpc is not the same")
	}

	// protocol
	if t.Protocol != string(existing.Protocol) {
		diff = true
		diffs.protocolDiff = true
		diffs.Messages = append(diffs.Messages, "protocol is not the same")
	}

	// port
	if t.Port != *existing.Port {
		diff = true
		diffs.portDiff = true
		diffs.Messages = append(diffs.Messages, "port is not the same")
	}

	// target type
	if t.TargetType != string(existing.TargetType) {
		diff = true
		diffs.targetTypeDiff = true
		diffs.Messages = append(diffs.Messages, "target type is not the same")
	}

	// healtcheck
	if t.Healthcheck.IntervalSeconds != *existing.HealthCheckIntervalSeconds ||
		!util.StringPtrEquals(existing.HealthCheckPath, &t.Healthcheck.Path) ||
		t.Healthcheck.Port != *existing.HealthCheckPort ||
		t.Healthcheck.Protocol != string(existing.HealthCheckProtocol) ||
		t.Healthcheck.TimeoutSeconds != *existing.HealthCheckTimeoutSeconds ||
		t.Healthcheck.HealthyCount != *existing.HealthyThresholdCount ||
		t.Healthcheck.UnhealthyCount != *existing.UnhealthyThresholdCount ||
		(existing.Matcher != nil && existing.Matcher.HttpCode != nil && t.Healthcheck.SuccessHTTPCodes != *existing.Matcher.HttpCode) {
		diff = true
		diffs.healthcheckDiff = true
		diffs.Messages = append(diffs.Messages, "healthcheck config is different")
	}

	// attributes
	awsAttrs, err := awsw.NewELB(ctx, t.Context.ProviderName).DescribeTargetGroupAttributesByArn(ctx, *existing.TargetGroupArn)
	if err != nil {
		return nil, err
	}

	for _, attr := range awsAttrs {
		// stickiness
		if *attr.Key == "stickiness.enabled" {
			stickinessEnabled, err := strconv.ParseBool(*attr.Value)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse value of stickiness.enabled attribute for target group %s", t.Name)
			}

			if t.Sticky != stickinessEnabled {
				diff = true
				diffs.attrsDiff = true
				diffs.Messages = append(diffs.Messages, "attributes `stickiness.enabled` value is not the same")
			}
		}
		// deregistration delay
		if *attr.Key == "deregistration_delay.timeout_seconds" {
			strDeRegisgrationDelay := strconv.FormatInt(*t.DeregistrationDelay, 10)
			if *attr.Value != strDeRegisgrationDelay {
				diff = true
				diffs.attrsDiff = true
				diffs.Messages = append(diffs.Messages, "attributes `deregistration_delay.timeout_seconds` value is not the same")
			}
		}
		// algorithm
		if *attr.Key == "load_balancing.algorithm.type" {
			if *attr.Value != t.Algorithm {
				diff = true
				diffs.attrsDiff = true
				diffs.Messages = append(diffs.Messages, "attributes `load_balancing.algorithm.type` value is not the same")
			}
		}
	}

	// tags
	awsTags, err := awsw.NewELB(ctx, t.Context.ProviderName).GetResourceTags(ctx, *existing.TargetGroupArn)
	if err != nil {
		return nil, err
	}
	if tagDiff := TagDiffForContext(ctx, awsTags, t.Tags); tagDiff.HasChanges() {
		diff = true
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, TagDiffSummary(awsTags, diffs.tagDiff)...)

	}

	// target
	if len(t.Targets) > 0 {

		existingTargets, err := targetsForTargetGroup(ctx, t.Context, *existing.TargetGroupArn)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to fetch existing targets for the target group")
		}
		existingTargets = normalizeTargets(existingTargets)

		if util.DiffStringSlices(t.Targets, existingTargets) {
			diff = true
			diffs.targetsDiff = true
			diffs.targets = existingTargets
			diffs.Messages = append(diffs.Messages, "static targets are not the same")
		}
	}

	if !diff {
		return nil, nil
	}

	return diffs, nil
}

// fetchExisting fetches the existing target group
func (t LBTargetGroup) fetchExisting(ctx context.Context) (*elbv2types.TargetGroup, error) {

	elbClient := client.ELB(ctx, t.Context.ProviderName)
	respDescribeTGs, err := elbClient.DescribeTargetGroups(ctx, &elbv2.DescribeTargetGroupsInput{
		Names: []string{t.Name},
	})

	if err != nil {
		var tgnfe *elbv2types.TargetGroupNotFoundException
		if errors.As(err, &tgnfe) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "error looking up target group: %v", t.Identifier())
	}

	if len(respDescribeTGs.TargetGroups) > 1 {
		return nil, errors.Errorf("multiple target groups named %s found", t.Name)
	}

	return &respDescribeTGs.TargetGroups[0], nil
}

// apply creates/updates the load balancer target group
func (t LBTargetGroup) apply(ctx context.Context) error {

	elbClient := client.ELB(ctx, t.Context.ProviderName)

	vpcid, err := awsw.NewEC2(ctx, t.Context.ProviderName).VpcIdByName(ctx, t.VPCName)
	if err != nil {
		return errors.Wrap(err, "error getting VPC Id")
	}

	var healthcheckPath *string
	if len(t.Healthcheck.Path) > 0 {
		healthcheckPath = aws.String(t.Healthcheck.Path)
	}

	t.Protocol = strings.ToUpper(t.Protocol)
	t.TargetType = strings.ToLower(t.TargetType)
	t.Healthcheck.Protocol = strings.ToUpper(t.Healthcheck.Protocol)

	var matcher *elbv2types.Matcher
	if len(t.Healthcheck.SuccessHTTPCodes) > 0 {
		matcher = &elbv2types.Matcher{
			HttpCode: &t.Healthcheck.SuccessHTTPCodes,
		}
	}

	var tags []elbv2types.Tag
	for k, v := range t.Tags {
		tag := elbv2types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		}
		tags = append(tags, tag)
	}

	input := &elbv2.CreateTargetGroupInput{
		Name:                       aws.String(t.Name),
		Port:                       aws.Int32(t.Port),
		Protocol:                   elbv2types.ProtocolEnum(t.Protocol),
		TargetType:                 elbv2types.TargetTypeEnum(t.TargetType),
		VpcId:                      vpcid,
		HealthCheckEnabled:         aws.Bool(true),
		Tags:                       tags,
		HealthCheckIntervalSeconds: aws.Int32(t.Healthcheck.IntervalSeconds),
		HealthCheckPath:            healthcheckPath,
		HealthCheckPort:            aws.String(t.Healthcheck.Port),
		HealthCheckProtocol:        elbv2types.ProtocolEnum(t.Healthcheck.Protocol),
		HealthCheckTimeoutSeconds:  aws.Int32(t.Healthcheck.TimeoutSeconds),
		HealthyThresholdCount:      aws.Int32(t.Healthcheck.HealthyCount),
		UnhealthyThresholdCount:    aws.Int32(t.Healthcheck.UnhealthyCount),
		Matcher:                    matcher,
	}

	createOutput, err := elbClient.CreateTargetGroup(ctx, input)
	if err != nil {
		return errors.Wrap(err, "failed creating target group")
	}

	targetGroupArn := createOutput.TargetGroups[0].TargetGroupArn
	log.WithFields(log.Fields{
		"Name": t.Name,
		"ARN":  *targetGroupArn,
	}).Info(color.Green("target group created"))

	// set attributes

	stickinessEnabled := strconv.FormatBool(t.Sticky)
	tgAttrArray := []elbv2types.TargetGroupAttribute{
		tgAttr("deregistration_delay.timeout_seconds", strconv.FormatInt(*t.DeregistrationDelay, 10)), //default 300
		tgAttr("stickiness.enabled", stickinessEnabled),                                               //default=false
	}

	//  when target type is IP (only applicable to ALBs)
	if (t.TargetType == IP || t.TargetType == Instance) && (t.Protocol == HTTP || t.Protocol == HTTPS) {
		tgAttrArray = append(tgAttrArray,
			tgAttr("load_balancing.algorithm.type", t.Algorithm), //default=least_outstanding_requests
			tgAttr("slow_start.duration_seconds", "0"),           //default=0, disabled (not configurable in buildit)
		)
	}

	// set stickiness attributes
	if t.Sticky {
		//when tg protocol is HTTP/S (only applicable to ALBs)
		if t.Protocol == HTTP || t.Protocol == HTTPS {
			//set stickiness type = lb_cookie
			tgAttrArray = append(tgAttrArray, tgAttr("stickiness.type", StickinessTypeLBCookie))
			//also when targetType = IP, add cookies duration
			if t.TargetType == IP {
				//TODO: make this configurable
				tgAttrArray = append(tgAttrArray,
					tgAttr("stickiness.lb_cookie.duration_seconds", "86400"), //default:86400 (not configurable in buildit)
				)
			}
		}

		// when tg protocol is TCP/UDP or both, (only applicable to NLBs)
		if t.Protocol == TCP || t.Protocol == UDP || t.Protocol == TCPOrUDP {
			//set stickiness type = client_ip
			tgAttrArray = append(tgAttrArray,
				tgAttr("stickiness.type", StickinessTypeSourceIP),
			)
		}
	}

	// update target group attributes
	_, err = elbClient.ModifyTargetGroupAttributes(ctx, &elbv2.ModifyTargetGroupAttributesInput{
		TargetGroupArn: targetGroupArn,
		Attributes:     tgAttrArray,
	})

	if err != nil {
		errDestroy := t.Destroy(ctx)
		if errDestroy != nil {
			return errors.Wrap(errDestroy, "failed updating target group attributes but target group was not deleted")
		}
		return errors.Wrap(err, "failed updating target group attributes, target group also deleted")
	}

	log.WithFields(log.Fields{
		"Name": t.Identifier(),
	}).Info("attributes updated for target group")

	// registering static targets
	// this feature is only supported for IP target groups. The list of provided targets in the
	// format ip:port will be registered, but buildit doesn't check health status after registration
	if t.TargetType == IP && len(t.Targets) > 0 {
		err = t.registerTargets(ctx, *targetGroupArn, t.Targets)
		if err != nil {
			return errors.Wrapf(err, "error registering IP targets to target group %v", t.Identifier())
		}

		log.WithFields(log.Fields{
			"Name": t.Identifier(),
		}).Infof("%v static targets registerd for target group", len(t.Targets))
	}

	return nil
}

// update the target group
func (t LBTargetGroup) applyDiffs(ctx context.Context, diffs ResourceDiff) error {

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": t.Identifier(),
		}).Info("no updates required for target group")
		return nil
	}

	tgDiffs, ok := diffs.(*TargetGroupDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	updateInput := &elbv2.ModifyTargetGroupInput{
		TargetGroupArn: aws.String(tgDiffs.targetGroupArn),
	}

	// following updates are invalid, return error
	if tgDiffs.vpcDiff {
		return errors.New("cannot modify target group VPC")
	}

	if tgDiffs.protocolDiff {
		return errors.New("cannot modify target group protocol")
	}

	if tgDiffs.portDiff {
		return errors.New("cannot modify target group port")
	}

	if tgDiffs.targetTypeDiff {
		return errors.New("Cannot modify target group target type")
	}

	elbClient := client.ELB(ctx, t.Context.ProviderName)

	// healthcheck
	if tgDiffs.healthcheckDiff {
		updateInput.HealthCheckIntervalSeconds = aws.Int32(t.Healthcheck.IntervalSeconds)
		updateInput.HealthCheckPath = aws.String(t.Healthcheck.Path)
		updateInput.HealthCheckPort = aws.String(t.Healthcheck.Port)
		updateInput.HealthCheckProtocol = elbv2types.ProtocolEnum(t.Healthcheck.Protocol)
		updateInput.HealthCheckTimeoutSeconds = aws.Int32(t.Healthcheck.TimeoutSeconds)
		updateInput.HealthyThresholdCount = aws.Int32(t.Healthcheck.HealthyCount)
		updateInput.UnhealthyThresholdCount = aws.Int32(t.Healthcheck.UnhealthyCount)
		updateInput.Matcher = &elbv2types.Matcher{HttpCode: &t.Healthcheck.SuccessHTTPCodes}

		_, err := elbClient.ModifyTargetGroup(ctx, updateInput)
		if err != nil {
			return errors.Wrapf(err, "failed to update target group %s", t.Name)
		}
	}

	// attributes
	if tgDiffs.attrsDiff {
		updateAttrs := t.toTgAttrArray()
		_, err := elbClient.ModifyTargetGroupAttributes(ctx, &elbv2.ModifyTargetGroupAttributesInput{
			TargetGroupArn: aws.String(tgDiffs.targetGroupArn),
			Attributes:     updateAttrs,
		})
		if err != nil {
			return errors.Wrap(err, "failed updating target group attributes")
		}
	}

	// tags
	if tgDiffs.tagsDiff {
		upserts := tgDiffs.tagDiff.Upserts()
		if len(upserts) > 0 {
			if err := awsw.NewELB(ctx, t.Context.ProviderName).AddResourceTags(ctx, tgDiffs.targetGroupArn, upserts); err != nil {
				return errors.Wrapf(err, "error updating target group tags for %v", t.Identifier())
			}
		}

		if len(tgDiffs.tagDiff.Deleted) > 0 {
			if err := awsw.NewELB(ctx, t.Context.ProviderName).DeleteResourceTags(ctx, tgDiffs.targetGroupArn, tgDiffs.tagDiff.Deleted); err != nil {
				return errors.Wrapf(err, "error deleting target group tags for %v", t.Identifier())
			}
		}
	}

	// targets

	if tgDiffs.targetsDiff {

		targets := tgDiffs.targets
		newMap := make(map[string]bool)
		delMap := make(map[string]bool)

		//start will intent to register all targets
		for _, newTarget := range t.Targets {
			newMap[newTarget] = true
		}
		for _, oldTarget := range targets {
			if ok := newMap[oldTarget]; !ok {
				delMap[oldTarget] = true //must be de-registered
			} else {
				delete(newMap, oldTarget) //already registered
			}
		}

		if len(newMap) > 0 {
			regTargets := make([]string, 0)
			for k := range newMap {
				log.Debugf("will register target %v", k)
				regTargets = append(regTargets, k)
			}
			err := t.registerTargets(ctx, tgDiffs.targetGroupArn, regTargets)
			if err != nil {
				return errors.Wrapf(err, "error registering targets to target group %v", t.Identifier())
			}
			log.WithFields(log.Fields{
				"Name": t.Identifier(),
			}).Infof("%v targets registered for target group", len(newMap))
		}

		if len(delMap) > 0 {
			deregTargets := make([]string, 0)
			for k := range delMap {
				log.Debugf("will de-register target %v", k)
				deregTargets = append(deregTargets, k)
			}
			err := t.deRegisterTargets(ctx, tgDiffs.targetGroupArn, deregTargets)
			if err != nil {
				return errors.Wrapf(err, "error de-registering targets to target group %v", t.Identifier())
			}
			log.WithFields(log.Fields{
				"Name": t.Identifier(),
			}).Infof("%v targets de-registered from target group", len(delMap))
		}
	}

	log.WithFields(log.Fields{
		"Name": t.Name,
		"ARN":  tgDiffs.targetGroupArn,
	}).Info(color.Yellow("target group updated"))

	return nil
}

// register targets to the target group specified
func (t LBTargetGroup) registerTargets(ctx context.Context, targetGroupArn string, targets []string) error {

	targetDesc, err := strArrToTargetDescription(targets)
	if err != nil {
		return errors.Wrap(err, "error building a target description list")
	}
	elbClient := client.ELB(ctx, t.Context.ProviderName)
	_, err = elbClient.RegisterTargets(ctx, &elbv2.RegisterTargetsInput{
		TargetGroupArn: aws.String(targetGroupArn),
		Targets:        targetDesc,
	})
	if err != nil {
		return errors.Wrapf(err, "error registering targets for target group %v", t.Identifier())
	}
	return nil
}

// deregister targets from the target group specified
func (t LBTargetGroup) deRegisterTargets(ctx context.Context, targetGroupArn string, targets []string) error {

	targetDesc, err := strArrToTargetDescription(targets)
	if err != nil {
		return errors.Wrap(err, "error building a target description list")
	}
	elbClient := client.ELB(ctx, t.Context.ProviderName)
	_, err = elbClient.DeregisterTargets(ctx, &elbv2.DeregisterTargetsInput{
		TargetGroupArn: aws.String(targetGroupArn),
		Targets:        targetDesc,
	})
	if err != nil {
		return errors.Wrapf(err, "error registering targets for target group %v", t.Identifier())
	}
	return nil
}

// toTgAttrArray converts the TB Attribute files to []elbv2/types/TargetGroupAttribute
func (t LBTargetGroup) toTgAttrArray() []elbv2types.TargetGroupAttribute {

	stickinessEnabled := strconv.FormatBool(t.Sticky)
	tgAttrArray := []elbv2types.TargetGroupAttribute{
		tgAttr("deregistration_delay.timeout_seconds", strconv.FormatInt(*t.DeregistrationDelay, 10)), //default 300
		tgAttr("stickiness.enabled", stickinessEnabled),                                               //default=false
	}

	//  when target type is IP (only applicable to ALBs)
	if (t.TargetType == IP || t.TargetType == Instance) && (t.Protocol == HTTP || t.Protocol == HTTPS) {
		tgAttrArray = append(tgAttrArray,
			tgAttr("load_balancing.algorithm.type", t.Algorithm), //default=least_outstanding_requests
			tgAttr("slow_start.duration_seconds", "0"),           //default=0, disabled (not configurable in buildit)
		)
	}

	// set stickiness attributes
	if t.Sticky {
		//when tg protocol is HTTP/S (only applicable to ALBs)
		if t.Protocol == HTTP || t.Protocol == HTTPS {
			//set stickiness type = lb_cookie
			tgAttrArray = append(tgAttrArray, tgAttr("stickiness.type", StickinessTypeLBCookie))
			//also when targetType = IP, add cookies duration
			if t.TargetType == IP {
				//TODO: make this configurable
				tgAttrArray = append(tgAttrArray,
					tgAttr("stickiness.lb_cookie.duration_seconds", "86400"), //default:86400 (not configurable in buildit)
				)
			}
		}

		// when tg protocol is TCP/UDP or both, (only applicable to NLBs)
		if t.Protocol == TCP || t.Protocol == UDP || t.Protocol == TCPOrUDP {
			//set stickiness type = client_ip
			tgAttrArray = append(tgAttrArray,
				tgAttr("stickiness.type", StickinessTypeSourceIP),
			)
		}
	}

	return tgAttrArray
}

// strArrToTargetDescription converts a []string representation of an IP target to []elbv2types.TargetDescription
func strArrToTargetDescription(targets []string) ([]elbv2types.TargetDescription, error) {

	var targetDesc []elbv2types.TargetDescription
	for _, s := range targets {
		ip, port, region, err := parseIPPort(s)
		if err != nil {
			return nil, errors.Wrap(err, "error parsing target ip and port")
		}
		targetDesc = append(targetDesc, elbv2types.TargetDescription{
			AvailabilityZone: aws.String(region),
			Id:               aws.String(ip),
			Port:             aws.Int32(port),
		})
	}
	return targetDesc, nil
}

// tgAttr returns a elbv2/types/TargetGroupAttribute from the supplied key/value
func tgAttr(key string, value string) elbv2types.TargetGroupAttribute {
	return elbv2types.TargetGroupAttribute{
		Key: aws.String(key), Value: aws.String(value)}
}

// normalizeTargets convets a []string of target group static target srings to
// the normalized form using `parseIPPort`
func normalizeTargets(targets []string) []string {

	var normalizedTargets []string
	for _, t := range targets {
		if ip, port, region, err := parseIPPort(t); err == nil {
			normalizedTargets = append(normalizedTargets, fmt.Sprintf(staticTargetFormatString, ip, port, region))
		} else {
			normalizedTargets = append(normalizedTargets, t)
		}
	}
	return normalizedTargets
}

// parses the provided ip:port@region formatted string as ip, port, region or error
// ip must be supplied, port and region are optional; so valid formatting is:
//   - ip
//   - ip:port
//   - ip@region
//   - ip:port@region
func parseIPPort(ipPort string) (string, int32, string, error) {
	//check if availability zone is specified here..
	region := availabilityZoneDefault
	if strings.Contains(ipPort, "@") {
		idx := strings.Index(ipPort, "@") //index of @, region separator
		region = ipPort[idx+1:]
		ipPort = ipPort[:idx] //strip away the region
	}

	//here ipPort must be of the form <ip>:<port> or just <ip>, where port defaults to :80
	idx := strings.Index(ipPort, ":")
	if idx == -1 {
		return ipPort, 80, region, nil
	}
	ip := ipPort[:idx]
	port, err := strconv.Atoi(ipPort[idx+1:])
	if err != nil {
		return "", 0, "", errors.Wrapf(err, "error parsing port from %v", ipPort)
	}
	return ip, int32(port), region, nil

}

// targetsForTargetGroup returns all registered targets for the target group; only supports
// IP targets targets are returned, irrespective of the current health status (draining, initial etc)
//
// as ip:port@ strings
func targetsForTargetGroup(ctx context.Context, rctx Context, targetGroupArn string) ([]string, error) {

	elbClient := client.ELB(ctx, rctx.ProviderName)

	targets, err := elbClient.DescribeTargetHealth(ctx, &elbv2.DescribeTargetHealthInput{
		TargetGroupArn: &targetGroupArn,
	})

	if err != nil {
		return nil, errors.Wrapf(err, "error getting target group health %v", targetGroupArn)
	}

	registeredTargets := make([]string, 0)
	for _, target := range targets.TargetHealthDescriptions {
		registeredTargets = append(registeredTargets,
			fmt.Sprintf(staticTargetFormatString, *target.Target.Id, *target.Target.Port, *target.Target.AvailabilityZone))
	}

	return registeredTargets, nil
}
