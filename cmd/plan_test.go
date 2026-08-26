package cmd

import (
	"reflect"
	"testing"

	"github.com/TouchBistro/buildit/util"
)

func TestNormalizeTagKeys(t *testing.T) {
	input := []string{"bob", " vance ", "", "fish", "bob"}
	expected := []string{"bob", "vance", "fish"}

	result := util.NormalizeStringKeys(input)
	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("unexpected keys: expected=%v got=%v", expected, result)
	}
}
