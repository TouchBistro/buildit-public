package util

import (
	"maps"
	"regexp"
	"sort"
	"strings"
)

type StringMap map[string]string

// NormalizeStringKeys deduplicates and trims whitespace from a slice of strings,
// dropping empty entries.
func NormalizeStringKeys(keys []string) []string {
	seen := make(map[string]struct{}, len(keys))
	normalized := make([]string, 0, len(keys))
	for _, key := range keys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

// TagDiffResult captures the tag delta between current and desired tags.
type TagDiffResult struct {
	Added   map[string]string
	Deleted map[string]string
	Changed map[string]string
}

// TagDiff returns the Added, Deleted, and Changed tags between current and desired maps.
func TagDiff(current map[string]string, desired map[string]string) TagDiffResult {
	diff := TagDiffResult{
		Added:   map[string]string{},
		Deleted: map[string]string{},
		Changed: map[string]string{},
	}

	for key, desiredValue := range desired {
		currentValue, exists := current[key]
		if !exists {
			diff.Added[key] = desiredValue
			continue
		}

		if currentValue != desiredValue {
			diff.Changed[key] = desiredValue
		}
	}

	for key, currentValue := range current {
		if _, exists := desired[key]; !exists {
			diff.Deleted[key] = currentValue
		}
	}

	return diff
}

// TagDiffWithIgnoredKeys returns the tag diff after dropping ignored keys from both maps.
func TagDiffWithIgnoredKeys(current map[string]string, desired map[string]string, ignoredKeys []string) TagDiffResult {
	if len(ignoredKeys) == 0 {
		return TagDiff(current, desired)
	}

	normalized := NormalizeStringKeys(ignoredKeys)
	if len(normalized) == 0 {
		return TagDiff(current, desired)
	}

	ignored := make(map[string]struct{}, len(normalized))
	for _, key := range normalized {
		ignored[key] = struct{}{}
	}

	return TagDiff(
		FilterStringMapIgnoringKeys(current, ignored),
		FilterStringMapIgnoringKeys(desired, ignored),
	)
}

// FilterStringMapIgnoringKeys returns a shallow copy of input without keys in ignored.
// If ignored is nil, it returns input as is.
func FilterStringMapIgnoringKeys(input map[string]string, ignored map[string]struct{}) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	if ignored == nil {
		return input
	}
	filtered := make(map[string]string, len(input))

	for key, value := range input {
		if _, skip := ignored[key]; skip {
			continue
		}
		filtered[key] = value
	}

	return filtered
}

// HasChanges returns true when Added, Deleted, or Changed contains entries.
func (d TagDiffResult) HasChanges() bool {
	return len(d.Added) > 0 || len(d.Deleted) > 0 || len(d.Changed) > 0
}

// Upserts returns a merged map containing Added and Changed entries.
func (d TagDiffResult) Upserts() map[string]string {
	upserts := make(map[string]string, len(d.Added)+len(d.Changed))
	maps.Copy(upserts, d.Added)
	maps.Copy(upserts, d.Changed)
	return upserts
}

// DeletedKeys returns deleted tag keys in sorted order.
func (d TagDiffResult) DeletedKeys() []string {
	keys := make([]string, 0, len(d.Deleted))
	for key := range d.Deleted {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Keys returns the keys of the map in sorted order
func (m StringMap) Keys() []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Equals compares this map to another map[string]string and returns
// true if all keys/values are the same;
func (m StringMap) Equals(right map[string]string) bool {

	left := m

	if len(left) != len(right) {
		return false //different
	}

	// if same size, compare key by key
	for lKey, lVal := range left {
		rVal, exists := right[lKey]
		if !exists {
			return false //left key doesn't exists in right map; !equal
		} else if rVal != lVal {
			return false //left key exists, but values don't match, !equal
		}
	}
	return true //equal
}

// Convert compares this map with `to` & returns a map[string]string
// of keys/values to add, and a map[string]string of keys/keys to remove from the this map` to
// make it the same as `to`
func (m StringMap) Convert(to map[string]string) (map[string]string, map[string]string) {

	from := m
	to2 := make(map[string]string)

	// make a copy of the right
	maps.Copy(to2, to)

	add := make(map[string]string)
	remove := make(map[string]string)

	for fKey, fVal := range from {
		if tVal, ok := to2[fKey]; !ok {
			// old key doesn't exist in `to` map
			remove[fKey] = fKey
		} else {
			// old key exists, but value is different, add
			if fVal != tVal {
				add[fKey] = tVal // add key with new value
			}
			delete(to2, fKey) //remove from to2
		}
	}
	// add remaining keys; that weren't found to add map
	maps.Copy(add, to2)

	return add, remove
}

// FindMatchingKeysGen takes a regular expression & a map[K]V where K:comparable & T:any
// as an input and checks if any of the keys match the expression; matching keys are returned, else a
// non-nil error if there a problem parsing the regex etc
func FindMatchingKeys[K comparable, V any](regex string, target map[K]V, convert_to_byte_array_fn func(K) []byte) ([]K, error) {

	var err error
	var regexObj *regexp.Regexp
	if regexObj, err = regexp.Compile(regex); err != nil {
		return nil, err
	}

	var matchingKeys []K
	for k := range target {
		if regexObj.Match(convert_to_byte_array_fn(k)) {
			matchingKeys = append(matchingKeys, k)
		}
	}
	return matchingKeys, nil
}

// MapConvert compares this map with `to` & returns a map[string]string
// of keys/values to add, and a map[string]string of keys/keys to remove from the this map` to
// make it the same as `to`
func MapConvert[T any](from map[string]T, to map[string]T, equalsFn func(T, T) bool) (map[string]T, map[string]T, map[string]T) {
	to2 := make(map[string]T)

	// make a copy of the target map
	maps.Copy(to2, to)

	add := make(map[string]T)
	remove := make(map[string]T)
	update := make(map[string]T)

	for fKey, fVal := range from {
		if tVal, ok := to2[fKey]; !ok {
			// old key doesn't exist in `to` map
			var zero T
			remove[fKey] = zero
		} else {
			// old key exists, but value is different, update
			if !equalsFn(fVal, tVal) {
				update[fKey] = tVal // add key with new value
			}
			delete(to2, fKey) // remove from to2
		}
	}
	// add remaining keys that weren't found in from map
	maps.Copy(add, to2)

	return add, remove, update
}

// MapEquals compares this map to another map[string]any and returns
// true if all keys/values are the same;
func MapEquals[T any](left map[string]T, right map[string]T, equalsFn func(T, T) bool) bool {

	if len(left) != len(right) {
		return false //different
	}

	// if same size, compare key by key
	for lKey, lVal := range left {
		rVal, exists := right[lKey]
		if !exists {
			return false //left key doesn't exists in right map; !equal
		} else if !equalsFn(rVal, lVal) {
			return false //left key exists, but values don't match, !equal
		}
	}
	return true //equal
}
