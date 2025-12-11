package yandex

import (
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestInstanceType_String(t *testing.T) {
	testCases := []struct {
		name         string
		instanceType InstanceType
		expected     string
	}{
		{
			name: "Intel Ice Lake with standard resources",
			instanceType: InstanceType{
				Platform:     PlatformIntelIceLake,
				CPU:          resource.MustParse("2"),
				Memory:       resource.MustParse("4Gi"),
				CoreFraction: CoreFraction100,
			},
			expected: "standard-v3_2_4Gi_100",
		},
		{
			name: "AMD EPYC with fractional CPU",
			instanceType: InstanceType{
				Platform:     PlatformAMDZen3,
				CPU:          resource.MustParse("500m"),
				Memory:       resource.MustParse("2048Mi"),
				CoreFraction: CoreFraction50,
			},
			expected: "amd-v1_500m_2Gi_50",
		},
		{
			name: "Intel Broadwell with 5% fraction",
			instanceType: InstanceType{
				Platform:     PlatformIntelBroadwell,
				CPU:          resource.MustParse("4"),
				Memory:       resource.MustParse("8G"),
				CoreFraction: CoreFraction5,
			},
			expected: "standard-v1_4_8G_5",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.instanceType.String()
			if result != tc.expected {
				t.Errorf("Expected: %s, got: %s", tc.expected, result)
			}
		})
	}
}

func TestInstanceType_FromString(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		expected    InstanceType
		expectError bool
	}{
		{
			name:  "Valid Intel Ice Lake instance",
			input: "standard-v3_2_4Gi_100",
			expected: InstanceType{
				Platform:     PlatformIntelIceLake,
				CPU:          resource.MustParse("2"),
				Memory:       resource.MustParse("4Gi"),
				CoreFraction: CoreFraction100,
			},
			expectError: false,
		},
		{
			name:  "Valid AMD EPYC instance",
			input: "amd-v1_500m_2Gi_50",
			expected: InstanceType{
				Platform:     PlatformAMDZen3,
				CPU:          resource.MustParse("500m"),
				Memory:       resource.MustParse("2Gi"),
				CoreFraction: CoreFraction50,
			},
			expectError: false,
		},
		{
			name:  "Valid Intel Broadwell instance",
			input: "standard-v1_4_8G_5",
			expected: InstanceType{
				Platform:     PlatformIntelBroadwell,
				CPU:          resource.MustParse("4"),
				Memory:       resource.MustParse("8G"),
				CoreFraction: CoreFraction5,
			},
			expectError: false,
		},
		{
			name:        "Invalid format - too few parts",
			input:       "standard-v3_2_4Gi",
			expected:    InstanceType{},
			expectError: true,
		},
		{
			name:        "Invalid format - too many parts",
			input:       "standard-v3_2_4Gi_100_extra",
			expected:    InstanceType{},
			expectError: true,
		},
		{
			name:        "Invalid CPU quantity",
			input:       "standard-v3_invalid_4Gi_100",
			expected:    InstanceType{},
			expectError: true,
		},
		{
			name:        "Invalid Memory quantity",
			input:       "standard-v3_2_invalid_100",
			expected:    InstanceType{},
			expectError: true,
		},
		{
			name:        "Invalid CoreFraction",
			input:       "standard-v3_2_4Gi_invalid",
			expected:    InstanceType{},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var result InstanceType
			err := result.FromString(tc.input)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result.Platform != tc.expected.Platform {
				t.Errorf("Platform: expected %v, got %v", tc.expected.Platform, result.Platform)
			}
			if !result.CPU.Equal(tc.expected.CPU) {
				t.Errorf("CPU: expected %v, got %v", tc.expected.CPU, result.CPU)
			}
			if !result.Memory.Equal(tc.expected.Memory) {
				t.Errorf("Memory: expected %v, got %v", tc.expected.Memory, result.Memory)
			}
			if result.CoreFraction != tc.expected.CoreFraction {
				t.Errorf("CoreFraction: expected %v, got %v", tc.expected.CoreFraction, result.CoreFraction)
			}
		})
	}
}

func TestInstanceType_RoundTrip(t *testing.T) {
	// Test that String() and FromString() are inverse operations
	testCases := []InstanceType{
		{
			Platform:     PlatformIntelIceLake,
			CPU:          resource.MustParse("2"),
			Memory:       resource.MustParse("4Gi"),
			CoreFraction: CoreFraction100,
		},
		{
			Platform:     PlatformAMDZen3,
			CPU:          resource.MustParse("1"),
			Memory:       resource.MustParse("2048Mi"),
			CoreFraction: CoreFraction50,
		},
		{
			Platform:     PlatformIntelBroadwell,
			CPU:          resource.MustParse("500m"),
			Memory:       resource.MustParse("1G"),
			CoreFraction: CoreFraction5,
		},
	}

	for i, original := range testCases {
		t.Run(fmt.Sprintf("RoundTrip_%d", i), func(t *testing.T) {
			// Convert to string and back
			str := original.String()
			var parsed InstanceType
			err := parsed.FromString(str)

			if err != nil {
				t.Fatalf("Failed to parse string %s: %v", str, err)
			}

			if parsed.Platform != original.Platform {
				t.Errorf("Platform mismatch: original %v, parsed %v", original.Platform, parsed.Platform)
			}
			if !parsed.CPU.Equal(original.CPU) {
				t.Errorf("CPU mismatch: original %v, parsed %v", original.CPU, parsed.CPU)
			}
			if !parsed.Memory.Equal(original.Memory) {
				t.Errorf("Memory mismatch: original %v, parsed %v", original.Memory, parsed.Memory)
			}
			if parsed.CoreFraction != original.CoreFraction {
				t.Errorf("CoreFraction mismatch: original %v, parsed %v", original.CoreFraction, parsed.CoreFraction)
			}

			t.Logf("Successfully round-tripped: %s", str)
		})
	}
}

func TestCoreFraction_String(t *testing.T) {
	testCases := []struct {
		fraction CoreFraction
		expected string
	}{
		{CoreFraction5, "5"},
		{CoreFraction20, "20"},
		{CoreFraction50, "50"},
		{CoreFraction100, "100"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			result := tc.fraction.String()
			if result != tc.expected {
				t.Errorf("Expected: %s, got: %s", tc.expected, result)
			}
		})
	}
}
