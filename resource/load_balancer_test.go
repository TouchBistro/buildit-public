package resource

import (
	"context"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

func TestLoadBalancerAttributeUpserts(t *testing.T) {
	tests := []struct {
		name     string
		existing map[string]string
		desired  map[string]string
		expected map[string]string
	}{
		{
			name: "ignores existing-only attributes",
			existing: map[string]string{
				"bob":   "1",
				"vance": "x",
			},
			desired: map[string]string{
				"vance": "x",
			},
			expected: map[string]string{},
		},
		{
			name: "updates only changed desired attributes",
			existing: map[string]string{
				"bob":   "1",
				"vance": "old",
			},
			desired: map[string]string{
				"vance": "new",
			},
			expected: map[string]string{
				"vance": "new",
			},
		},
		{
			name: "adds desired attributes missing in existing",
			existing: map[string]string{
				"vance": "x",
			},
			desired: map[string]string{
				"vance": "x",
				"new":   "y",
			},
			expected: map[string]string{
				"new": "y",
			},
		},
		{
			name: "nil desired means no upserts",
			existing: map[string]string{
				"bob": "1",
			},
			desired:  nil,
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := loadBalancerAttributeUpserts(tt.existing, tt.desired)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Fatalf("unexpected upserts: expected=%v got=%v", tt.expected, got)
			}
		})
	}
}

func TestFormatListenerDiffMessages(t *testing.T) {
	tests := []struct {
		name        string
		listenerKey string
		messages    []string
		expected    []string
	}{
		{
			name:        "prefixes every listener message",
			listenerKey: "HTTPS:443",
			messages: []string{
				"updating tag audit: old -> new",
				"default actions are not the same",
			},
			expected: []string{
				"listener HTTPS:443: updating tag audit: old -> new",
				"listener HTTPS:443: default actions are not the same",
			},
		},
		{
			name:        "empty messages returns empty result",
			listenerKey: "HTTP:80",
			messages:    nil,
			expected:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatListenerDiffMessages(tt.listenerKey, tt.messages)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Fatalf("unexpected listener diff messages: expected=%v got=%v", tt.expected, got)
			}
		})
	}
}

func TestSortedListenerKeys(t *testing.T) {
	tests := []struct {
		name      string
		listeners map[string]int
		expected  []string
	}{
		{
			name: "sorts listener keys for deterministic output",
			listeners: map[string]int{
				"HTTPS:443": 1,
				"HTTP:80":   1,
				"TCP:8443":  1,
			},
			expected: []string{"HTTP:80", "HTTPS:443", "TCP:8443"},
		},
		{
			name:      "empty map has no keys",
			listeners: map[string]int{},
			expected:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sortedListenerKeys(tt.listeners)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Fatalf("unexpected sorted keys: expected=%v got=%v", tt.expected, got)
			}
		})
	}
}

var _ DetailedResourceDiff = &LoadBalancerDiff{}

const testLoadBalancerArn = "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/example-lb/0123456789abcdef"

func testExistingLoadBalancer() *elbv2types.LoadBalancer {
	return &elbv2types.LoadBalancer{
		LoadBalancerArn: aws.String(testLoadBalancerArn),
		Scheme:          elbv2types.LoadBalancerSchemeEnumInternetFacing,
		Type:            elbv2types.LoadBalancerTypeEnumApplication,
		AvailabilityZones: []elbv2types.AvailabilityZone{
			{SubnetId: aws.String("subnet-0123456789abcdef0")},
		},
		SecurityGroups: []string{"sg-0123456789abcdef0"},
	}
}

// mustLoadBalancerDiff fails the test when compareLoadBalancer found nothing.
func mustLoadBalancerDiff(t *testing.T, diffs *LoadBalancerDiff) *LoadBalancerDiff {
	t.Helper()
	if diffs == nil {
		t.Fatal("expected a diff")
	}
	return diffs
}

func TestCompareLoadBalancerChanges(t *testing.T) {
	lb := LoadBalancer{
		Name:               "example-lb",
		Scheme:             aws.String(LoadBalancerSchemeInternal),
		Type:               aws.String(LoadBalancerTypeApplication),
		SubnetNames:        []string{"example-subnet-a", "example-subnet-b"},
		SecurityGroupNames: []string{"example-sg"},
		Attributes: map[string]string{
			"idle_timeout.timeout_seconds": "120",
			"deletion_protection.enabled":  "true",
		},
		Tags: map[string]string{"env": "new-env", "team": "example-team"},
	}
	existing := testExistingLoadBalancer()
	existingAttrs := map[string]string{
		"idle_timeout.timeout_seconds": "60",
		"deletion_protection.enabled":  "true",
	}
	plan := listenerPlan{
		add: map[string]LBListener{
			"HTTPS:443": {
				Protocol: "HTTPS",
				Port:     443,
				DefaultRule: LBRule{Actions: []LBAction{
					{ActionType: "forward", FwTargetGroups: []LBTargetGroupRef{{Name: "example-tg"}}},
				}},
			},
		},
		del: map[string]elbv2types.Listener{
			"HTTP:80": {
				Protocol:       elbv2types.ProtocolEnumHttp,
				Port:           aws.Int32(80),
				DefaultActions: []elbv2types.Action{{Type: elbv2types.ActionTypeEnumRedirect}},
			},
		},
		upd: map[*LBListener]*LoadBalancerListenerDiff{
			{Protocol: "HTTPS", Port: 8443}: {},
		},
		updates: []listenerUpdate{
			{
				key: "HTTPS:8443", port: 8443,
				messages: []string{"ssl policy is different old-policy -> new-policy", "tag env is different: old-env -> new-env"},
				changes:  []Change{{Field: "sslPolicy", From: "old-policy", To: "new-policy"}, {Field: "tags.env", From: "old-env", To: "new-env"}},
			},
		},
	}
	awsTags := map[string]string{"env": "old-env"}

	diffs := mustLoadBalancerDiff(t, lb.compareLoadBalancer(context.Background(), existing,
		[]string{"subnet-0123456789abcdef0", "subnet-0123456789abcdef1"},
		[]string{"sg-0123456789abcdef0"},
		existingAttrs, plan, awsTags))

	wantMessages := []string{
		"load balancer schema is different",
		"1 attributes to be updated for load balancer example-lb - map[idle_timeout.timeout_seconds:120]",
		"listener HTTPS:8443: ssl policy is different old-policy -> new-policy",
		"listener HTTPS:8443: tag env is different: old-env -> new-env",
		"1 listener(s) to be added to load balancer example-lb: HTTPS:443",
		"1 listener(s) to be updated to load balancer example-lb: HTTPS:8443",
		"1 listener(s) to be removed from load balancer example-lb: HTTP:80",
		"tag env is different: old-env -> new-env",
		"tag team is added: example-team",
	}
	if got := diffs.Differences(); !reflect.DeepEqual(got, wantMessages) {
		t.Errorf("Differences() = %q, want %q", got, wantMessages)
	}

	wantChanges := []Change{
		{Field: "scheme", From: "internet-facing", To: "internal"},
		{Field: "subnetNames", From: "subnet-0123456789abcdef0", To: "subnet-0123456789abcdef0,subnet-0123456789abcdef1"},
		{Field: "attributes.idle_timeout.timeout_seconds", From: "60", To: "120"},
		{Field: "listeners[8443].sslPolicy", From: "old-policy", To: "new-policy"},
		{Field: "listeners[8443].tags.env", From: "old-env", To: "new-env"},
		{Field: "listeners[443]", To: "HTTPS forward example-tg"},
		{Field: "listeners[80]", From: "HTTP redirect"},
		{Field: "tags.env", From: "old-env", To: "new-env"},
		{Field: "tags.team", To: "example-team"},
	}
	if got := diffs.Changes(); !reflect.DeepEqual(got, wantChanges) {
		t.Errorf("Changes() = %+v, want %+v", got, wantChanges)
	}

	if !diffs.schemeDiff || !diffs.subnetsDiff || !diffs.attributesDiffs || !diffs.listenerDiffs || !diffs.tagsDiff {
		t.Errorf("apply flags not set: %+v", diffs)
	}
	if diffs.securityGroupsDiff || diffs.typeDiff {
		t.Errorf("unexpected apply flags set: %+v", diffs)
	}
	if diffs.loadbalancerArn != testLoadBalancerArn {
		t.Errorf("loadbalancerArn = %q, want %q", diffs.loadbalancerArn, testLoadBalancerArn)
	}
}

func TestCompareLoadBalancerTypeAndSecurityGroups(t *testing.T) {
	lb := LoadBalancer{
		Name:       "example-lb",
		Scheme:     aws.String(LoadBalancerSchemeInternetFacing),
		Type:       aws.String(LoadBalancerTypeNetwork),
		Attributes: map[string]string{},
		Tags:       map[string]string{},
	}
	existing := testExistingLoadBalancer()

	diffs := mustLoadBalancerDiff(t, lb.compareLoadBalancer(context.Background(), existing,
		[]string{"subnet-0123456789abcdef0"},
		[]string{"sg-0123456789abcdef1"},
		map[string]string{}, listenerPlan{}, map[string]string{}))

	wantMessages := []string{"load balancer type is different"}
	if got := diffs.Differences(); !reflect.DeepEqual(got, wantMessages) {
		t.Errorf("Differences() = %q, want %q", got, wantMessages)
	}

	wantChanges := []Change{
		{Field: "type", From: "application", To: "network"},
		{Field: "securityGroupNames", From: "sg-0123456789abcdef0", To: "sg-0123456789abcdef1"},
	}
	if got := diffs.Changes(); !reflect.DeepEqual(got, wantChanges) {
		t.Errorf("Changes() = %+v, want %+v", got, wantChanges)
	}
}

func TestCompareLoadBalancerIdentical(t *testing.T) {
	lb := LoadBalancer{
		Name:       "example-lb",
		Scheme:     aws.String(LoadBalancerSchemeInternetFacing),
		Type:       aws.String(LoadBalancerTypeApplication),
		Attributes: map[string]string{"idle_timeout.timeout_seconds": "60"},
		Tags:       map[string]string{"env": "example-env"},
	}
	existing := testExistingLoadBalancer()

	diffs := lb.compareLoadBalancer(context.Background(), existing,
		[]string{"subnet-0123456789abcdef0"},
		[]string{"sg-0123456789abcdef0"},
		map[string]string{"idle_timeout.timeout_seconds": "60", "access_logs.s3.enabled": "false"},
		listenerPlan{}, map[string]string{"env": "example-env"})
	if diffs != nil {
		t.Errorf("expected no diff, got messages %q changes %+v", diffs.Differences(), diffs.Changes())
	}
}

func TestDescribeListener(t *testing.T) {
	tests := []struct {
		name     string
		listener LBListener
		expected string
	}{
		{
			name:     "forward with target groups",
			listener: LBListener{Protocol: "https", DefaultRule: LBRule{Actions: []LBAction{{ActionType: "forward", FwTargetGroups: []LBTargetGroupRef{{Name: "example-tg"}, {Name: "example-tg-2"}}}}}},
			expected: "HTTPS forward example-tg,example-tg-2",
		},
		{
			name:     "redirect",
			listener: LBListener{Protocol: "HTTP", DefaultRule: LBRule{Actions: []LBAction{{ActionType: "redirect"}}}},
			expected: "HTTP redirect",
		},
		{
			name:     "no default action",
			listener: LBListener{Protocol: "TCP"},
			expected: "TCP",
		},
		{
			name:     "fixed response carries the status code",
			listener: LBListener{Protocol: "HTTPS", DefaultRule: LBRule{Actions: []LBAction{{ActionType: "fixed-response", FxStatusCode: "404", FxMessageBody: "Target not found"}}}},
			expected: "HTTPS fixed-response 404",
		},
		{
			name:     "redirect carries protocol, port and status",
			listener: LBListener{Protocol: "HTTP", DefaultRule: LBRule{Actions: []LBAction{{ActionType: "redirect", ReProtocol: "https", RePort: "443", ReStatusCode: "HTTP_301"}}}},
			expected: "HTTP redirect HTTPS:443 HTTP_301",
		},
		{
			name:     "redirect-https is rendered as written",
			listener: LBListener{Protocol: "HTTP", DefaultRule: LBRule{Actions: []LBAction{{ActionType: "redirect-https"}}}},
			expected: "HTTP redirect-https",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := describeListener(tt.listener); got != tt.expected {
				t.Errorf("describeListener() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDescribeRule(t *testing.T) {
	rule := LBRule{
		Conditions: []LBCondition{
			{ConditionType: HostHeaderCondition, Values: []string{"example.com", "www.example.com"}},
			{ConditionType: HttpHeaderCondition, HeaderName: "x-example", Values: []string{"a"}},
		},
		Actions: []LBAction{{ActionType: "forward", FwTargetGroups: []LBTargetGroupRef{{Name: "example-tg"}}}},
	}
	if got, want := describeRule(rule), "if host-header is example.com,www.example.com and http-header x-example is a then forward example-tg"; got != want {
		t.Errorf("describeRule() = %q, want %q", got, want)
	}

	awsRule := elbv2types.Rule{
		Conditions: []elbv2types.RuleCondition{
			{Field: aws.String(HostHeaderCondition), HostHeaderConfig: &elbv2types.HostHeaderConditionConfig{Values: []string{"example.com", "www.example.com"}}},
			{Field: aws.String(HttpHeaderCondition), HttpHeaderConfig: &elbv2types.HttpHeaderConditionConfig{HttpHeaderName: aws.String("x-example"), Values: []string{"a"}}},
			{Field: aws.String(PathPatternCondition), Values: []string{"/api/*"}},
		},
		Actions: []elbv2types.Action{{Type: elbv2types.ActionTypeEnumForward}},
	}
	// AWS rules name target groups by ARN only, so the action carries no names.
	if got, want := describeAWSRule(awsRule), "if host-header is example.com,www.example.com and http-header x-example is a and path-pattern is /api/* then forward"; got != want {
		t.Errorf("describeAWSRule() = %q, want %q", got, want)
	}

	if got, want := describeAWSListener(elbv2types.Listener{Protocol: elbv2types.ProtocolEnumHttp, DefaultActions: []elbv2types.Action{{Type: elbv2types.ActionTypeEnumRedirect}}}), "HTTP redirect"; got != want {
		t.Errorf("describeAWSListener() = %q, want %q", got, want)
	}

	awsActions := []elbv2types.Action{
		{Type: elbv2types.ActionTypeEnumRedirect, RedirectConfig: &elbv2types.RedirectActionConfig{Protocol: aws.String("HTTPS"), Port: aws.String("443"), StatusCode: elbv2types.RedirectActionStatusCodeEnumHttp301}},
		{Type: elbv2types.ActionTypeEnumFixedResponse, FixedResponseConfig: &elbv2types.FixedResponseActionConfig{StatusCode: aws.String("404")}},
	}
	if got, want := describeAWSActions(awsActions), "redirect HTTPS:443 HTTP_301 fixed-response 404"; got != want {
		t.Errorf("describeAWSActions() = %q, want %q", got, want)
	}
}
