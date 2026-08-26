package util

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestIsBuilditTagKey(t *testing.T) {
	tests := []struct {
		key      string
		reserved bool
	}{
		{BuilditResourceIDTagKey, true},
		{"buildit:owner", true},
		{"buildit:", true},
		{"x-buildit:owner", false},
		{"iac:created-with", false},
		{"audit", false},
		{"buildit", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := IsBuilditTagKey(tt.key); got != tt.reserved {
				t.Fatalf("IsBuilditTagKey(%q) = %v, want %v", tt.key, got, tt.reserved)
			}
		})
	}
}

func TestSafeTagValue(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"plain name is untouched", "example-bucket", "example-bucket"},
		{"empty stays empty", "", ""},
		{"every legal special character survives", "a/b:c=d+e-f@g", "a/b:c=d+e-f@g"},
		{"spaces survive", "my example bucket", "my example bucket"},
		{"log group path survives", "/aws/lambda/example-fn", "/aws/lambda/example-fn"},
		{"iam policy path survives", "service-role/example-policy", "service-role/example-policy"},

		// The case that would otherwise fail RequestCertificate outright.
		{"wildcard certificate domain", "*.example.com", "_.example.com"},

		{"log group hash", "example#1", "example_1"},
		{"security group brackets", "example (sg) [1]", "example _sg_ _1_"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SafeTagValue(tt.value); got != tt.want {
				t.Fatalf("SafeTagValue(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestSafeTagValueTruncatesToAWSLimit(t *testing.T) {
	got := SafeTagValue(strings.Repeat("a", maxTagValueLen+44))

	if len(got) != maxTagValueLen {
		t.Fatalf("length = %d, want %d", len(got), maxTagValueLen)
	}
}

// AWS counts a tag value in characters, so a multi-byte value must be cut on a rune
// boundary — truncating bytes would leave a partial rune and an invalid value.
func TestSafeTagValueTruncatesOnRuneBoundary(t *testing.T) {
	got := SafeTagValue(strings.Repeat("é", maxTagValueLen+44))

	if n := utf8.RuneCountInString(got); n != maxTagValueLen {
		t.Fatalf("rune count = %d, want %d", n, maxTagValueLen)
	}
	if !utf8.ValidString(got) {
		t.Fatal("truncation produced an invalid utf-8 string")
	}
}
