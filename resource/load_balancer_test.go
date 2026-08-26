package resource

import (
	"reflect"
	"testing"
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
