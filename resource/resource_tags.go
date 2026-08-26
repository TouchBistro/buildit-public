package resource

import (
	"context"
	"fmt"
	"maps"

	"github.com/TouchBistro/buildit/util"
)

// ResourceTags is a type definition for a map of resource tags values, or a tag map.
type ResourceTags map[string]string

func (t ResourceTags) Contains(key string) bool {
	_, ok := t[key]
	return ok
}

// Merge appends all key/values from the supplied `other` tag map. If there are duplicate
// keys, then values from this tag map are kept.
//
// Keys in buildit's reserved namespace are the exception: they are dropped from this map
// first, so whatever buildit supplies through `other` wins. Config load rejects reserved
// keys outright (see config/reserved_tags.go); this is the backstop for any path that
// reaches a merge without passing through that check.
func (t ResourceTags) Merge(other ResourceTags) {
	if t == nil {
		return
	}

	maps.DeleteFunc(t, func(k, _ string) bool {
		return util.IsBuilditTagKey(k)
	})

	for k, v := range other {
		if !t.Contains(k) {
			t[k] = v
		}
	}
}

// InheritedTags returns a copy of a parent resource's tags for handing to a nested child:
// a load balancer listener or rule, an EFS access point, an EventBridge target.
//
// Nested items are not top-level buildit resources and have no identifier of their own, so
// they must not carry the parent's buildit tags — a search for a buildit:resource-id would
// otherwise resolve one id to the parent plus every child hanging off it. The whole
// reserved namespace is stripped rather than just the keys buildit writes today, so a
// built-in added later does not start leaking on the day it lands.
func InheritedTags(tags map[string]string) map[string]string {
	if tags == nil {
		return nil
	}

	inherited := maps.Clone(tags)
	maps.DeleteFunc(inherited, func(k, _ string) bool {
		return util.IsBuilditTagKey(k)
	})

	return inherited
}

// TagDiffSummary returns detailed user-facing tag diff messages.
func TagDiffSummary(current map[string]string, diff util.TagDiffResult) []string {
	messages := make([]string, 0, len(diff.Changed)+len(diff.Deleted)+len(diff.Added))

	for _, key := range util.StringMap(diff.Changed).Keys() {
		messages = append(messages, fmt.Sprintf("tag %s is different: %s -> %s", key, current[key], diff.Changed[key]))
	}

	for _, key := range util.StringMap(diff.Deleted).Keys() {
		messages = append(messages, fmt.Sprintf("tag %s:%s is removed", key, current[key]))
	}

	for _, key := range util.StringMap(diff.Added).Keys() {
		messages = append(messages, fmt.Sprintf("tag %s is added: %s", key, diff.Added[key]))
	}

	return messages
}

// TagDiffForContext returns a tag diff and honors ignored tags from context when present.
func TagDiffForContext(ctx context.Context, current map[string]string, desired map[string]string) util.TagDiffResult {
	if ctx != nil {
		ignored := util.IgnoreTagsFromContext(ctx)
		if len(ignored) > 0 {
			return util.TagDiffWithIgnoredKeys(current, desired, ignored)
		}
	}
	return util.TagDiff(current, desired)
}
