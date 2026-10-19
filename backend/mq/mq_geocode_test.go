package mq

import "testing"

func TestBuildGeocodeQuery(t *testing.T) {
	tests := []struct {
		name         string
		address      string
		intersection string
		expected     string
	}{
		{
			name:         "street address ignores appended intersection",
			address:      "3168 DALMAC RD",
			intersection: "FOREST RD & SPRINGHILL RD",
			expected:     "3168 DALMAC RD",
		},
		{
			name:         "swapped intersection prefers civic address",
			address:      "CENOTE RD & STONE SCHOOL RD",
			intersection: "6525 BANK ST",
			expected:     "6525 BANK ST",
		},
		{
			name:         "intersection only remains intersection",
			address:      "FOREST RD & SPRINGHILL RD",
			intersection: "",
			expected:     "FOREST RD & SPRINGHILL RD",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := buildGeocodeQuery(test.address, test.intersection)
			if actual != test.expected {
				t.Fatalf("expected %q, got %q", test.expected, actual)
			}
		})
	}
}
