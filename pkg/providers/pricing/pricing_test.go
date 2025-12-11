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
		name          string
		instanceType  yandex.InstanceType
		expectPrice   bool
		expectedPrice float64
		tolerance     float64
	}{
		{
			name: "Intel Ice Lake 2 CPU 100% 4Gi RAM with SSD 30GB",
			instanceType: yandex.InstanceType{
				Platform:     yandex.PlatformIntelIceLake,
				CPU:          resource.MustParse("2"),
				Memory:       resource.MustParse("4Gi"),
				CoreFraction: yandex.CoreFraction100,
			},
			expectPrice:   true,
			expectedPrice: 1.134*2 + 0.3024*4, // cpuPrice * cores + ramPrice * GB
			tolerance:     0.001,
		},
		{
			name: "AMD EPYC 1 CPU 50% 2Gi RAM with HDD 50GB",
			instanceType: yandex.InstanceType{
				Platform:     yandex.PlatformAMDZen3,
				CPU:          resource.MustParse("1"),
				Memory:       resource.MustParse("2Gi"),
				CoreFraction: yandex.CoreFraction50,
			},
			expectPrice:   true,
			expectedPrice: 0.6912*1 + 0.3024*2, // cpuPrice * cores + ramPrice * GB
			tolerance:     0.001,
		},
		{
			name: "Intel Broadwell 4 CPU 5% 8Gi RAM with SSD-non-replicated 93GB",
			instanceType: yandex.InstanceType{
				Platform:     yandex.PlatformIntelBroadwell,
				CPU:          resource.MustParse("4"),
				Memory:       resource.MustParse("8Gi"),
				CoreFraction: yandex.CoreFraction5,
			},
			expectPrice:   true,
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
			expectPrice:   false,
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
			expectPrice:   false,
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
		name          string
		instanceType  yandex.InstanceType
		expectPrice   bool
		expectedPrice float64
		tolerance     float64
	}{
		{
			name: "Intel Ice Lake preemptible 2 CPU 100% 4Gi RAM with SSD 30GB",
			instanceType: yandex.InstanceType{
				Platform:     yandex.PlatformIntelIceLake,
				CPU:          resource.MustParse("2"),
				Memory:       resource.MustParse("4Gi"),
				CoreFraction: yandex.CoreFraction100,
			},
			expectPrice:   true,
			expectedPrice: 0.3132*2 + 0.0756*4, // preemptible cpuPrice * cores + preemptible ramPrice * GB
			tolerance:     0.001,
		},
		{
			name: "AMD EPYC preemptible 1 CPU 50% 2Gi RAM with HDD 50GB",
			instanceType: yandex.InstanceType{
				Platform:     yandex.PlatformAMDZen3,
				CPU:          resource.MustParse("1"),
				Memory:       resource.MustParse("2Gi"),
				CoreFraction: yandex.CoreFraction50,
			},
			expectPrice:   true,
			expectedPrice: 0.2160*1 + 0.0756*2,
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
			expectPrice:   false,
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
		diskType yandex.DiskType
		diskSize int64
	}{
		{"500m CPU 1Gi RAM", "500m", "1Gi", yandex.PlatformIntelIceLake, yandex.CoreFraction100, yandex.SSD, 30},
		{"2 CPU 4Gi RAM", "2", "4Gi", yandex.PlatformIntelIceLake, yandex.CoreFraction100, yandex.SSD, 30},
		{"4 CPU 8G RAM", "4", "8G", yandex.PlatformIntelIceLake, yandex.CoreFraction100, yandex.HDD, 50},
		{"1 CPU 2048Mi RAM", "1", "2048Mi", yandex.PlatformIntelIceLake, yandex.CoreFraction100, yandex.SSD, 30},
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

func TestDiskPrice(t *testing.T) {
	provider := NewDefaultProvider()

	testCases := []struct {
		name          string
		disk          yandex.Disk
		expectPrice   bool
		expectedPrice float64
		tolerance     float64
	}{
		{
			name: "SSD 30GB",
			disk: yandex.Disk{
				Type: yandex.SSD,
				Size: 30,
			},
			expectPrice:   true,
			expectedPrice: 0.0179 * 30,
			tolerance:     0.001,
		},
		{
			name: "SSDIO 93GB",
			disk: yandex.Disk{
				Type: yandex.SSDIo,
				Size: 93,
			},
			expectPrice:   true,
			expectedPrice: 0.0297 * 93,
			tolerance:     0.001,
		},
		{
			name: "SSDIO 279GB (multiple of 93)",
			disk: yandex.Disk{
				Type: yandex.SSDIo,
				Size: 279,
			},
			expectPrice:   true,
			expectedPrice: 0.0297 * 279,
			tolerance:     0.001,
		},
		{
			name: "HDD 200GB",
			disk: yandex.Disk{
				Type: yandex.HDD,
				Size: 200,
			},
			expectPrice:   true,
			expectedPrice: 0.0044 * 200,
			tolerance:     0.001,
		},
		{
			name: "Unknown disk type",
			disk: yandex.Disk{
				Type: yandex.DiskType("unknown"),
				Size: 30,
			},
			expectPrice:   false,
			expectedPrice: 0,
			tolerance:     0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			price, ok := provider.DiskPrice(tc.disk)

			if ok != tc.expectPrice {
				t.Fatalf("Expected price availability: %v, got: %v", tc.expectPrice, ok)
			}

			if tc.expectPrice {
				if price < 0 {
					t.Errorf("Expected non-negative price, got: %f", price)
				}

				diff := price - tc.expectedPrice
				if diff < 0 {
					diff = -diff
				}

				if diff > tc.tolerance {
					t.Errorf("Price %.6f differs from expected %.6f by %.6f (tolerance: %.6f)",
						price, tc.expectedPrice, diff, tc.tolerance)
				}

				t.Logf("Disk: %s %dGB, Price: %.4f RUB/hour (expected: %.4f)", tc.disk.Type, tc.disk.Size, price, tc.expectedPrice)
			}
		})
	}
}

func TestDiskPriceWithInstanceType(t *testing.T) {
	provider := NewDefaultProvider()

	testCases := []struct {
		name          string
		instanceType  yandex.InstanceType
		disk          yandex.Disk
		expectPrice   bool
		expectedTotal float64
		tolerance     float64
	}{
		{
			name: "Instance with SSD 30GB",
			instanceType: yandex.InstanceType{
				Platform:     yandex.PlatformIntelIceLake,
				CPU:          resource.MustParse("2"),
				Memory:       resource.MustParse("4Gi"),
				CoreFraction: yandex.CoreFraction100,
			},
			disk: yandex.Disk{
				Type: yandex.SSD,
				Size: 30,
			},
			expectPrice:   true,
			expectedTotal: 1.134*2 + 0.3024*4 + 0.0179*30, // cpu + memory + disk
			tolerance:     0.001,
		},
		{
			name: "Instance with SSD-non-replicated 93GB",
			instanceType: yandex.InstanceType{
				Platform:     yandex.PlatformIntelIceLake,
				CPU:          resource.MustParse("2"),
				Memory:       resource.MustParse("4Gi"),
				CoreFraction: yandex.CoreFraction100,
			},
			disk: yandex.Disk{
				Type: yandex.SSDNonreplicated,
				Size: 93,
			},
			expectPrice:   true,
			expectedTotal: 1.134*2 + 0.3024*4 + 0.0132*93,
			tolerance:     0.001,
		},
		{
			name: "Instance with SSDIO 186GB",
			instanceType: yandex.InstanceType{
				Platform:     yandex.PlatformIntelIceLake,
				CPU:          resource.MustParse("2"),
				Memory:       resource.MustParse("4Gi"),
				CoreFraction: yandex.CoreFraction100,
			},
			disk: yandex.Disk{
				Type: yandex.SSDIo,
				Size: 186,
			},
			expectPrice:   true,
			expectedTotal: 1.134*2 + 0.3024*4 + 0.0297*186,
			tolerance:     0.001,
		},
		{
			name: "Instance with HDD 50GB",
			instanceType: yandex.InstanceType{
				Platform:     yandex.PlatformAMDZen3,
				CPU:          resource.MustParse("1"),
				Memory:       resource.MustParse("2Gi"),
				CoreFraction: yandex.CoreFraction50,
			},
			disk: yandex.Disk{
				Type: yandex.HDD,
				Size: 50,
			},
			expectPrice:   true,
			expectedTotal: 0.6912*1 + 0.3024*2 + 0.0044*50,
			tolerance:     0.001,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			instancePrice, instanceOk := provider.OnDemandPrice(tc.instanceType)
			diskPrice, diskOk := provider.DiskPrice(tc.disk)

			if !instanceOk {
				t.Fatalf("Expected instance price to be available")
			}

			if diskOk != tc.expectPrice {
				t.Fatalf("Expected disk price availability: %v, got: %v", tc.expectPrice, diskOk)
			}

			totalPrice := instancePrice + diskPrice

			if tc.expectPrice {
				if totalPrice <= 0 {
					t.Errorf("Expected positive total price, got: %f", totalPrice)
				}

				diff := totalPrice - tc.expectedTotal
				if diff < 0 {
					diff = -diff
				}

				if diff > tc.tolerance {
					t.Errorf("Total price %.6f differs from expected %.6f by %.6f (tolerance: %.6f)",
						totalPrice, tc.expectedTotal, diff, tc.tolerance)
				}

				t.Logf("Instance: %s, Instance Price: %.4f RUB/hour, Disk Price: %.4f RUB/hour, Total: %.4f RUB/hour",
					tc.name, instancePrice, diskPrice, totalPrice)
			}
		})
	}
}

func TestDiskPriceComparison(t *testing.T) {
	provider := NewDefaultProvider()

	// Test that larger disks cost more
	smallDisk := yandex.Disk{Type: yandex.SSD, Size: 30}
	largeDisk := yandex.Disk{Type: yandex.SSD, Size: 100}

	smallPrice, smallOk := provider.DiskPrice(smallDisk)
	largePrice, largeOk := provider.DiskPrice(largeDisk)

	if !smallOk || !largeOk {
		t.Fatal("Expected both disk prices to be available")
	}

	if largePrice <= smallPrice {
		t.Errorf("Expected larger disk (%.4f) to cost more than smaller disk (%.4f)", largePrice, smallPrice)
	}

	t.Logf("Small disk (30GB): %.4f RUB/hour, Large disk (100GB): %.4f RUB/hour",
		smallPrice, largePrice)
}

func TestDiskPriceByType(t *testing.T) {
	provider := NewDefaultProvider()

	// Test that different disk types have different prices for the same size
	size := int64(100)
	ssd := yandex.Disk{Type: yandex.SSD, Size: size}
	hdd := yandex.Disk{Type: yandex.HDD, Size: size}
	ssdNonrep := yandex.Disk{Type: yandex.SSDNonreplicated, Size: size}
	ssdIo := yandex.Disk{Type: yandex.SSDIo, Size: size}

	ssdPrice, _ := provider.DiskPrice(ssd)
	hddPrice, _ := provider.DiskPrice(hdd)
	ssdNonrepPrice, _ := provider.DiskPrice(ssdNonrep)
	ssdIoPrice, _ := provider.DiskPrice(ssdIo)

	// HDD should be cheapest
	if hddPrice >= ssdPrice {
		t.Errorf("Expected HDD (%.4f) to be cheaper than SSD (%.4f)", hddPrice, ssdPrice)
	}

	// SSDIO should be most expensive
	if ssdIoPrice <= ssdPrice {
		t.Errorf("Expected SSDIO (%.4f) to be more expensive than SSD (%.4f)", ssdIoPrice, ssdPrice)
	}

	t.Logf("100GB disk prices - HDD: %.4f, SSD: %.4f, SSD-non-replicated: %.4f, SSDIO: %.4f RUB/hour",
		hddPrice, ssdPrice, ssdNonrepPrice, ssdIoPrice)
}
