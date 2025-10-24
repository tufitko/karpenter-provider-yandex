package yandex

import (
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
)

type PlatformId string

const (
	PlatformUnknown PlatformId = "unknown"

	// standard platforms
	PlatformIntelBroadwell   PlatformId = "standard-v1"
	PlatformIntelCascadeLake PlatformId = "standard-v2"
	PlatformIntelIceLake     PlatformId = "standard-v3"
	PlatformAMDZen3          PlatformId = "amd-v1" // ?
	PlatformAMDZen4          PlatformId = "standard-v4a"

	// highfreq platforms
	PlatformIntelIceLakeComputeOptimized PlatformId = "highfreq-v3"
	PlatformAmdZen4ComputeOptimized      PlatformId = "highfreq-v4a"

	// gpu platforms
	PlatformIntelBroadwellNVIDIATeslaV100   PlatformId = "gpu-standard-v1"
	PlatformIntelCascadeLakeNVIDIATeslaV100 PlatformId = "gpu-standard-v2"
	PlatformAMDEPYCNVIDIAAmpereA100         PlatformId = "gpu-standard-v3"
	PlatformAMDEPYC9474FGen2                PlatformId = "gpu-standard-v3i"
	PlatformIntelIceLakeNVIDIATeslaT4       PlatformId = "standard-v3-t4"
	PlatformIntelIceLakeNVIDIATeslaT4i      PlatformId = "standard-v3-t4i"
)

type CoreFraction int64

const (
	CoreFraction5   CoreFraction = 5
	CoreFraction20  CoreFraction = 20
	CoreFraction50  CoreFraction = 50
	CoreFraction100 CoreFraction = 100
)

func (r CoreFraction) String() string {
	return strconv.FormatInt(int64(r), 10)
}

type InstanceType struct {
	Platform     PlatformId
	CPU          resource.Quantity
	Memory       resource.Quantity
	CoreFraction CoreFraction
}

func (r *InstanceType) String() string {
	return fmt.Sprintf("%s_%s_%s_%d", r.Platform, r.CPU.String(), r.Memory.String(), r.CoreFraction)
}

func (r *InstanceType) FromString(str string) error {
	parts := strings.Split(str, "_")
	if len(parts) != 4 {
		return fmt.Errorf("invalid instance type string format: %s", str)
	}

	// Parse platform
	r.Platform = PlatformId(parts[0])

	// Parse CPU - parts[2]
	cpu, err := resource.ParseQuantity(parts[1])
	if err != nil {
		return fmt.Errorf("failed to parse CPU quantity: %w", err)
	}
	r.CPU = cpu

	// Parse Memory - parts[3]
	memory, err := resource.ParseQuantity(parts[2])
	if err != nil {
		return fmt.Errorf("failed to parse Memory quantity: %w", err)
	}
	r.Memory = memory

	// Parse CoreFraction
	fractionStr := parts[3]
	fraction, err := strconv.ParseInt(fractionStr, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse CoreFraction: %w", err)
	}
	r.CoreFraction = CoreFraction(fraction)

	return nil
}
