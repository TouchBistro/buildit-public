package util

import (
	"regexp"
	"strings"
)

// BuilditTagPrefix is reserved for buildit's own AWS tag keys. A buildit config may not
// set a key in this namespace, in globalTags or in a resource's tags — config load rejects
// it. See config/reserved_tags.go.
//
// The pre-existing `audit` tag predates this namespace and deliberately sits outside it:
// it remains user-overridable and `buildit audit` queries for it by that exact key.
const BuilditTagPrefix = "buildit:"

// BuilditResourceIDTagKey holds a resource's buildit identifier, with the provider prefix
// stripped. It lets a resource be found from its tags alone, rather than by guessing at
// whichever field happens to be its name.
const BuilditResourceIDTagKey = BuilditTagPrefix + "resource-id"

// IsBuilditTagKey reports whether key belongs to buildit's reserved tag namespace.
func IsBuilditTagKey(key string) bool {
	return strings.HasPrefix(key, BuilditTagPrefix)
}

// maxTagValueLen is the AWS limit on a tag value, counted in characters.
const maxTagValueLen = 256

// disallowedTagValueChars matches every character AWS rejects in a tag value. AWS permits
// only [\p{L}\p{Z}\p{N}_.:/=+\-@] — notably not '*', which appears in the domain name of a
// wildcard ACM certificate and therefore in that resource's identifier.
var disallowedTagValueChars = regexp.MustCompile(`[^\p{L}\p{Z}\p{N}_.:/=+\-@]`)

// SafeTagValue rewrites s into a value AWS will accept: disallowed characters become '_'
// and the result is capped at the AWS length limit.
//
// Anything that writes a buildit tag and anything that later looks one up must both go
// through this, or the values will not match.
func SafeTagValue(s string) string {
	v := disallowedTagValueChars.ReplaceAllString(s, "_")

	// AWS counts characters, so truncate on runes; cutting bytes could split one.
	if r := []rune(v); len(r) > maxTagValueLen {
		v = string(r[:maxTagValueLen])
	}

	return v
}
