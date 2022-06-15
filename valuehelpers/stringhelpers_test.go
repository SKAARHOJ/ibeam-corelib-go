package valuehelpers

import "testing"

func TestInsertEveryN(t *testing.T) {
	type TestCase struct {
		str      string
		insert   string
		n        int
		expected string
	}

	TestCases := []TestCase{
		{"............", "!", 2, "..!..!..!..!..!.."},
	}

	for _, tc := range TestCases {
		got := InsertEveryN(tc.str, tc.insert, tc.n)
		if got != tc.expected {
			t.Errorf("%s != %s", got, tc.expected)
		}
	}
}
