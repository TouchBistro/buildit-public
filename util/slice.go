package util

// StringSlicPtrEquals return true if both *string slices have the same values
func StringPtrSliceEquals(left []*string, right []*string) bool {
	return !DiffStringPtrSlices(left, right)
}

// DiffStringPtrSlices compares 2 *string slices by values, and returns
// true if they are different
func DiffStringPtrSlices(left []*string, right []*string) bool {

	if len(left) != len(right) {
		return true
	}

	rMap := make(map[string]struct{})
	for _, rVal := range right {
		rMap[*rVal] = struct{}{}
	}
	for _, lVal := range left {
		if _, ok := rMap[*lVal]; !ok {
			return true
		}
		delete(rMap, *lVal)
	}

	return len(rMap) > 0
}

// StringSliceEquals return true if both string slices have the same values
func StringSliceEquals(left []string, right []string) bool {
	return !DiffStringSlices(left, right)
}

// DiffStringSlices compares 2 string slices by values, and return true if they
// are different
func DiffStringSlices(left []string, right []string) bool {

	if len(left) != len(right) {
		return true
	}

	rMap := make(map[string]struct{})
	for _, rVal := range right {
		rMap[rVal] = struct{}{}
	}
	for _, lVal := range left {
		if _, ok := rMap[lVal]; !ok {
			return true
		}
		delete(rMap, lVal)
	}

	return len(rMap) > 0
}

// // ToStringArray converts an array of any type to an array of string
// // of the same length by converting each item to a string using String()
// func ToStringArray(in []any) []string {
// 	return ToStringArrayWithFn(in, func(v any) string {
// 		return fmt.Sprintf("%v", v)
// 	})
// }

// // ToStringArray converts an array of any type to an array of string
// // of the same length by converting each item to a string using the supplied fn
// func ToStringArrayWithFn(in []any, fn func(any) string) []string {
// 	var out []string
// 	for _, v := range in {
// 		out = append(out, fn(v))
// 	}
// 	return out
// }

// Convert returns the elements to add to `left` and the elements to remove from `left`
// so that, after applying them, `left` would equal `right` (as sets: order ignored, no dup counts).
func Convert(left []string, right []string) (toAdd []string, toDelete []string) {
	inLeft := make(map[string]struct{}, len(left))
	inRight := make(map[string]struct{}, len(right))

	for _, s := range left {
		inLeft[s] = struct{}{}
	}
	for _, s := range right {
		inRight[s] = struct{}{}
	}

	// in right but not in left
	for s := range inRight {
		if _, ok := inLeft[s]; !ok {
			toAdd = append(toAdd, s)
		}
	}

	// in left but not in right
	for s := range inLeft {
		if _, ok := inRight[s]; !ok {
			toDelete = append(toDelete, s)
		}
	}

	return toAdd, toDelete
}
