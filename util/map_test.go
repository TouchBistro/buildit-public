package util

import "testing"

func TestDiffStringToStringMaps(t *testing.T) {

	map1 := map[string]string{
		"a": "A",
		"b": "B",
		"c": "C",
		"d": "D",
	}

	map2 := map[string]string{
		"c": "C",
		"d": "D",
		"b": "B",
		"a": "A",
	}

	map3 := map[string]string{
		"c": "A",
		"d": "B",
		"b": "C",
		"a": "D",
	}

	map4 := map[string]string{
		"c": "c",
		"d": "d",
		"b": "b",
		"a": "a",
	}

	tests := []struct {
		left        map[string]string
		right       map[string]string
		description string
		expected    bool
	}{
		{
			map1, map2, "map1 == map2", true, //not different
		},
		{
			map1, map3, "map1 == map3", false, //different
		},
		{
			map3, map4, "map3 == map4", false, //different
		},
		{
			map2, map4, "map2 == map4", false, //different
		},
		{
			map1, map4, "map1 == map4", false, //different
		},
	}

	for _, test := range tests {

		if found := StringMap(test.left).Equals(test.right); found != test.expected {
			t.Errorf("test: %v, expected %v, found %v", test.description, test.expected, found)
		}
	}

}

func TestStringMapKeys(t *testing.T) {
	m := map[string]string{
		"c": "1",
		"a": "2",
		"b": "3",
	}
	keys := StringMap(m).Keys()
	expected := []string{"a", "b", "c"}

	if len(keys) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(keys))
	}

	for i, k := range keys {
		if k != expected[i] {
			t.Errorf("expected key at index %d to be %s, got %s", i, expected[i], k)
		}
	}
}
