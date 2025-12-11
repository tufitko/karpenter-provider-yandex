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

// Generated on 2025-12-10 17:52:48 by price_gen tool
package pricing

import "github.com/tufitko/karpenter-provider-yandex/pkg/yandex"

var ruPricing = map[yandex.PlatformId]pricingPlatform{
	yandex.PlatformAMDZen3: {
		perFraction: map[yandex.CoreFraction]float64{
			yandex.CoreFraction20:  0.4752,
			yandex.CoreFraction50:  0.6912,
			yandex.CoreFraction100: 1.1340,
		},
		preemptiblePerFraction: map[yandex.CoreFraction]float64{
			yandex.CoreFraction20:  0.1512,
			yandex.CoreFraction50:  0.2160,
			yandex.CoreFraction100: 0.3132,
		},
		ram:            0.3024,
		preemptibleRAM: 0.0756,
	},
	yandex.PlatformAmdZen4ComputeOptimized: {
		perFraction: map[yandex.CoreFraction]float64{
			yandex.CoreFraction20:  0.4580,
			yandex.CoreFraction50:  1.1450,
			yandex.CoreFraction100: 2.2900,
		},
		preemptiblePerFraction: map[yandex.CoreFraction]float64{
			yandex.CoreFraction20:  0.1374,
			yandex.CoreFraction50:  0.3435,
			yandex.CoreFraction100: 1.6030,
		},
		ram:            0.4200,
		preemptibleRAM: 0.1260,
	},
	yandex.PlatformIntelBroadwell: {
		perFraction: map[yandex.CoreFraction]float64{
			yandex.CoreFraction5:   0.3193,
			yandex.CoreFraction20:  0.9064,
			yandex.CoreFraction100: 1.1536,
		},
		preemptiblePerFraction: map[yandex.CoreFraction]float64{
			yandex.CoreFraction5:   0.1957,
			yandex.CoreFraction20:  0.2781,
			yandex.CoreFraction100: 0.3502,
		},
		ram:            0.4017,
		preemptibleRAM: 0.1236,
	},
	yandex.PlatformIntelCascadeLake: {
		perFraction: map[yandex.CoreFraction]float64{
			yandex.CoreFraction5:   0.1728,
			yandex.CoreFraction20:  0.5292,
			yandex.CoreFraction50:  0.7776,
			yandex.CoreFraction100: 1.2852,
		},
		preemptiblePerFraction: map[yandex.CoreFraction]float64{
			yandex.CoreFraction5:   0.1080,
			yandex.CoreFraction20:  0.1728,
			yandex.CoreFraction50:  0.2376,
			yandex.CoreFraction100: 0.3456,
		},
		ram:            0.3348,
		preemptibleRAM: 0.0756,
	},
	yandex.PlatformIntelIceLake: {
		perFraction: map[yandex.CoreFraction]float64{
			yandex.CoreFraction20:  0.4752,
			yandex.CoreFraction50:  0.6912,
			yandex.CoreFraction100: 1.1340,
		},
		preemptiblePerFraction: map[yandex.CoreFraction]float64{
			yandex.CoreFraction20:  0.1512,
			yandex.CoreFraction50:  0.2160,
			yandex.CoreFraction100: 0.3132,
		},
		ram:            0.3024,
		preemptibleRAM: 0.0756,
	},
	yandex.PlatformIntelIceLakeComputeOptimized: {
		perFraction: map[yandex.CoreFraction]float64{
			yandex.CoreFraction100: 1.9008,
		},
		preemptiblePerFraction: map[yandex.CoreFraction]float64{},
		ram:                    0.3456,
		preemptibleRAM:         0.0000,
	},
}

// Per hour for 1GB of disk storage
var ruDiskPricing = map[yandex.DiskType]float64{
	yandex.SSD:              0.0179,
	yandex.HDD:              0.0044,
	yandex.SSDNonreplicated: 0.0132,
	yandex.SSDIo:            0.0297,
}
