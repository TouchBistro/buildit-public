package resource

import (
	"fmt"
	"strings"

	"github.com/TouchBistro/buildit/client"
)

var DefaultKeyFieldSeparator = "::"

var DefaultKeyParseDelim = "::"

// Key represents the fully-qualified buildit resource name
// if the key does not contain a provider section, `main`
// is assumed
// Once constructed, the Key.string will always use the "::"
// separator
type Key struct {
	string
}

// NewKey creates a new key from a string... vararg
// This builder only considers 1, or the first 2 parameters
// ignore 3rd & onwards parameters if supplied..
// when 1 string passed as param: it is split using "::" as delim & token[0] is provider, token[1] is name of resource
// when 2 strings are passed, the name[0] is provider, & name[1] is name of resource
func NewKey(name ...string) Key {
	return newkey_internal(DefaultKeyParseDelim, name...)
}

// same as NewKey, but uses a custom delimiter supplied as first parameters.
func NewKeyWithDelim(delim string, name ...string) Key {
	return newkey_internal(delim, name...)
}

func newkey_internal(delim string, name ...string) Key {
	provider := client.MainProvider
	id := "resource000"

	if len(name) == 1 {
		provider, id = splitKeyIntoSegements(name[0], delim)
	} else if len(name) > 1 {
		if len(name[0]) != 0 {
			provider = name[0]
		}
		if len(name[1]) != 0 {
			id = name[1]
		}
	}
	return Key{fmt.Sprintf("%v::%v", provider, id)}
}

// NewKeys creates and returns a []Key from []string using the NewKey(name ...) version
// that uses the "::" delim for single parameter
func NewKeys(names []string) []Key {
	var keys []Key
	for _, s := range names {
		keys = append(keys, NewKey(s))
	}
	return keys
}

func (k Key) Split() (string, string) {
	return splitKeyIntoSegements(k.string, DefaultKeyFieldSeparator)
}

// func (k Key) SplitWithDelim(delim string) (string, string) {
// 	return splitKeyIntoSegements(k.string, delim)
// }

func (k Key) Equals(other Key) bool {
	return k.string == other.string
}

func (k Key) EqualsStr(other string) bool {
	return k.string == other
}

func (k Key) String() string {
	return k.string
}

// UnmarshalYAML implements the Unmarsher interface so we ca
// unmarshall a Key from string in yaml
func (e *Key) UnmarshalYAML(unmarshal func(interface{}) error) error {
	rawKey := ""
	err := unmarshal(&rawKey)
	if err != nil {
		return err
	}

	e.string = NewKey(rawKey).string // crazy but good for now..
	return nil
}

// splitKeyIntoSegements helper method to convert a raw string
func splitKeyIntoSegements(s, delim string) (string, string) {
	i := strings.Index(s, delim)
	if i == -1 {
		return client.MainProvider, string(s)
	}
	return s[:i], s[i+len(delim):]
}
