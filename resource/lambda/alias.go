package lambda

import "github.com/TouchBistro/buildit/resource"

// LambdaAlias defines an function alias
type LambdaAlias struct {
	Name        string             `yaml:"name"`                  // name of alias
	Description string             `yaml:"description"`           // description for alias
	Version     string             `yaml:"version"`               // version of the function
	Policy      *Policy            `yaml:"policy,omitempty"`      // policy for the alias
	FunctionUrl *FunctionUrlConfig `yaml:"functionUrl,omitempty"` // function url for this alias
	//TODO: routing config for weighted routing
}

// compares two lambda aliases & returns a first bool value true if
// the two are equal, the 2nd bool indicates the lambda function is
// also equal
func (a LambdaAlias) equals(other LambdaAlias) (bool, resource.EqualsResult, resource.EqualsResult) {

	// check alias equality
	aliasDiff := a.Description != other.Description ||
		a.Name != other.Name ||
		a.Version != other.Version

	// check alias function url &  equality
	funcEquals := a.FunctionUrl.equals(other.FunctionUrl)
	_, policyEquals := a.Policy.equals(other.Policy)
	eq := !aliasDiff && funcEquals == resource.Equal && policyEquals == resource.Equal
	return eq, funcEquals, policyEquals
}
