package pricing

import "github.com/tufitko/karpenter-provider-yandex/pkg/yandex"

type pricingPlatform struct {
	perFraction            map[yandex.CoreFraction]float64
	preemptiblePerFraction map[yandex.CoreFraction]float64
	ram                    float64
	preemptibleRAM         float64

	// todo: add pricing per gpu
	// todo: add CVoS support
}
