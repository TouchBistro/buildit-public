package util_test

import (
	"reflect"
	"testing"

	"github.com/TouchBistro/buildit/util"
)

func TestFixMapKeys(t *testing.T) {
	in := map[interface{}]interface{}{
		"a": 1,
		"b": "hello",
		"obj": map[interface{}]interface{}{
			"c": true,
			"d": "nested",
		},
		"arr": []interface{}{
			"world",
			1.5,
			map[interface{}]interface{}{
				"key": "value",
			},
		},
	}

	expected := map[string]interface{}{
		"a": 1,
		"b": "hello",
		"obj": map[string]interface{}{
			"c": true,
			"d": "nested",
		},
		"arr": []interface{}{
			"world",
			1.5,
			map[string]interface{}{
				"key": "value",
			},
		},
	}

	out := util.FixMapKeys(in)
	if !reflect.DeepEqual(expected, out) {
		t.Errorf("\nwant: %+v\ngot: %+v", expected, out)
	}
}
