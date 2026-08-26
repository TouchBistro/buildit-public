package util

import (
	"regexp"
	"strings"
)

// StringPtrEquals compares values for 2 string pointers
// and returns true if both pointers are non-nil & point to
// the same value, or if they are nil, they are
func StringPtrEquals(val *string, val2 *string) bool {
	var defaultStr string
	return Coalesce(val, defaultStr) == Coalesce(val2, defaultStr)

}

// ToStringPtr returns a pointer to the supplied string val
func ToStringPtr(val string) *string {
	return &val
}

// RemoveWhitespace removes/purges all whitespaces in the supplied
// string & returns the result
func RemoveWhitespace(str string) string {
	copy := str
	copy = strings.ReplaceAll(copy, " ", "")
	copy = strings.ReplaceAll(copy, "\t", "")
	re := regexp.MustCompile(`\r?\n`)
	copy = re.ReplaceAllString(copy, "")
	return copy
}
