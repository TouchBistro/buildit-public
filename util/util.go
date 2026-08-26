package util

import (
	"context"
	"fmt"
	"time"
)

// FixMapKeys converts all map keys to strings.
// That is it will convert map[interface{}]interface{} to map[string]interface
//
// This is needed because the go-yaml package unmarshals yaml into
// map[interface{}]interface{} since yaml keys can be values other than strings.
// However for our purposes we always want string keys.
// Solution was adapted from https://github.com/go-yaml/yaml/issues/139#issuecomment-220072190
func FixMapKeys(v interface{}) interface{} {
	switch v := v.(type) {
	case []interface{}:
		nv := make([]interface{}, len(v))
		for i, item := range v {
			nv[i] = FixMapKeys(item)
		}
		return nv
	case map[interface{}]interface{}:
		nv := make(map[string]interface{})
		for k, val := range v {
			nv[fmt.Sprintf("%v", k)] = FixMapKeys(val)
		}
		return nv
	default:
		return v
	}
}

// Contains returns a true if the value str is in one
// of the indeicies os s
func Contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

// ContainsComparable is a generic version of Contains
func ContainsComparable[T comparable](s []T, str T) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

// TODO(@maintainer): This is probably a good candidate for goutils.

func SleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	select {
	case <-ctx.Done():
		// Cancel the timer to free resources.
		timer.Stop()
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// SliceElementsEqual checks if the two slices contain the same elements,
// that is they are equal regardless of order.
func SliceElementsEqual[E comparable](s1, s2 []E) bool {
	// Easy path, if the lengths are different they can't be equal
	if len(s1) != len(s2) {
		return false
	}
	s1Set := make(map[E]struct{})
	for _, v := range s1 {
		s1Set[v] = struct{}{}
	}
	for _, v := range s2 {
		if _, ok := s1Set[v]; !ok {
			return false
		}
	}
	return true
}
