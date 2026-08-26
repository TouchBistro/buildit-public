package util

// CoalesceInt32 returns the value for the supplied *int32
// or val if ref is nil
func CoalesceInt32(ref *int32, val int32) int32 {
	if ref == nil {
		return val
	}
	return *ref
}

// CoalesceFloat64 returns the value for the supplied *float64
// or val if ref is nil
func CoalesceFloat64(ref *float64, val float64) float64 {
	if ref == nil {
		return val
	}
	return *ref
}

// Coalesce returns the value pointed by the non-nil pointer
// to string. If it's nil, then defaultVal is returned
func Coalesce(val *string, defaulVal string) string {
	if val != nil {
		return *val
	}
	return defaulVal
}

// CoalesceComparable would replace CoalesceXXX() after unit testing
func CoalesceComparable[T comparable](ref *T, val T) T {
	if ref == nil {
		return val
	}
	return *ref
}
