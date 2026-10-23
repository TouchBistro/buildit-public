package resource

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testTGVpcID = "vpc-0123456789abcdef0"
	testTGArn   = "arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/example-tg/0123456789abcdef"
)

// testTargetGroup returns a normalized definition with static targets.
func testTargetGroup() LBTargetGroup {
	tg := LBTargetGroup{
		Name:       "example-tg",
		VPCName:    "example-vpc",
		Protocol:   HTTP,
		Port:       8080,
		TargetType: IP,
		Healthcheck: &LBHealthCheck{
			IntervalSeconds:  30,
			Path:             "/health",
			Port:             TrafficPort,
			Protocol:         HTTP,
			TimeoutSeconds:   5,
			HealthyCount:     5,
			UnhealthyCount:   2,
			SuccessHTTPCodes: "200",
		},
		Sticky:              false,
		Algorithm:           RoundRobin,
		DeregistrationDelay: aws.Int64(60),
		Targets:             []string{"10.0.0.1:8080@all", "10.0.0.2:8080@all"},
		Tags:                map[string]string{"env": "test", "team": "example-team"},
	}
	tg.Normalize(context.Background())
	return tg
}

// testExistingTargetGroup returns the AWS representation that matches testTargetGroup.
func testExistingTargetGroup() *elbv2types.TargetGroup {
	return &elbv2types.TargetGroup{
		TargetGroupArn:             aws.String(testTGArn),
		TargetGroupName:            aws.String("example-tg"),
		VpcId:                      aws.String(testTGVpcID),
		Protocol:                   elbv2types.ProtocolEnumHttp,
		Port:                       aws.Int32(8080),
		TargetType:                 elbv2types.TargetTypeEnumIp,
		HealthCheckIntervalSeconds: aws.Int32(30),
		HealthCheckPath:            aws.String("/health"),
		HealthCheckPort:            aws.String(TrafficPort),
		HealthCheckProtocol:        elbv2types.ProtocolEnumHttp,
		HealthCheckTimeoutSeconds:  aws.Int32(5),
		HealthyThresholdCount:      aws.Int32(5),
		UnhealthyThresholdCount:    aws.Int32(2),
		Matcher:                    &elbv2types.Matcher{HttpCode: aws.String("200")},
	}
}

// testExistingTGAttrs returns the AWS attributes that match testTargetGroup.
func testExistingTGAttrs() []elbv2types.TargetGroupAttribute {
	return []elbv2types.TargetGroupAttribute{
		{Key: aws.String("stickiness.enabled"), Value: aws.String("false")},
		{Key: aws.String("deregistration_delay.timeout_seconds"), Value: aws.String("60")},
		{Key: aws.String("load_balancing.algorithm.type"), Value: aws.String(RoundRobin)},
	}
}

func testExistingTGTags() map[string]string {
	return map[string]string{"env": "test", "team": "example-team"}
}

func testExistingTGTargets() []string {
	return []string{"10.0.0.1:8080@all", "10.0.0.2:8080@all"}
}

func TestLBTargetGroup_CompareExisting_NoDiff(t *testing.T) {
	tg := testTargetGroup()

	diff, err := tg.compareExisting(context.Background(), testExistingTargetGroup(), testTGVpcID,
		testExistingTGAttrs(), testExistingTGTags(), testExistingTGTargets())

	require.NoError(t, err)
	assert.Nil(t, diff)
}

func TestLBTargetGroup_CompareExisting_Changes(t *testing.T) {
	tg := testTargetGroup()
	tg.Port = 9090
	tg.Healthcheck.Path = "/ping"
	tg.DeregistrationDelay = aws.Int64(300)
	tg.Targets = []string{"10.0.0.1:8080@all", "10.0.0.3:8080@all"}
	tg.Tags["env"] = "new-env"

	diff, err := tg.compareExisting(context.Background(), testExistingTargetGroup(), testTGVpcID,
		testExistingTGAttrs(), testExistingTGTags(), testExistingTGTargets())

	require.NoError(t, err)
	require.NotNil(t, diff)

	assert.Equal(t, []string{
		"port is not the same",
		"healthcheck config is different",
		"attributes `deregistration_delay.timeout_seconds` value is not the same",
		"tag env is different: test -> new-env",
		"static targets are not the same",
	}, diff.Differences())

	assert.Equal(t, []Change{
		{Field: "port", From: "8080", To: "9090"},
		{Field: "healthcheck.path", From: "/health", To: "/ping"},
		{Field: "deregistrationDelay", From: "60", To: "300"},
		{Field: "tags.env", From: "test", To: "new-env"},
		{Field: "targets", From: "10.0.0.1:8080@all,10.0.0.2:8080@all", To: "10.0.0.1:8080@all,10.0.0.3:8080@all"},
	}, diff.Changes())

	// apply instructions are untouched by the structured changes
	assert.True(t, diff.portDiff)
	assert.True(t, diff.healthcheckDiff)
	assert.True(t, diff.attrsDiff)
	assert.True(t, diff.tagsDiff)
	assert.True(t, diff.targetsDiff)
	assert.Equal(t, testExistingTGTargets(), diff.targets)
	assert.Equal(t, testTGArn, diff.targetGroupArn)
	assert.False(t, diff.vpcDiff)
	assert.False(t, diff.protocolDiff)
	assert.False(t, diff.targetTypeDiff)
}

func TestLBTargetGroup_CompareExisting_ScalarAndHealthcheckFields(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*LBTargetGroup)
		existing func(*elbv2types.TargetGroup)
		messages []string
		changes  []Change
	}{
		{
			name:     "protocol",
			mutate:   func(tg *LBTargetGroup) { tg.Protocol = HTTPS },
			messages: []string{"protocol is not the same"},
			changes:  []Change{{Field: "protocol", From: "HTTP", To: "HTTPS"}},
		},
		{
			name:     "vpc",
			existing: func(e *elbv2types.TargetGroup) { e.VpcId = aws.String("vpc-0fedcba9876543210") },
			messages: []string{"vpc is not the same"},
			changes:  []Change{{Field: "vpc", From: "vpc-0fedcba9876543210", To: testTGVpcID}},
		},
		{
			name:     "target type",
			mutate:   func(tg *LBTargetGroup) { tg.TargetType = Instance },
			messages: []string{"target type is not the same"},
			changes:  []Change{{Field: "targetType", From: "ip", To: "instance"}},
		},
		{
			name: "several healthcheck fields yield one message and one change each",
			mutate: func(tg *LBTargetGroup) {
				tg.Healthcheck.IntervalSeconds = 10
				tg.Healthcheck.Protocol = HTTPS
				tg.Healthcheck.SuccessHTTPCodes = "200-299"
			},
			messages: []string{"healthcheck config is different"},
			changes: []Change{
				{Field: "healthcheck.interval", From: "30", To: "10"},
				{Field: "healthcheck.protocol", From: "HTTP", To: "HTTPS"},
				{Field: "healthcheck.successHttpCodes", From: "200", To: "200-299"},
			},
		},
		{
			name: "healthcheck matcher absent on AWS is not a difference",
			existing: func(e *elbv2types.TargetGroup) {
				e.Matcher = nil
				e.HealthCheckTimeoutSeconds = aws.Int32(10)
			},
			messages: []string{"healthcheck config is different"},
			changes:  []Change{{Field: "healthcheck.timeout", From: "10", To: "5"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tg := testTargetGroup()
			if tt.mutate != nil {
				tt.mutate(&tg)
			}
			existing := testExistingTargetGroup()
			if tt.existing != nil {
				tt.existing(existing)
			}

			diff, err := tg.compareExisting(context.Background(), existing, testTGVpcID,
				testExistingTGAttrs(), testExistingTGTags(), testExistingTGTargets())

			require.NoError(t, err)
			require.NotNil(t, diff)
			assert.Equal(t, tt.messages, diff.Differences())
			assert.Equal(t, tt.changes, diff.Changes())
		})
	}
}

func TestLBTargetGroup_CompareExisting_Attributes(t *testing.T) {
	tg := testTargetGroup()
	tg.Sticky = true
	tg.Algorithm = LeastOutstandingRequests

	diff, err := tg.compareExisting(context.Background(), testExistingTargetGroup(), testTGVpcID,
		testExistingTGAttrs(), testExistingTGTags(), testExistingTGTargets())

	require.NoError(t, err)
	require.NotNil(t, diff)
	assert.Equal(t, []string{
		"attributes `stickiness.enabled` value is not the same",
		"attributes `load_balancing.algorithm.type` value is not the same",
	}, diff.Differences())
	assert.Equal(t, []Change{
		{Field: "sticky", From: "false", To: "true"},
		{Field: "algorithm", From: RoundRobin, To: LeastOutstandingRequests},
	}, diff.Changes())
	assert.True(t, diff.attrsDiff)
}

func TestLBTargetGroup_CompareExisting_StickinessParseError(t *testing.T) {
	tg := testTargetGroup()
	attrs := []elbv2types.TargetGroupAttribute{
		{Key: aws.String("stickiness.enabled"), Value: aws.String("not-a-bool")},
	}

	diff, err := tg.compareExisting(context.Background(), testExistingTargetGroup(), testTGVpcID,
		attrs, testExistingTGTags(), testExistingTGTargets())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse value of stickiness.enabled attribute for target group example-tg")
	assert.Nil(t, diff)
}

func TestLBTargetGroup_CompareExisting_Tags(t *testing.T) {
	tg := testTargetGroup()
	tg.Tags = map[string]string{"env": "test", "owner": "example-owner"} // team removed, owner added

	diff, err := tg.compareExisting(context.Background(), testExistingTargetGroup(), testTGVpcID,
		testExistingTGAttrs(), testExistingTGTags(), testExistingTGTargets())

	require.NoError(t, err)
	require.NotNil(t, diff)
	assert.Equal(t, TagDiffSummary(testExistingTGTags(), diff.tagDiff), diff.Differences())
	assert.Equal(t, []Change{
		{Field: "tags.team", From: "example-team", To: ""},
		{Field: "tags.owner", From: "", To: "example-owner"},
	}, diff.Changes())
	assert.True(t, diff.tagsDiff)
}

func TestLBTargetGroup_CompareExisting_NoTargetsDeclared(t *testing.T) {
	tg := testTargetGroup()
	tg.Targets = nil

	// Compare only fetches registered targets when the definition declares some,
	// so a registered-but-undeclared target is not a difference.
	diff, err := tg.compareExisting(context.Background(), testExistingTargetGroup(), testTGVpcID,
		testExistingTGAttrs(), testExistingTGTags(), nil)

	require.NoError(t, err)
	assert.Nil(t, diff)
}
