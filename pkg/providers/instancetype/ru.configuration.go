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

// Generated on 2025-09-24 13:47:38 by config_gen tool
package instancetype

import "github.com/tufitko/karpenter-provider-yandex/pkg/yandex"

var ruAvailableConfigurations = map[yandex.PlatformId][]InstanceConfiguration{
	yandex.PlatformAMDEPYC9474FGen2: {
		{
			CoreFraction:     yandex.CoreFraction100,
			VCPU:             []int{ 18, 36, 72, 180 },
			MemoryPerCore:    []float64{ 8.00 },
			CanBePreemptible: true,
		},
	},
	yandex.PlatformAMDEPYCNVIDIAAmpereA100: {
		{
			CoreFraction:     yandex.CoreFraction100,
			VCPU:             []int{ 28, 56, 112, 224 },
			MemoryPerCore:    []float64{ 4.25 },
			CanBePreemptible: true,
		},
	},
	yandex.PlatformIntelBroadwell: {
		{
			CoreFraction:     yandex.CoreFraction5,
			VCPU:             []int{ 2, 4 },
			MemoryPerCore:    []float64{ 0.50, 1.00, 1.50, 2.00 },
			CanBePreemptible: true,
		},
		{
			CoreFraction:     yandex.CoreFraction20,
			VCPU:             []int{ 2, 4 },
			MemoryPerCore:    []float64{ 0.50, 1.00, 1.50, 2.00, 2.50, 3.00, 3.50, 4.00 },
			CanBePreemptible: true,
		},
		{
			CoreFraction:     yandex.CoreFraction100,
			VCPU:             []int{ 2, 4, 6, 8, 10, 12, 14, 16, 20, 24, 28, 32 },
			MemoryPerCore:    []float64{ 1.00, 2.00, 3.00, 4.00, 5.00, 6.00, 7.00, 8.00 },
			CanBePreemptible: true,
		},
	},
	yandex.PlatformIntelBroadwellNVIDIATeslaV100: {
		{
			CoreFraction:     yandex.CoreFraction100,
			VCPU:             []int{ 8, 16, 32 },
			MemoryPerCore:    []float64{ 12.00 },
			CanBePreemptible: true,
		},
	},
	yandex.PlatformIntelCascadeLake: {
		{
			CoreFraction:     yandex.CoreFraction5,
			VCPU:             []int{ 2, 4 },
			MemoryPerCore:    []float64{ 0.25, 0.50, 1.00, 1.50, 2.00 },
			CanBePreemptible: true,
		},
		{
			CoreFraction:     yandex.CoreFraction20,
			VCPU:             []int{ 2, 4 },
			MemoryPerCore:    []float64{ 0.50, 1.00, 1.50, 2.00, 2.50, 3.00, 3.50, 4.00 },
			CanBePreemptible: true,
		},
		{
			CoreFraction:     yandex.CoreFraction50,
			VCPU:             []int{ 2, 4 },
			MemoryPerCore:    []float64{ 0.50, 1.00, 1.50, 2.00, 2.50, 3.00, 3.50, 4.00 },
			CanBePreemptible: true,
		},
		{
			CoreFraction:     yandex.CoreFraction100,
			VCPU:             []int{ 2, 4, 6, 8, 10, 12, 14, 16, 20, 24, 28, 32, 36, 40, 44, 48, 52, 56, 60, 64, 68, 72, 76, 80 },
			MemoryPerCore:    []float64{ 1.00, 2.00, 3.00, 4.00, 5.00, 6.00, 7.00, 8.00, 9.00, 10.00, 11.00, 12.00, 13.00, 14.00, 15.00, 16.00 },
			CanBePreemptible: true,
		},
	},
	yandex.PlatformIntelCascadeLakeNVIDIATeslaV100: {
		{
			CoreFraction:     yandex.CoreFraction100,
			VCPU:             []int{ 8, 16, 32, 64 },
			MemoryPerCore:    []float64{ 6.00 },
			CanBePreemptible: true,
		},
	},
	yandex.PlatformIntelIceLake: {
		{
			CoreFraction:     yandex.CoreFraction20,
			VCPU:             []int{ 2, 4 },
			MemoryPerCore:    []float64{ 0.50, 1.00, 1.50, 2.00, 2.50, 3.00, 3.50, 4.00 },
			CanBePreemptible: true,
		},
		{
			CoreFraction:     yandex.CoreFraction50,
			VCPU:             []int{ 2, 4 },
			MemoryPerCore:    []float64{ 0.50, 1.00, 1.50, 2.00, 2.50, 3.00, 3.50, 4.00 },
			CanBePreemptible: true,
		},
		{
			CoreFraction:     yandex.CoreFraction100,
			VCPU:             []int{ 2, 4, 6, 8, 10, 12, 14, 16, 20, 24, 28, 32, 36, 40, 44, 48, 52, 56, 60, 64, 68, 72, 76, 80, 84, 88, 92, 96 },
			MemoryPerCore:    []float64{ 1.00, 2.00, 3.00, 4.00, 5.00, 6.00, 7.00, 8.00, 9.00, 10.00, 11.00, 12.00, 13.00, 14.00, 15.00, 16.00 },
			CanBePreemptible: true,
		},
	},
	yandex.PlatformIntelIceLakeComputeOptimized: {
		{
			CoreFraction:     yandex.CoreFraction100,
			VCPU:             []int{ 2, 4, 6, 8, 10, 12, 14, 16, 20, 24, 28, 32, 36, 40, 44, 48, 52, 56 },
			MemoryPerCore:    []float64{ 1.00, 2.00, 3.00, 4.00, 5.00, 6.00, 7.00, 8.00, 9.00, 10.00, 11.00, 12.00, 13.00, 14.00, 15.00, 16.00 },
			CanBePreemptible: false,
		},
	},
	yandex.PlatformIntelIceLakeNVIDIATeslaT4: {
		{
			CoreFraction:     yandex.CoreFraction100,
			VCPU:             []int{ 4, 8, 16, 32 },
			MemoryPerCore:    []float64{ 4.00 },
			CanBePreemptible: true,
		},
	},
	yandex.PlatformIntelIceLakeNVIDIATeslaT4i: {
		{
			CoreFraction:     yandex.CoreFraction100,
			VCPU:             []int{ 4, 8, 16, 32 },
			MemoryPerCore:    []float64{ 4.00 },
			CanBePreemptible: true,
		},
	},
}
