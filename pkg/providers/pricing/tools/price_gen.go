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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/tufitko/karpenter-provider-yandex/pkg/yandex"
)

type PlatformPricing struct {
	PlatformID             yandex.PlatformId
	PerFraction            map[yandex.CoreFraction]float64
	PreemptiblePerFraction map[yandex.CoreFraction]float64
	RAM                    float64
	PreemptibleRAM         float64
}

type DiskPricing struct {
	SSD              float64
	HDD              float64
	SSDNonreplicated float64
	SSDIo            float64
}

type RegionPricing struct {
	Region    string
	Currency  string
	Platforms map[yandex.PlatformId]PlatformPricing
	Disks     DiskPricing
}

const (
	// thx for a1k0u and moleus for api
	baseURL      = "https://yandex.cloud/api/priceList/getPriceList"
	computeCloud = "dn22pas77ftg9h3f2djj"
)

type PriceResponse struct {
	SKUs          []SKU  `json:"skus"`
	NextPageToken string `json:"nextPageToken"`
}

type SKU struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	PricingUnit     string           `json:"pricingUnit"`
	ServiceID       string           `json:"serviceId"`
	UsageType       string           `json:"usageType"`
	Deprecated      bool             `json:"deprecated"`
	CreatedAt       int64            `json:"createdAt"`
	PricingVersions []PricingVersion `json:"pricingVersions"`
	EffectiveTime   int64            `json:"effectiveTime"`
}

type PricingVersion struct {
	ID                string            `json:"id"`
	PricingExpression PricingExpression `json:"pricingExpression"`
	EffectiveTime     int64             `json:"effectiveTime"`
}

type PricingExpression struct {
	Quantum string `json:"quantum"`
	Rates   []Rate `json:"rates"`
}

type Rate struct {
	StartPricingQuantity string `json:"startPricingQuantity"`
	UnitPrice            string `json:"unitPrice"`
}

var platformMapping = map[string]yandex.PlatformId{
	"broadwell":        yandex.PlatformIntelBroadwell,
	"cascade":          yandex.PlatformIntelCascadeLake,
	"ice":              yandex.PlatformIntelIceLake,
	"amd":              yandex.PlatformAMDZen3,
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

var fractionMapping = map[string]yandex.CoreFraction{
	"5":   yandex.CoreFraction5,
	"20":  yandex.CoreFraction20,
	"50":  yandex.CoreFraction50,
	"100": yandex.CoreFraction100,
}

const pricingTemplate = `/*
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

// Generated on {{.Timestamp}} by price_gen tool
package pricing

import "github.com/tufitko/karpenter-provider-yandex/pkg/yandex"

var {{.Region}}Pricing = map[yandex.PlatformId]pricingPlatform{
{{range $platformId, $platform := .Platforms}}	yandex.{{$platformId}}: {
		perFraction: map[yandex.CoreFraction]float64{
{{range $fraction, $price := $platform.PerFraction}}			yandex.CoreFraction{{$fraction}}: {{printf "%.4f" $price}},
{{end}}		},
		preemptiblePerFraction: map[yandex.CoreFraction]float64{
{{range $fraction, $price := $platform.PreemptiblePerFraction}}			yandex.CoreFraction{{$fraction}}: {{printf "%.4f" $price}},
{{end}}		},
		ram:            {{printf "%.4f" $platform.RAM}},
		preemptibleRAM: {{printf "%.4f" $platform.PreemptibleRAM}},
	},
{{end}}}

// Per hour for 1GB of disk storage
var {{.Region}}DiskPricing = map[yandex.DiskType]float64{
{{if .Disks.SSD}}	yandex.SSD: {{printf "%.4f" .Disks.SSD}},
{{end}}{{if .Disks.HDD}}	yandex.HDD: {{printf "%.4f" .Disks.HDD}},
{{end}}{{if .Disks.SSDNonreplicated}}	yandex.SSDNonreplicated: {{printf "%.4f" .Disks.SSDNonreplicated}},
{{end}}{{if .Disks.SSDIo}}	yandex.SSDIo: {{printf "%.4f" .Disks.SSDIo}},
{{end}}}
`

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run price_gen.go <region>")
	}

	region := os.Args[1]
	if region != "ru" && region != "kz" {
		log.Fatalf("Unsupported region: %s. Supported regions: ru, kz", region)
	}

	pricing, err := fetchPricingFromAPI(region)
	if err != nil {
		log.Fatalf("Failed to fetch pricing: %v", err)
	}

	if err := generatePricingFile(pricing); err != nil {
		log.Fatalf("Failed to generate pricing file: %v", err)
	}

	fmt.Printf("Successfully generated %s.pricing.go\n", region)
}

func fetchPricingFromAPI(region string) (*RegionPricing, error) {
	currency := getCurrency(region)
	installationCode := region

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	pricing := &RegionPricing{
		Region:    region,
		Currency:  currency,
		Platforms: make(map[yandex.PlatformId]PlatformPricing),
		Disks:     DiskPricing{},
	}

	var nextPageToken string
	totalSKUs := 0

	for {
		params := url.Values{}
		params.Add("installationCode", installationCode)
		params.Add("services[]", computeCloud)
		params.Add("from", time.Now().Format("2006-01-02"))
		params.Add("to", time.Now().Format("2006-01-02"))
		params.Add("currency", currency)
		params.Add("lang", installationCode)

		if nextPageToken != "" {
			params.Add("pageToken", nextPageToken)
		}

		apiURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())
		fmt.Printf("Fetching pricing page (token: %s)\n", nextPageToken)

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

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
		}

		var priceResponse PriceResponse
		if err := json.NewDecoder(resp.Body).Decode(&priceResponse); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		resp.Body.Close()

		fmt.Printf("Processing %d SKUs from current page\n", len(priceResponse.SKUs))
		totalSKUs += len(priceResponse.SKUs)

		for _, sku := range priceResponse.SKUs {
			if sku.Deprecated {
				continue
			}
			// todo: support reservation
			if strings.Contains(sku.Name, "резервирование") ||
				strings.Contains(sku.Name, "Программно ускоренная сеть") ||
				strings.Contains(sku.Name, "Самостоятельная покупка") ||
				strings.Contains(sku.Name, "Выделенный хост") {
				continue
			}

			if processDiskSKU(sku, pricing) {
				continue
			}

			processSKU(sku, pricing)
		}

		nextPageToken = priceResponse.NextPageToken
		if nextPageToken == "" {
			break
		}
	}

	fmt.Printf("Processed total %d SKUs, found pricing for %d platforms\n", totalSKUs, len(pricing.Platforms))

	return pricing, nil
}

func getCurrency(region string) string {
	switch region {
	case "ru":
		return "RUB"
	case "kz":
		return "KZT"
	default:
		return "USD"
	}
}

func processSKU(sku SKU, pricing *RegionPricing) {
	fmt.Println("Processing SKU", sku.Name)
	if len(sku.PricingVersions) == 0 {
		return
	}

	latestVersion := sku.PricingVersions[0]
	if len(latestVersion.PricingExpression.Rates) == 0 {
		return
	}

	unitPrice := latestVersion.PricingExpression.Rates[0].UnitPrice
	price, err := strconv.ParseFloat(unitPrice, 64)
	if err != nil {
		fmt.Printf("Failed to parse price %s for SKU %s: %v\n", unitPrice, sku.Name, err)
		return
	}

	platformID := findPlatformFromSKU(sku)
	if platformID == yandex.PlatformUnknown {
		fmt.Printf("Unknown platform for SKU: %s\n", sku.Name)
		return
	}

	if _, exists := pricing.Platforms[platformID]; !exists {
		pricing.Platforms[platformID] = PlatformPricing{
			PlatformID:             platformID,
			PerFraction:            make(map[yandex.CoreFraction]float64),
			PreemptiblePerFraction: make(map[yandex.CoreFraction]float64),
		}
	}

	platform := pricing.Platforms[platformID]

	switch sku.PricingUnit {
	case "core*hour":
		fraction := extractFractionFromSKU(sku)
		if fraction == 0 {
			fraction = yandex.CoreFraction100
		}

		if isPreemptible(sku) {
			platform.PreemptiblePerFraction[fraction] = price
		} else {
			platform.PerFraction[fraction] = price
		}

	case "gbyte*hour":
		if isPreemptible(sku) {
			platform.PreemptibleRAM = price
		} else {
			platform.RAM = price
		}
	}

	pricing.Platforms[platformID] = platform
}

func findPlatformFromSKU(sku SKU) yandex.PlatformId {
	name := strings.ToLower(sku.Name)

	if strings.Contains(name, "broadwell") {
		if strings.Contains(name, "tesla") || strings.Contains(name, "v100") {
			return yandex.PlatformIntelBroadwellNVIDIATeslaV100
		}
		return yandex.PlatformIntelBroadwell
	}

	if strings.Contains(name, "cascade") {
		if strings.Contains(name, "tesla") || strings.Contains(name, "v100") {
			return yandex.PlatformIntelCascadeLakeNVIDIATeslaV100
		}
		return yandex.PlatformIntelCascadeLake
	}

	if strings.Contains(name, "ice") {
		if strings.Contains(name, "tesla") && strings.Contains(name, "t4") {
			if strings.Contains(name, "t4i") {
				return yandex.PlatformIntelIceLakeNVIDIATeslaT4i
			}
			return yandex.PlatformIntelIceLakeNVIDIATeslaT4
		}
		if strings.Contains(name, "compute") || strings.Contains(name, "highfreq") {
			return yandex.PlatformIntelIceLakeComputeOptimized
		}
		return yandex.PlatformIntelIceLake
	}

	if strings.Contains(name, "amd") || strings.Contains(name, "epyc") {
		if strings.Contains(name, "9474f") || strings.Contains(name, "gen2") {
			return yandex.PlatformAMDEPYC9474FGen2
		}
		if strings.Contains(name, "ampere") || strings.Contains(name, "a100") {
			return yandex.PlatformAMDEPYCNVIDIAAmpereA100
		}
		if strings.Contains(name, "compute") || strings.Contains(name, "highfreq") {
			return yandex.PlatformAmdZen4ComputeOptimized
		}
		if strings.Contains(name, "standard-v4a") {
			return yandex.PlatformAMDZen4
		}
		return yandex.PlatformAMDZen3
	}

	return yandex.PlatformUnknown
}

func extractFractionFromSKU(sku SKU) yandex.CoreFraction {
	name := strings.ToLower(sku.Name)

	if strings.Contains(name, "5%") {
		return yandex.CoreFraction5
	}
	if strings.Contains(name, "20%") {
		return yandex.CoreFraction20
	}
	if strings.Contains(name, "50%") {
		return yandex.CoreFraction50
	}
	if strings.Contains(name, "100%") {
		return yandex.CoreFraction100
	}

	re := regexp.MustCompile(`(\d+)%`)
	matches := re.FindStringSubmatch(name)
	if len(matches) > 1 {
		if frac, exists := fractionMapping[matches[1]]; exists {
			return frac
		}
	}

	return yandex.CoreFraction100
}

func isPreemptible(sku SKU) bool {
	name := strings.ToLower(sku.Name)
	return strings.Contains(name, "preemptible") || strings.Contains(name, "прерываем")
}

// processDiskSKU processes disk-related SKUs and returns true if the SKU was a disk
func processDiskSKU(sku SKU, pricing *RegionPricing) bool {
	nameLocal := strings.ToLower(sku.Name)

	if strings.Contains(nameLocal, "образ") || strings.Contains(nameLocal, "снимок") {
		return false
	}

	// Check if this is a disk SKU by pricingUnit or name
	isDisk := sku.PricingUnit == "gbyte*hour" && (strings.Contains(nameLocal, "хранилище") ||
		strings.Contains(nameLocal, "файловая система") ||
		strings.Contains(nameLocal, "hdd") ||
		strings.Contains(nameLocal, "ssd") ||
		strings.Contains(nameLocal, "disk") ||
		strings.Contains(nameLocal, "storage"))

	if !isDisk {
		return false
	}

	if len(sku.PricingVersions) == 0 {
		return true
	}

	latestVersion := sku.PricingVersions[0]
	if len(latestVersion.PricingExpression.Rates) == 0 {
		return true
	}

	unitPrice := latestVersion.PricingExpression.Rates[0].UnitPrice
	price, err := strconv.ParseFloat(unitPrice, 64)
	if err != nil {
		fmt.Printf("Failed to parse disk price %s for SKU %s: %v\n", unitPrice, sku.Name, err)
		return true
	}

	//  SSDIO
	if strings.Contains(nameLocal, "сверхбыстрое") && strings.Contains(nameLocal, "3 репликами") {
		pricing.Disks.SSDIo = price
		fmt.Printf("Found SSD IO price: %.4f RUB/hour (from SKU: %s)\n", price, sku.Name)
		return true
	}

	//  SSDNonreplicated
	if strings.Contains(nameLocal, "нереплицируемое") ||
		strings.Contains(nameLocal, "non-replicated") ||
		strings.Contains(nameLocal, "nonreplicated") {
		pricing.Disks.SSDNonreplicated = price
		fmt.Printf("Found SSD Non-replicated price: %.4f RUB/hour (from SKU: %s)\n", price, sku.Name)
		return true
	}

	//  SSD
	if (strings.Contains(nameLocal, "быстрое") || strings.Contains(nameLocal, "быстрая")) &&
		strings.Contains(nameLocal, "ssd") &&
		!strings.Contains(nameLocal, "сверхбыстрое") &&
		!strings.Contains(nameLocal, "нереплицируемое") {
		pricing.Disks.SSD = price
		fmt.Printf("Found SSD price: %.4f RUB/hour (from SKU: %s)\n", price, sku.Name)
		return true
	}

	//  HDD
	if (strings.Contains(nameLocal, "стандартное") || strings.Contains(nameLocal, "стандартная")) &&
		strings.Contains(nameLocal, "hdd") {
		pricing.Disks.HDD = price
		fmt.Printf("Found HDD price: %.4f RUB/hour (from SKU: %s)\n", price, sku.Name)
		return true
	}

	fmt.Printf("Unknown disk type for SKU: %s (name: %s, pricingUnit: %s)\n", sku.Name, nameLocal, sku.PricingUnit)
	return true
}

func generatePricingFile(pricing *RegionPricing) error {
	filename := fmt.Sprintf("%s.pricing.go", pricing.Region)

	tmpl, err := template.New("pricing").Parse(pricingTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	data := struct {
		Timestamp string
		Region    string
		Platforms map[string]struct {
			PerFraction            map[int]float64
			PreemptiblePerFraction map[int]float64
			RAM                    float64
			PreemptibleRAM         float64
		}
		Disks DiskPricing
	}{
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Region:    pricing.Region,
		Platforms: make(map[string]struct {
			PerFraction            map[int]float64
			PreemptiblePerFraction map[int]float64
			RAM                    float64
			PreemptibleRAM         float64
		}),
		Disks: pricing.Disks,
	}

	platformNames := make([]string, 0, len(pricing.Platforms))
	for platformID := range pricing.Platforms {
		platformNames = append(platformNames, string(platformID))
	}
	sort.Strings(platformNames)

	for _, platformName := range platformNames {
		platformID := yandex.PlatformId(platformName)
		platform := pricing.Platforms[platformID]

		convertedPlatform := struct {
			PerFraction            map[int]float64
			PreemptiblePerFraction map[int]float64
			RAM                    float64
			PreemptibleRAM         float64
		}{
			PerFraction:            make(map[int]float64),
			PreemptiblePerFraction: make(map[int]float64),
			RAM:                    platform.RAM,
			PreemptibleRAM:         platform.PreemptibleRAM,
		}

		for fraction, price := range platform.PerFraction {
			convertedPlatform.PerFraction[int(fraction)] = price
		}

		for fraction, price := range platform.PreemptiblePerFraction {
			convertedPlatform.PreemptiblePerFraction[int(fraction)] = price
		}

		data.Platforms[getConstantName(platformID)] = convertedPlatform
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
		return "PlatformIntelIceLakeNVIDIaTeslaT4i"
	default:
		return string(platformID)
	}
}
