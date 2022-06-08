package valuehelpers

import "testing"

func TestNormaliseFloat(t *testing.T) {
	type TestCase struct {
		in       float32
		minVal   float32
		maxVal   float32
		newMin   float32
		newMax   float32
		expected float32
	}

	TestCases := []TestCase{
		{0, 0, 10, 0, 5, 0},
		{0, 0, 10, 0, 20, 0},
		{0, 0, 10, 5, 20, 5},
		{0, 0, 10, 15, 20, 15},
		{2, 0, 10, 0, 5, 1},
		{4, 0, 10, 0, 5, 2},
		{10, 0, 10, 0, 5, 5},
		{10, 0, 10, 0, 56, 56},
	}

	for _, tc := range TestCases {
		if got := Normalise(tc.in, tc.minVal, tc.maxVal, tc.newMin, tc.newMax); got != tc.expected {
			t.Errorf("Got %f, but expected %f", got, tc.expected)
		}
	}
}
func TestNormaliseInt(t *testing.T) {
	type TestCase struct {
		in       int
		minVal   int
		maxVal   int
		newMin   int
		newMax   int
		expected int
	}

	TestCases := []TestCase{
		{0, 0, 10, 0, 5, 0},
		{0, 0, 10, 0, 20, 0},
		{0, 0, 10, 5, 20, 5},
		{0, 0, 10, 15, 20, 15},
		{2, 0, 10, 0, 5, 1},
		{4, 0, 10, 0, 5, 2},
		{10, 0, 10, 0, 5, 5},
		{10, 0, 10, 0, 60, 60},
	}
	for _, tc := range TestCases {
		if got := Normalise(tc.in, tc.minVal, tc.maxVal, tc.newMin, tc.newMax); got != tc.expected {
			t.Errorf("Got %d, but expected %d", got, tc.expected)
		}
	}
}

func TestConstrain(t *testing.T) {
	type TestCase struct {
		in       float32
		min      float32
		max      float32
		expected float32
	}

	TestCases := []TestCase{
		{0, 0, 10, 0},
		{0, 2, 10, 2},
		{1.9, 2, 10, 2},
		{2.1, 2, 10, 2.1},
		{111.1, 2, 10, 10},
	}

	for _, tc := range TestCases {
		if got := Constrain(tc.in, tc.min, tc.max); got != tc.expected {
			t.Errorf("Got %f, but expected %f", got, tc.expected)
		}
	}
}
