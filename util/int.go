package util

// Int32PtrEquals compares values for 2 int32 pointers
// and returns true if both pointers are non-nil & point to
// the same value, or if they are nil.
func Int32PtrEquals(val *int32, val2 *int32) bool {
	// TODO: Decide whether nil should be distinct from 0; current behavior treats them as equal.
	return CoalesceInt32(val, 0) == CoalesceInt32(val2, 0)
}
