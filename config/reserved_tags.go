package config

import (
	"fmt"
	"slices"

	"github.com/TouchBistro/buildit/resource"
	"github.com/TouchBistro/buildit/util"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// checkReservedTags rejects tag keys a config placed in buildit's reserved namespace.
//
// It has to see the tags as parsed, before Normalize merges buildit's own keys in — after
// that point a user's key is indistinguishable from ours, which is why this cannot live in
// a resource's Validate().
func checkReservedTags(resourceType, name string, tags map[string]string) error {
	reserved := reservedKeysIn(tags)
	if len(reserved) == 0 {
		return nil
	}

	msgs := make([]string, 0, len(reserved))
	for _, key := range reserved {
		msgs = append(msgs, fmt.Sprintf("tag key %q uses the reserved %q prefix", key, util.BuilditTagPrefix))
	}

	return &resource.ValidationError{
		ResourceIdentifier: name,
		ResourceType:       resourceType,
		Messages:           msgs,
	}
}

// checkBuilditTagsApplied verifies buildit's own tags actually reached a resource once it
// has been normalized. A resource type whose Generate block was never wired to tagsFor
// would otherwise ship silently untagged — which is how eventbridge-connection's tags came
// to be dead for as long as they were.
func checkBuilditTagsApplied(resourceType, name string, tags map[string]string) error {
	if _, ok := tags[util.BuilditResourceIDTagKey]; ok {
		return nil
	}

	return &resource.ValidationError{
		ResourceIdentifier: name,
		ResourceType:       resourceType,
		Messages: []string{fmt.Sprintf(
			"buildit tag %q was not applied; this resource type is missing its tagsFor wiring in Generate",
			util.BuilditResourceIDTagKey)},
	}
}

// checkReservedGlobalTags rejects reserved keys in the globalTags of every collected
// config and of the override.
//
// Checked once per file, and early: a reserved key here is copied onto every resource, so
// leaving it to the per-resource checks would report the same mistake once per resource.
// Failing here also means a bad config costs no AWS calls.
func (i *InternalConfig) checkReservedGlobalTags() error {
	configs := i._config
	if i._override != nil {
		configs = append(slices.Clone(configs), *i._override)
	}

	var errs []error

	for _, c := range configs {
		source := c.Source
		if source == "" {
			source = "config"
		}

		for _, key := range reservedKeysIn(c.GlobalTags) {
			errs = append(errs, errors.Errorf("globalTags in %v: tag key %q uses the reserved %q prefix",
				source, key, util.BuilditTagPrefix))
		}
	}

	if len(errs) == 0 {
		return nil
	}

	for _, err := range errs {
		log.Error(err)
	}

	return errors.Errorf("%v globalTags key(s) use the reserved %q prefix, which is owned by buildit",
		len(errs), util.BuilditTagPrefix)
}

// reservedKeysIn returns a tag map's reserved keys, sorted so that a config with several
// offenders reports them in a stable order.
func reservedKeysIn(tags map[string]string) []string {
	var found []string

	for key := range tags {
		if util.IsBuilditTagKey(key) {
			found = append(found, key)
		}
	}
	slices.Sort(found)

	return found
}
