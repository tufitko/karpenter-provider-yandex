package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"text/template"
	"time"

	"github.com/tufitko/karpenter-provider-yandex/pkg/yandex"
)

const (
	baseURL = "https://yandex.cloud/api/prices/compute/config"
)

type ConfigResponse struct {
	OSProducts []OSProduct `json:"osProducts"`
	Platforms  []Platform  `json:"platforms"`
}

type OSProduct struct {
	Name         string       `json:"name"`
	ID           string       `json:"id"`
	ResourceSpec ResourceSpec `json:"resourceSpec"`
}

type ResourceSpec struct {
	CalcPlatform     string   `json:"calcPlatform"`
	ComputePlatforms []string `json:"computePlatforms"`
	Cores            int      `json:"cores"`
	DiskSize         int64    `json:"diskSize"`
	Memory           int64    `json:"memory"`
	UserDataFormID   string   `json:"userDataFormId"`
}

type Platform struct {
	AllowedConfigurations    []AllowedConfiguration    `json:"allowedConfigurations"`
	AllowedGpuConfigurations []AllowedGpuConfiguration `json:"allowedGpuConfigurations"`
	ZoneIDs                  []string                  `json:"zoneIds"`
	ID                       string                    `json:"id"`
	IsDefault                bool                      `json:"isDefault"`
	Name                     string                    `json:"name"`
	RejectPreemptible        bool                      `json:"rejectPreemptible"`
}

type AllowedConfiguration struct {
	Cores                           interface{} `json:"cores"`
	SoftwareAcceleratedNetworkCores []string    `json:"softwareAcceleratedNetworkCores"`
	MemoryPerCore                   []string    `json:"memoryPerCore"`
	CoresDedicated                  interface{} `json:"coresDedicated"`
	MemoryPerCoreDedicated          interface{} `json:"memoryPerCoreDedicated"`
	CoreFraction                    string      `json:"coreFraction"`
	MaxMemory                       string      `json:"maxMemory"`
	MaxMemoryPerNumaNode            string      `json:"maxMemoryPerNumaNode"`
}

type CoreConfig struct {
	Cores   []string `json:"cores"`
	Sockets string   `json:"sockets"`
}

type AllowedGpuConfiguration struct {
	GPUs         string `json:"gpus"`
	Cores        string `json:"cores"`
	Interconnect bool   `json:"interconnect"`
}

type InstanceConfiguration struct {
	CoreFraction     yandex.CoreFraction
	VCPU             []int
	MemoryPerCore    []float64
	CanBePreemptible bool
}

type RegionConfig struct {
	Region         string
	Configurations map[yandex.PlatformId][]InstanceConfiguration
}

var platformMapping = map[string]yandex.PlatformId{
	"standard-v1":      yandex.PlatformIntelBroadwell,
	"standard-v2":      yandex.PlatformIntelCascadeLake,
	"standard-v3":      yandex.PlatformIntelIceLake,
	"amd-v1":           yandex.PlatformAMDZen3,
	"standard-v4a":     yandex.PlatformAMDZen4,
	"highfreq-v3":      yandex.PlatformIntelIceLakeComputeOptimized,
	"highfreq-v4a":     yandex.PlatformAmdZen4ComputeOptimized,
	"gpu-standard-v1":  yandex.PlatformIntelBroadwellNVIDIATeslaV100,
	"gpu-standard-v2":  yandex.PlatformIntelCascadeLakeNVIDIATeslaV100,
	"gpu-standard-v3":  yandex.PlatformAMDEPYCNVIDIAAmpereA100,
	"gpu-standard-v3i": yandex.PlatformAMDEPYC9474FGen2,
	"standard-v3-t4":   yandex.PlatformIntelIceLakeNVIDIATeslaT4,
	"standard-v3-t4i":  yandex.PlatformIntelIceLakeNVIDIATeslaT4i,
}

const configTemplate = `/*
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

// Generated on {{.Timestamp}} by config_gen tool
package instancetype

import "github.com/tufitko/karpenter-provider-yandex/pkg/yandex"

var {{.Region}}AvailableConfigurations = map[yandex.PlatformId][]InstanceConfiguration{
{{range $platformId, $configs := .Configurations}}	yandex.{{$platformId}}: {
{{range $config := $configs}}		{
			CoreFraction:     yandex.CoreFraction{{$config.CoreFraction}},
			VCPU:             []int{ {{range $i, $cpu := $config.VCPU}}{{if $i}}, {{end}}{{$cpu}}{{end}} },
			MemoryPerCore:    []float64{ {{range $i, $mem := $config.MemoryPerCore}}{{if $i}}, {{end}}{{printf "%.2f" $mem}}{{end}} },
			CanBePreemptible: {{$config.CanBePreemptible}},
		},
{{end}}	},
{{end}}}
`

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run config_gen.go <region>")
	}

	region := os.Args[1]
	if region != "ru" && region != "kz" {
		log.Fatalf("Unsupported region: %s. Supported regions: ru, kz", region)
	}

	config, err := fetchConfigFromAPI(region)
	if err != nil {
		log.Fatalf("Failed to fetch config: %v", err)
	}

	if err := generateConfigFile(config); err != nil {
		log.Fatalf("Failed to generate config file: %v", err)
	}

	fmt.Printf("Successfully generated %s.configuration.go\n", region)
}

func fetchConfigFromAPI(region string) (*RegionConfig, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	params := url.Values{}
	params.Add("installationCode", region)
	params.Add("lang", region)

	apiURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())
	fmt.Printf("Fetching config from: %s\n", apiURL)

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		apiURL,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var configResponse ConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&configResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	config := &RegionConfig{
		Region:         region,
		Configurations: make(map[yandex.PlatformId][]InstanceConfiguration),
	}

	fmt.Printf("Processing %d platforms\n", len(configResponse.Platforms))

	for _, platform := range configResponse.Platforms {
		processPlatform(platform, config)
	}

	fmt.Printf("Found configurations for %d platforms\n", len(config.Configurations))

	return config, nil
}

func processPlatform(platform Platform, config *RegionConfig) {
	platformID, exists := platformMapping[platform.ID]
	if !exists {
		fmt.Printf("Unknown platform: %s\n", platform.ID)
		return
	}

	fmt.Printf("Processing platform: %s (%s)\n", platform.Name, platform.ID)

	var configurations []InstanceConfiguration

	for _, allowedConfig := range platform.AllowedConfigurations {
		// Parse core fraction
		fraction, err := strconv.Atoi(allowedConfig.CoreFraction)
		if err != nil {
			fmt.Printf("Invalid core fraction '%s' for platform %s\n", allowedConfig.CoreFraction, platform.ID)
			continue
		}

		var coreFraction yandex.CoreFraction
		switch fraction {
		case 5:
			coreFraction = yandex.CoreFraction5
		case 20:
			coreFraction = yandex.CoreFraction20
		case 50:
			coreFraction = yandex.CoreFraction50
		case 100:
			coreFraction = yandex.CoreFraction100
		default:
			fmt.Printf("Unsupported core fraction %d for platform %s\n", fraction, platform.ID)
			continue
		}

		// Collect all available cores
		var vcpus []int

		// Handle different core formats
		switch cores := allowedConfig.Cores.(type) {
		case []interface{}:
			for _, coreItem := range cores {
				switch coreConfig := coreItem.(type) {
				case map[string]interface{}:
					// Handle CoreConfig format
					if coresList, ok := coreConfig["cores"].([]interface{}); ok {
						for _, coreStr := range coresList {
							if coreStrVal, ok := coreStr.(string); ok {
								core, err := strconv.Atoi(coreStrVal)
								if err != nil {
									fmt.Printf("Invalid core value '%s' for platform %s\n", coreStrVal, platform.ID)
									continue
								}
								vcpus = append(vcpus, core)
							}
						}
					}
				case string:
					// Handle string format
					core, err := strconv.Atoi(coreConfig)
					if err != nil {
						fmt.Printf("Invalid core value '%s' for platform %s\n", coreConfig, platform.ID)
						continue
					}
					vcpus = append(vcpus, core)
				}
			}
		case []string:
			// Handle direct string array
			for _, coreStr := range cores {
				core, err := strconv.Atoi(coreStr)
				if err != nil {
					fmt.Printf("Invalid core value '%s' for platform %s\n", coreStr, platform.ID)
					continue
				}
				vcpus = append(vcpus, core)
			}
		default:
			fmt.Printf("Unknown cores format for platform %s: %T\n", platform.ID, allowedConfig.Cores)
			continue
		}

		// Remove duplicates and sort
		vcpus = removeDuplicatesInt(vcpus)
		sort.Ints(vcpus)

		// Parse memory per core (in bytes, convert to GB)
		var memoryPerCore []float64
		for _, memStr := range allowedConfig.MemoryPerCore {
			memBytes, err := strconv.ParseInt(memStr, 10, 64)
			if err != nil {
				fmt.Printf("Invalid memory value '%s' for platform %s\n", memStr, platform.ID)
				continue
			}
			memGB := float64(memBytes) / (1024 * 1024 * 1024) // Convert bytes to GB
			memoryPerCore = append(memoryPerCore, memGB)
		}

		// Remove duplicates and sort
		memoryPerCore = removeDuplicatesFloat(memoryPerCore)
		sort.Float64s(memoryPerCore)

		if len(vcpus) > 0 && len(memoryPerCore) > 0 {
			configurations = append(configurations, InstanceConfiguration{
				CoreFraction:     coreFraction,
				VCPU:             vcpus,
				MemoryPerCore:    memoryPerCore,
				CanBePreemptible: !platform.RejectPreemptible,
			})
		}
	}

	if len(configurations) > 0 {
		config.Configurations[platformID] = configurations
	}
}

func removeDuplicatesInt(slice []int) []int {
	seen := make(map[int]bool)
	var result []int
	for _, v := range slice {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

func removeDuplicatesFloat(slice []float64) []float64 {
	seen := make(map[float64]bool)
	var result []float64
	for _, v := range slice {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

func generateConfigFile(config *RegionConfig) error {
	filename := fmt.Sprintf("%s.configuration.go", config.Region)

	tmpl, err := template.New("config").Parse(configTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	data := struct {
		Timestamp      string
		Region         string
		Configurations map[string][]struct {
			CoreFraction     int
			VCPU             []int
			MemoryPerCore    []float64
			CanBePreemptible bool
		}
	}{
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Region:    config.Region,
		Configurations: make(map[string][]struct {
			CoreFraction     int
			VCPU             []int
			MemoryPerCore    []float64
			CanBePreemptible bool
		}),
	}

	// Sort platform names for consistent output
	platformNames := make([]string, 0, len(config.Configurations))
	for platformID := range config.Configurations {
		platformNames = append(platformNames, string(platformID))
	}
	sort.Strings(platformNames)

	for _, platformName := range platformNames {
		platformID := yandex.PlatformId(platformName)
		configurations := config.Configurations[platformID]

		var convertedConfigs []struct {
			CoreFraction     int
			VCPU             []int
			MemoryPerCore    []float64
			CanBePreemptible bool
		}

		for _, config := range configurations {
			convertedConfigs = append(convertedConfigs, struct {
				CoreFraction     int
				VCPU             []int
				MemoryPerCore    []float64
				CanBePreemptible bool
			}{
				CoreFraction:     int(config.CoreFraction),
				VCPU:             config.VCPU,
				MemoryPerCore:    config.MemoryPerCore,
				CanBePreemptible: config.CanBePreemptible,
			})
		}

		data.Configurations[getConstantName(platformID)] = convertedConfigs
	}

	return tmpl.Execute(file, data)
}

func getConstantName(platformID yandex.PlatformId) string {
	switch platformID {
	case yandex.PlatformIntelBroadwell:
		return "PlatformIntelBroadwell"
	case yandex.PlatformIntelCascadeLake:
		return "PlatformIntelCascadeLake"
	case yandex.PlatformIntelIceLake:
		return "PlatformIntelIceLake"
	case yandex.PlatformAMDZen3:
		return "PlatformAMDZen3"
	case yandex.PlatformAMDZen4:
		return "PlatformAMDZen4"
	case yandex.PlatformIntelIceLakeComputeOptimized:
		return "PlatformIntelIceLakeComputeOptimized"
	case yandex.PlatformAmdZen4ComputeOptimized:
		return "PlatformAmdZen4ComputeOptimized"
	case yandex.PlatformIntelBroadwellNVIDIATeslaV100:
		return "PlatformIntelBroadwellNVIDIATeslaV100"
	case yandex.PlatformIntelCascadeLakeNVIDIATeslaV100:
		return "PlatformIntelCascadeLakeNVIDIATeslaV100"
	case yandex.PlatformAMDEPYCNVIDIAAmpereA100:
		return "PlatformAMDEPYCNVIDIAAmpereA100"
	case yandex.PlatformAMDEPYC9474FGen2:
		return "PlatformAMDEPYC9474FGen2"
	case yandex.PlatformIntelIceLakeNVIDIATeslaT4:
		return "PlatformIntelIceLakeNVIDIATeslaT4"
	case yandex.PlatformIntelIceLakeNVIDIATeslaT4i:
		return "PlatformIntelIceLakeNVIDIATeslaT4i"
	default:
		return string(platformID)
	}
}
