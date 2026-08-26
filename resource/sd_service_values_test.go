package resource

import (
	"context"
	"testing"
)

func TestValidateSDServiceRecordValue(t *testing.T) {
	tests := []struct {
		name       string
		recordType string
		value      string
		wantErr    bool
	}{
		{name: "valid ipv4", recordType: "A", value: "10.0.0.1"},
		{name: "invalid ipv4", recordType: "A", value: "api.example.com", wantErr: true},
		{name: "valid ipv6", recordType: "AAAA", value: "2001:db8::1"},
		{name: "invalid ipv6", recordType: "AAAA", value: "10.0.0.1", wantErr: true},
		{name: "valid cname", recordType: "CNAME", value: "service1.tb.example"},
		{name: "invalid cname", recordType: "CNAME", value: "-service1.tb.example", wantErr: true},
		{name: "srv unsupported", recordType: "SRV", value: "service1.tb.example", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := validateSDServiceRecordValue(tt.recordType, tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateSDServiceRecordValue() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestSDServiceDesiredStaticInstances(t *testing.T) {
	svc := SDService{
		Name:          "test-sd",
		DiscoveryName: "service1",
		Namespace:     "tb.example",
		Records: []SDDnsRecord{
			{
				Type:   "CNAME",
				TTL:    0,
				Values: []string{"service1.tb.example"},
			},
			{
				Type:   "A",
				TTL:    0,
				Values: []string{"10.0.0.5"},
			},
		},
		RoutingPolicy: "WEIGHTED",
	}

	instances, err := svc.desiredStaticInstances()
	if err != nil {
		t.Fatalf("desiredStaticInstances() error = %v", err)
	}
	if len(instances) != 2 {
		t.Fatalf("desiredStaticInstances() count = %v, want 2", len(instances))
	}

	cnameID := sdStaticInstanceID("CNAME", "service1.tb.example")
	if instances[cnameID].AttributeKey != sdAttributeCNAME {
		t.Fatalf("cname attribute key = %v, want %v", instances[cnameID].AttributeKey, sdAttributeCNAME)
	}
	if instances[cnameID].Value != "service1.tb.example" {
		t.Fatalf("cname value = %v, want service1.tb.example", instances[cnameID].Value)
	}

	aID := sdStaticInstanceID("A", "10.0.0.5")
	if instances[aID].AttributeKey != sdAttributeIPv4 {
		t.Fatalf("A attribute key = %v, want %v", instances[aID].AttributeKey, sdAttributeIPv4)
	}
	if instances[aID].Value != "10.0.0.5" {
		t.Fatalf("A value = %v, want 10.0.0.5", instances[aID].Value)
	}
}

func TestSDServiceValidateCNAMERequiresWeighted(t *testing.T) {
	svc := SDService{
		Name:          "test-sd",
		DiscoveryName: "alias2",
		Namespace:     "tb.example",
		RoutingPolicy: "MULTIVALUE",
		Records: []SDDnsRecord{
			{
				Type:   "CNAME",
				TTL:    0,
				Values: []string{"service1.tb.example"},
			},
		},
	}
	svc.Normalize(context.Background())

	err := svc.Validate(context.Background())
	if err == nil {
		t.Fatalf("Validate() error = nil, want non-nil")
	}
}

func TestSDServiceNormalizeRoutingPolicyDefaults(t *testing.T) {
	tests := []struct {
		name       string
		records    []SDDnsRecord
		wantPolicy string
	}{
		{
			name: "defaults to weighted when cname record exists",
			records: []SDDnsRecord{
				{Type: "CNAME", TTL: 0},
			},
			wantPolicy: "WEIGHTED",
		},
		{
			name: "defaults to multivalue when no cname record exists",
			records: []SDDnsRecord{
				{Type: "A", TTL: 0},
			},
			wantPolicy: "MULTIVALUE",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			svc := SDService{
				Name:          "test-sd",
				DiscoveryName: "service",
				Namespace:     "tb.example",
				Records:       tt.records,
			}

			svc.Normalize(context.Background())
			if svc.RoutingPolicy != tt.wantPolicy {
				t.Fatalf("Normalize() routingPolicy = %v, want %v", svc.RoutingPolicy, tt.wantPolicy)
			}
		})
	}
}
