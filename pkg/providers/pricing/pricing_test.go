/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pricing

import (
	"testing"

	"github.com/tufitko/karpenter-provider-yandex/pkg/yandex"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestNewDefaultProvider(t *testing.T) {
	provider := NewDefaultProvider()
	
	if provider == nil {
		t.Fatal("NewDefaultProvider() returned nil")
	}
	
	if provider.mapping == nil {
		t.Fatal("DefaultProvider mapping is nil")
	}
}

func TestOnDemandPrice(t *testing.T) {
	provider := NewDefaultProvider()
	
	testCases := []struct {
		name         string
		instanceType yandex.InstanceType
		expectPrice  bool
		expectedPrice float64
		tolerance     float64
	}{
		{
			name: "Intel Ice Lake 2 CPU 100% 4Gi RAM",
			instanceType: yandex.InstanceType{
				Platform:     yandex.PlatformIntelIceLake,
				CPU:          resource.MustParse("2"),
				Memory:       resource.MustParse("4Gi"),
				CoreFraction: yandex.CoreFraction100,
			},
			expectPrice: true,
			expectedPrice: 1.134*2 + 0.3024*4, // cpuPrice * cores + ramPrice * GB
			tolerance:     0.001,
		},
		{
			name: "AMD EPYC 1 CPU 50% 2Gi RAM",
			instanceType: yandex.InstanceType{
				Platform:     yandex.PlatformAMDZen3,
				CPU:          resource.MustParse("1"),
				Memory:       resource.MustParse("2Gi"),
				CoreFraction: yandex.CoreFraction50,
			},
			expectPrice: true,
			expectedPrice: 0.6912*1 + 0.3024*2, // cpuPrice * cores + ramPrice * GB
			tolerance:     0.001,
		},
		{
			name: "Intel Broadwell 4 CPU 5% 8Gi RAM",
			instanceType: yandex.InstanceType{
				Platform:     yandex.PlatformIntelBroadwell,
				CPU:          resource.MustParse("4"),
				Memory:       resource.MustParse("8Gi"),
				CoreFraction: yandex.CoreFraction5,
			},
			expectPrice: true,
			expectedPrice: 0.3193*4 + 0.4017*8, // cpuPrice * cores + ramPrice * GB
			tolerance:     0.001,
		},
		{
			name: "Unknown platform",
			instanceType: yandex.InstanceType{
				Platform:     yandex.PlatformUnknown,
				CPU:          resource.MustParse("1"),
				Memory:       resource.MustParse("1Gi"),
				CoreFraction: yandex.CoreFraction100,
			},
			expectPrice: false,
			expectedPrice: 0,
			tolerance:     0,
		},
		{
			name: "Unsupported fraction for platform",
			instanceType: yandex.InstanceType{
				Platform:     yandex.PlatformIntelIceLakeComputeOptimized,
				CPU:          resource.MustParse("1"),
				Memory:       resource.MustParse("1Gi"),
				CoreFraction: yandex.CoreFraction5, // This platform doesn't support 5%
			},
			expectPrice: false,
			expectedPrice: 0,
			tolerance:     0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			price, ok := provider.OnDemandPrice(tc.instanceType)
			
			if ok != tc.expectPrice {
				t.Fatalf("Expected price availability: %v, got: %v", tc.expectPrice, ok)
			}
			
			if tc.expectPrice {
				if price <= 0 {
					t.Errorf("Expected positive price, got: %f", price)
				}
				
				diff := price - tc.expectedPrice
				if diff < 0 {
					diff = -diff
				}
				
				if diff > tc.tolerance {
					t.Errorf("Price %.6f differs from expected %.6f by %.6f (tolerance: %.6f)", 
						price, tc.expectedPrice, diff, tc.tolerance)
				}
				
				t.Logf("Instance: %s, Price: %.4f RUB/hour (expected: %.4f)", tc.name, price, tc.expectedPrice)
			}
		})
	}
}

func TestSpotPrice(t *testing.T) {
	provider := NewDefaultProvider()
	
	testCases := []struct {
		name         string
		instanceType yandex.InstanceType
		expectPrice  bool
		expectedPrice float64
		tolerance     float64
	}{
		{
			name: "Intel Ice Lake preemptible 2 CPU 100% 4Gi RAM",
			instanceType: yandex.InstanceType{
				Platform:     yandex.PlatformIntelIceLake,
				CPU:          resource.MustParse("2"),
				Memory:       resource.MustParse("4Gi"),
				CoreFraction: yandex.CoreFraction100,
			},
			expectPrice: true,
			expectedPrice: 0.3132*2 + 0.0756*4, // preemptible cpuPrice * cores + preemptible ramPrice * GB
			tolerance:     0.001,
		},
		{
			name: "AMD EPYC preemptible 1 CPU 50% 2Gi RAM",
			instanceType: yandex.InstanceType{
				Platform:     yandex.PlatformAMDZen3,
				CPU:          resource.MustParse("1"),
				Memory:       resource.MustParse("2Gi"),
				CoreFraction: yandex.CoreFraction50,
			},
			expectPrice: true,
			expectedPrice: 0.2160*1 + 0.0756*2, // preemptible cpuPrice * cores + preemptible ramPrice * GB
			tolerance:     0.001,
		},
		{
			name: "Unknown platform preemptible",
			instanceType: yandex.InstanceType{
				Platform:     yandex.PlatformUnknown,
				CPU:          resource.MustParse("1"),
				Memory:       resource.MustParse("1Gi"),
				CoreFraction: yandex.CoreFraction100,
			},
			expectPrice: false,
			expectedPrice: 0,
			tolerance:     0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			price, ok := provider.SpotPrice(tc.instanceType)
			
			if ok != tc.expectPrice {
				t.Fatalf("Expected spot price availability: %v, got: %v", tc.expectPrice, ok)
			}
			
			if tc.expectPrice {
				if price <= 0 {
					t.Errorf("Expected positive spot price, got: %f", price)
				}
				
				diff := price - tc.expectedPrice
				if diff < 0 {
					diff = -diff
				}
				
				if diff > tc.tolerance {
					t.Errorf("Spot price %.6f differs from expected %.6f by %.6f (tolerance: %.6f)", 
						price, tc.expectedPrice, diff, tc.tolerance)
				}
				
				t.Logf("Instance: %s, Spot Price: %.4f RUB/hour (expected: %.4f)", tc.name, price, tc.expectedPrice)
			}
		})
	}
}

func TestPriceComparison(t *testing.T) {
	provider := NewDefaultProvider()
	
	instanceType := yandex.InstanceType{
		Platform:     yandex.PlatformIntelIceLake,
		CPU:          resource.MustParse("2"),
		Memory:       resource.MustParse("4Gi"),
		CoreFraction: yandex.CoreFraction100,
	}
	
	onDemandPrice, onDemandOk := provider.OnDemandPrice(instanceType)
	spotPrice, spotOk := provider.SpotPrice(instanceType)
	
	if !onDemandOk {
		t.Fatal("Expected on-demand price to be available")
	}
	
	if !spotOk {
		t.Fatal("Expected spot price to be available")
	}
	
	if spotPrice >= onDemandPrice {
		t.Errorf("Expected spot price (%.4f) to be less than on-demand price (%.4f)", spotPrice, onDemandPrice)
	}
	
	t.Logf("On-demand: %.4f RUB/hour, Spot: %.4f RUB/hour (%.1f%% savings)", 
		onDemandPrice, spotPrice, (1-spotPrice/onDemandPrice)*100)
}

func TestResourceQuantityParsing(t *testing.T) {
	provider := NewDefaultProvider()
	
	testCases := []struct {
		name     string
		cpu      string
		memory   string
		platform yandex.PlatformId
		fraction yandex.CoreFraction
	}{
		{"500m CPU 1Gi RAM", "500m", "1Gi", yandex.PlatformIntelIceLake, yandex.CoreFraction100},
		{"2 CPU 4Gi RAM", "2", "4Gi", yandex.PlatformIntelIceLake, yandex.CoreFraction100},
		{"4 CPU 8G RAM", "4", "8G", yandex.PlatformIntelIceLake, yandex.CoreFraction100},
		{"1 CPU 2048Mi RAM", "1", "2048Mi", yandex.PlatformIntelIceLake, yandex.CoreFraction100},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			instanceType := yandex.InstanceType{
				Platform:     tc.platform,
				CPU:          resource.MustParse(tc.cpu),
				Memory:       resource.MustParse(tc.memory),
				CoreFraction: tc.fraction,
			}
			
			price, ok := provider.OnDemandPrice(instanceType)
			if !ok {
				t.Fatalf("Expected price to be available for %s", tc.name)
			}
			
			if price <= 0 {
				t.Errorf("Expected positive price, got: %f", price)
			}
			
			t.Logf("Instance: %s, Price: %.4f RUB/hour", tc.name, price)
		})
	}
}

func TestPricingConsistency(t *testing.T) {
	provider := NewDefaultProvider()
	
	// Test that doubling resources approximately doubles the price
	instanceType1 := yandex.InstanceType{
		Platform:     yandex.PlatformIntelIceLake,
		CPU:          resource.MustParse("1"),
		Memory:       resource.MustParse("2Gi"),
		CoreFraction: yandex.CoreFraction100,
	}
	
	instanceType2 := yandex.InstanceType{
		Platform:     yandex.PlatformIntelIceLake,
		CPU:          resource.MustParse("2"),
		Memory:       resource.MustParse("4Gi"),
		CoreFraction: yandex.CoreFraction100,
	}
	
	price1, ok1 := provider.OnDemandPrice(instanceType1)
	price2, ok2 := provider.OnDemandPrice(instanceType2)
	
	if !ok1 || !ok2 {
		t.Fatal("Expected both prices to be available")
	}
	
	ratio := price2 / price1
	expectedRatio := 2.0
	tolerance := 0.1 // 10% tolerance
	
	if ratio < expectedRatio-tolerance || ratio > expectedRatio+tolerance {
		t.Errorf("Expected price ratio close to %.1f, got: %.2f", expectedRatio, ratio)
	}
	
	t.Logf("1x resources: %.4f RUB/hour, 2x resources: %.4f RUB/hour, ratio: %.2f", 
		price1, price2, ratio)
}

func BenchmarkOnDemandPrice(b *testing.B) {
	provider := NewDefaultProvider()
	
	instanceType := yandex.InstanceType{
		Platform:     yandex.PlatformIntelIceLake,
		CPU:          resource.MustParse("2"),
		Memory:       resource.MustParse("4Gi"),
		CoreFraction: yandex.CoreFraction100,
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = provider.OnDemandPrice(instanceType)
	}
}

func BenchmarkSpotPrice(b *testing.B) {
	provider := NewDefaultProvider()
	
	instanceType := yandex.InstanceType{
		Platform:     yandex.PlatformIntelIceLake,
		CPU:          resource.MustParse("2"),
		Memory:       resource.MustParse("4Gi"),
		CoreFraction: yandex.CoreFraction100,
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = provider.SpotPrice(instanceType)
	}
}