package resource

import (
	"encoding/json"
	"testing"
)

func mustParsePolicyDocument(t *testing.T, doc string) *IAMPolicyDocument {
	t.Helper()
	var d IAMPolicyDocument
	if err := json.Unmarshal([]byte(doc), &d); err != nil {
		t.Fatalf("failed to parse policy document: %v", err)
	}
	return &d
}

func TestIAMPolicyDocumentEquals(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
		want  EqualsResult
	}{
		{
			name: "identical documents",
			left: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":"s3:GetObject","Resource":"arn:aws:s3:::bucket/*"}]}`,
			right: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":"s3:GetObject","Resource":"arn:aws:s3:::bucket/*"}]}`,
			want: Equal,
		},
		{
			name: "reordered resource ARNs",
			left: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":"s3:GetObject",
				 "Resource":["arn:aws:s3:::a/*","arn:aws:s3:::b/*"]}]}`,
			right: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":"s3:GetObject",
				 "Resource":["arn:aws:s3:::b/*","arn:aws:s3:::a/*"]}]}`,
			want: Equal,
		},
		{
			name: "reordered actions",
			left: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":["s3:GetObject","s3:PutObject"],"Resource":"*"}]}`,
			right: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":["s3:PutObject","s3:GetObject"],"Resource":"*"}]}`,
			want: Equal,
		},
		{
			name: "string vs single-element array",
			left: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":"s3:GetObject","Resource":"arn:aws:s3:::bucket/*"}]}`,
			right: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::bucket/*"]}]}`,
			want: Equal,
		},
		{
			name: "principal type key casing is insignificant",
			left: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Principal":{"service":"s3.amazonaws.com"},"Action":"lambda:InvokeFunction","Resource":"*"}]}`,
			right: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Principal":{"Service":"s3.amazonaws.com"},"Action":"lambda:InvokeFunction","Resource":"*"}]}`,
			want: Equal,
		},
		{
			name: "different principal type keys are not equal",
			left: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Principal":{"Service":"s3.amazonaws.com"},"Action":"lambda:InvokeFunction","Resource":"*"}]}`,
			right: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Principal":{"Federated":"s3.amazonaws.com"},"Action":"lambda:InvokeFunction","Resource":"*"}]}`,
			want: NotEqual,
		},
		{
			name: "principal AWS string vs single-element array",
			left: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::123456789012:root"},
				 "Action":"sts:AssumeRole"}]}`,
			right: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Principal":{"AWS":["arn:aws:iam::123456789012:root"]},
				 "Action":"sts:AssumeRole"}]}`,
			want: Equal,
		},
		{
			name: "principal AWS reordered list",
			left: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow",
				 "Principal":{"AWS":["arn:aws:iam::123456789012:root","999999999999"]},
				 "Action":"sts:AssumeRole"}]}`,
			right: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow",
				 "Principal":{"AWS":["999999999999","arn:aws:iam::123456789012:root"]},
				 "Action":"sts:AssumeRole"}]}`,
			want: Equal,
		},
		{
			name: "not principal union forms",
			left: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Deny","NotPrincipal":{"AWS":"arn:aws:iam::123456789012:root"},
				 "NotAction":"s3:GetObject","NotResource":"arn:aws:s3:::bucket/*"}]}`,
			right: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Deny","NotPrincipal":{"AWS":["arn:aws:iam::123456789012:root"]},
				 "NotAction":["s3:GetObject"],"NotResource":["arn:aws:s3:::bucket/*"]}]}`,
			want: Equal,
		},
		{
			name: "wildcard principal string differs from AWS wildcard map",
			left: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Principal":"*","Action":"sts:AssumeRole"}]}`,
			right: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"sts:AssumeRole"}]}`,
			want: NotEqual,
		},
		{
			name: "condition values reordered and string vs array",
			left: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":"s3:GetObject","Resource":"*",
				 "Condition":{
					"StringEquals":{"aws:PrincipalOrgID":"o-1234567890"},
					"IpAddress":{"aws:SourceIp":["10.0.0.0/8","192.168.0.0/16"]}}}]}`,
			right: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":"s3:GetObject","Resource":"*",
				 "Condition":{
					"StringEquals":{"aws:PrincipalOrgID":["o-1234567890"]},
					"IpAddress":{"aws:SourceIp":["192.168.0.0/16","10.0.0.0/8"]}}}]}`,
			want: Equal,
		},
		{
			name: "condition bool value vs string form",
			left: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Deny","Action":"*","Resource":"*",
				 "Condition":{"Bool":{"aws:SecureTransport":false}}}]}`,
			right: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Deny","Action":"*","Resource":"*",
				 "Condition":{"Bool":{"aws:SecureTransport":"false"}}}]}`,
			want: Equal,
		},
		{
			name: "reordered statements with sids",
			left: `{"Version":"2012-10-17","Statement":[
				{"Sid":"First","Effect":"Allow","Action":"s3:GetObject","Resource":"*"},
				{"Sid":"Second","Effect":"Deny","Action":"s3:DeleteObject","Resource":"*"}]}`,
			right: `{"Version":"2012-10-17","Statement":[
				{"Sid":"Second","Effect":"Deny","Action":"s3:DeleteObject","Resource":"*"},
				{"Sid":"First","Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`,
			want: Equal,
		},
		{
			name: "reordered statements without sids",
			left: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"},
				{"Effect":"Deny","Action":"s3:DeleteObject","Resource":"*"}]}`,
			right: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Deny","Action":"s3:DeleteObject","Resource":"*"},
				{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`,
			want: Equal,
		},
		{
			name: "single statement object vs array of one",
			left: `{"Version":"2012-10-17","Statement":
				{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}}`,
			right: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`,
			want: Equal,
		},
		{
			name: "different resource ARN",
			left: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":"s3:GetObject","Resource":"arn:aws:s3:::a/*"}]}`,
			right: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":"s3:GetObject","Resource":"arn:aws:s3:::b/*"}]}`,
			want: NotEqual,
		},
		{
			name: "different effect",
			left: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`,
			right: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Deny","Action":"s3:GetObject","Resource":"*"}]}`,
			want: NotEqual,
		},
		{
			name: "different sid",
			left: `{"Version":"2012-10-17","Statement":[
				{"Sid":"AllowRead","Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`,
			right: `{"Version":"2012-10-17","Statement":[
				{"Sid":"AllowReads","Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`,
			want: NotEqual,
		},
		{
			name: "missing statement",
			left: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"},
				{"Effect":"Allow","Action":"s3:PutObject","Resource":"*"}]}`,
			right: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`,
			want: NotEqual,
		},
		{
			name: "extra action",
			left: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":["s3:GetObject"],"Resource":"*"}]}`,
			right: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":["s3:GetObject","s3:PutObject"],"Resource":"*"}]}`,
			want: NotEqual,
		},
		{
			name: "different condition key",
			left: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":"s3:GetObject","Resource":"*",
				 "Condition":{"StringEquals":{"aws:PrincipalOrgID":"o-1234567890"}}}]}`,
			right: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":"s3:GetObject","Resource":"*",
				 "Condition":{"StringEquals":{"aws:PrincipalAccount":"o-1234567890"}}}]}`,
			want: NotEqual,
		},
		{
			name: "different version",
			left: `{"Version":"2012-10-17","Statement":[
				{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`,
			right: `{"Version":"2008-10-17","Statement":[
				{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`,
			want: NotEqual,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := mustParsePolicyDocument(t, tt.left)
			right := mustParsePolicyDocument(t, tt.right)
			if got := left.equals(right); got != tt.want {
				t.Errorf("equals() = %v, want %v\nleft: %s\nright: %s", got, tt.want, tt.left, tt.right)
			}
			// Comparison must be symmetric
			if got := right.equals(left); got != tt.want {
				t.Errorf("reversed equals() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIAMPolicyDocumentEqualsNil(t *testing.T) {
	doc := mustParsePolicyDocument(t, `{"Version":"2012-10-17","Statement":[
		{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`)

	tests := []struct {
		name  string
		left  *IAMPolicyDocument
		right *IAMPolicyDocument
		want  EqualsResult
	}{
		{name: "both nil", left: nil, right: nil, want: Equal},
		{name: "left nil", left: nil, right: doc, want: LeftZero},
		{name: "right nil", left: doc, right: nil, want: RightZero},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.left.equals(tt.right); got != tt.want {
				t.Errorf("equals() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIAMPolicyDocumentEqualsYAMLDefined ensures a document defined in buildit
// config (Go native types, e.g. []string actions) compares equal to the same
// document decoded from the JSON AWS returns.
func TestIAMPolicyDocumentEqualsYAMLDefined(t *testing.T) {
	defined := &IAMPolicyDocument{
		Version: "2012-10-17",
		Statement: []IAMPolicyStatement{
			{
				Effect: "Allow",
				Action: []string{"s3:PutObject", "s3:GetObject"},
				Resource: []any{
					"arn:aws:s3:::b/*",
					"arn:aws:s3:::a/*",
				},
				Principal: map[any]any{
					"AWS": "arn:aws:iam::123456789012:root",
				},
				Condition: map[any]any{
					"StringEquals": map[any]any{
						"aws:PrincipalOrgID": "o-1234567890",
					},
				},
			},
		},
	}

	existing := mustParsePolicyDocument(t, `{"Version":"2012-10-17","Statement":[
		{"Effect":"Allow",
		 "Action":["s3:GetObject","s3:PutObject"],
		 "Resource":["arn:aws:s3:::a/*","arn:aws:s3:::b/*"],
		 "Principal":{"AWS":["arn:aws:iam::123456789012:root"]},
		 "Condition":{"StringEquals":{"aws:PrincipalOrgID":["o-1234567890"]}}}]}`)

	if got := defined.equals(existing); got != Equal {
		t.Errorf("equals() = %v, want Equal", got)
	}
}
