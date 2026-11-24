package main

import "testing"

func TestMaxDeploysPerDayConfig(t *testing.T) {
	tests := []struct {
		value string
		want  int
	}{
		{value: "", want: defaultMaxDeploysPerDay},
		{value: "1", want: 1},
		{value: "120", want: 120},
		{value: "0", want: 0},
		{value: "-1", want: defaultMaxDeploysPerDay},
		{value: "1001", want: defaultMaxDeploysPerDay},
		{value: "abc", want: defaultMaxDeploysPerDay},
	}
	for _, test := range tests {
		t.Setenv("MAX_DEPLOYS_PER_DAY", test.value)
		if got := maxDeploysPerDay(); got != test.want {
			t.Fatalf("maxDeploysPerDay(%q) = %d, want %d", test.value, got, test.want)
		}
	}
}
