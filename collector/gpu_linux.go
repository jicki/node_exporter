// Copyright 2024 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build linux

package collector

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// GPU vendor IDs (whitelist)
const (
	vendorNVIDIA = "0x10de"
	vendorAMD    = "0x1002"
	vendorIntel  = "0x8086"
)

// Known BMC/Management graphics (blacklist)
var bmcVendors = map[string]bool{
	"0x1a03": true, // ASPEED
	"0x102b": true, // Matrox
}

// NVIDIA device ID to product name mapping (common GPUs)
var nvidiaProducts = map[string]string{
	// Data Center - Tesla
	"0x1eb8": "Tesla T4",
	"0x1db4": "Tesla V100-PCIE-16GB",
	"0x1db5": "Tesla V100-PCIE-32GB",
	"0x1db6": "Tesla V100-SXM2-16GB",
	"0x1db7": "Tesla V100-SXM2-32GB",
	"0x1df6": "Tesla V100S-PCIE-32GB",
	"0x1e30": "Tesla T4",
	"0x1e37": "Tesla T4",

	// Data Center - Ampere (A-series)
	"0x20b0": "A100-PCIE-40GB",
	"0x20b2": "A100-SXM4-40GB",
	"0x20b5": "A100-PCIE-80GB",
	"0x20b7": "A30",
	"0x20f1": "A100-SXM4-80GB",
	"0x20f3": "A800-SXM4-80GB",
	"0x20f5": "A800-PCIE-80GB",
	"0x2236": "A10",
	"0x2237": "A10G",
	"0x25b6": "A16",

	// Data Center - Hopper (H-series)
	"0x2330": "H100-PCIE",
	"0x2331": "H100-SXM5-80GB",
	"0x2339": "H100-NVL",
	"0x233a": "H800-PCIE",
	"0x233d": "H800-SXM5",

	// Data Center - Ada Lovelace (L-series)
	"0x26b5": "L40",
	"0x26b9": "L40S",
	"0x26ba": "L20",
	"0x27b0": "L4",
	"0x27b6": "L2",

	// GeForce RTX 50 Series (Blackwell)
	"0x2b85": "GeForce RTX 5090", // Verified
	"0x2c02": "GeForce RTX 5080", // Verified
	"0x2980": "GeForce RTX 5090",
	"0x29c0": "GeForce RTX 5090",
	"0x2c18": "GeForce RTX 5090 Laptop",
	"0x2c58": "GeForce RTX 5090 Laptop",
	"0x2c19": "GeForce RTX 5080 Laptop",
	"0x2c2c": "GeForce RTX 5080",
	"0x2c59": "GeForce RTX 5080 Laptop",

	// GeForce RTX 40 Series (Ada Lovelace)
	"0x2684": "GeForce RTX 4090",
	"0x2685": "GeForce RTX 4090 D",
	"0x2702": "GeForce RTX 4080 SUPER",
	"0x2704": "GeForce RTX 4080",
	"0x2705": "GeForce RTX 4070 Ti SUPER",
	"0x2709": "GeForce RTX 4070",
	"0x2782": "GeForce RTX 4070 Ti",
	"0x2783": "GeForce RTX 4070 SUPER",
	"0x2786": "GeForce RTX 4060",
	"0x2803": "GeForce RTX 4060 Ti",
	"0x2882": "GeForce RTX 4060 Laptop",
	"0x28a0": "GeForce RTX 4080 Laptop",
	"0x28a1": "GeForce RTX 4090 Laptop",

	// GeForce RTX 30 Series (Ampere)
	"0x2204": "GeForce RTX 3090",
	"0x2205": "GeForce RTX 3090 Ti",
	"0x2206": "GeForce RTX 3080",
	"0x2207": "GeForce RTX 3080 LHR",
	"0x2208": "GeForce RTX 3080 Ti",
	"0x220a": "GeForce RTX 3080 12GB",
	"0x2216": "GeForce RTX 3080 Laptop",
	"0x2230": "GeForce RTX 3080 Ti Laptop",
	"0x2414": "GeForce RTX 3060 Ti",
	"0x2420": "GeForce RTX 3080 Laptop",
	"0x2482": "GeForce RTX 3070 Ti",
	"0x2484": "GeForce RTX 3070",
	"0x2486": "GeForce RTX 3060 Ti LHR",
	"0x2488": "GeForce RTX 3070 LHR",
	"0x2503": "GeForce RTX 3060",
	"0x2504": "GeForce RTX 3060 LHR",
	"0x2507": "GeForce RTX 3050",
	"0x2520": "GeForce RTX 3060 Laptop",
	"0x2560": "GeForce RTX 3060 Laptop",
	"0x2563": "GeForce RTX 3050 Ti Laptop",

	// GeForce RTX 20 Series (Turing)
	"0x1e04": "GeForce RTX 2080 Ti",
	"0x1e07": "GeForce RTX 2080 Ti",
	"0x1e82": "GeForce RTX 2080",
	"0x1e84": "GeForce RTX 2080 SUPER",
	"0x1e87": "GeForce RTX 2080",
	"0x1e89": "GeForce RTX 2060",
	"0x1f02": "GeForce RTX 2070",
	"0x1f06": "GeForce RTX 2060 SUPER",
	"0x1f07": "GeForce RTX 2070",
	"0x1f08": "GeForce RTX 2060",
	"0x1f10": "GeForce RTX 2070 Laptop",
	"0x1f11": "GeForce RTX 2060 Laptop",
	"0x1f42": "GeForce RTX 2060",
	"0x1f47": "GeForce RTX 2060 SUPER",

	// GeForce GTX 16 Series (Turing)
	"0x1f82": "GeForce GTX 1650",
	"0x1f91": "GeForce GTX 1650 Laptop",
	"0x1f92": "GeForce GTX 1650 Laptop",
	"0x2182": "GeForce GTX 1660 Ti",
	"0x2184": "GeForce GTX 1660",
	"0x2187": "GeForce GTX 1650 SUPER",
	"0x2188": "GeForce GTX 1650",
	"0x21c4": "GeForce GTX 1660 SUPER",

	// Quadro/Professional
	"0x1e36": "Quadro RTX 6000",
	"0x1e78": "Quadro RTX 6000",
	"0x1eb0": "Quadro RTX 5000",
	"0x1eb1": "Quadro RTX 4000",
	"0x1fb0": "Quadro T1000",
	"0x1fb8": "Quadro T2000",
	"0x2231": "RTX A6000",
	"0x2232": "RTX A5000",
	"0x2233": "RTX A5500",
	"0x2235": "RTX A40",
	"0x2571": "RTX A2000",
	"0x25a2": "RTX A5000 Laptop",
	"0x25a5": "RTX A4000 Laptop",
	"0x25b8": "RTX A4000",
	"0x26b1": "RTX 6000 Ada",
	"0x26b2": "RTX 5000 Ada",
}

type gpuCollector struct {
	logger *slog.Logger
}

func init() {
	registerCollector("gpu", defaultEnabled, NewGPUCollector)
}

// NewGPUCollector returns a new Collector exposing GPU stats.
func NewGPUCollector(logger *slog.Logger) (Collector, error) {
	return &gpuCollector{
		logger: logger,
	}, nil
}

// readSysfsFile reads a file from sysfs and returns trimmed content
func readSysfsFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// isGPUDriverLoaded checks if a GPU driver (not vfio) is bound to the device
func isGPUDriverLoaded(devicePath string) bool {
	driverLink := filepath.Join(devicePath, "driver")
	target, err := os.Readlink(driverLink)
	if err != nil {
		return false
	}
	driverName := filepath.Base(target)
	// nvidia, nouveau, amdgpu, i915 are valid GPU drivers
	// vfio-pci means passthrough, not usable by host
	validDrivers := []string{"nvidia", "nouveau", "amdgpu", "radeon", "i915", "xe"}
	for _, d := range validDrivers {
		if driverName == d {
			return true
		}
	}
	return false
}

// getProductName returns human-readable product name
func getProductName(vendorID, deviceID string) string {
	if vendorID == vendorNVIDIA {
		if name, ok := nvidiaProducts[deviceID]; ok {
			return name
		}
	}
	// Fallback to device ID
	return deviceID
}

func (c *gpuCollector) Update(ch chan<- prometheus.Metric) error {
	sysfsPath := sysFilePath("bus/pci/devices")

	entries, err := os.ReadDir(sysfsPath)
	if err != nil {
		c.logger.Debug("Failed to read PCI devices", "error", err)
		return ErrNoData
	}

	var gpuMetrics []prometheus.Metric
	gpuCount := 0

	for _, entry := range entries {
		devicePath := filepath.Join(sysfsPath, entry.Name())

		// Read class
		classStr, err := readSysfsFile(filepath.Join(devicePath, "class"))
		if err != nil {
			continue
		}
		// Class 0x03xxxx = Display controller
		if !strings.HasPrefix(classStr, "0x03") {
			continue
		}

		// Read vendor
		vendorID, err := readSysfsFile(filepath.Join(devicePath, "vendor"))
		if err != nil {
			continue
		}

		// Skip BMC vendors
		if bmcVendors[vendorID] {
			c.logger.Debug("Skipping BMC device", "vendor", vendorID, "device", entry.Name())
			continue
		}

		// Only allow known GPU vendors
		if vendorID != vendorNVIDIA && vendorID != vendorAMD && vendorID != vendorIntel {
			c.logger.Debug("Skipping unknown vendor", "vendor", vendorID, "device", entry.Name())
			continue
		}

		// Check if GPU driver is loaded
		if !isGPUDriverLoaded(devicePath) {
			c.logger.Debug("GPU driver not loaded", "device", entry.Name())
			continue
		}

		// Read device ID
		deviceID, err := readSysfsFile(filepath.Join(devicePath, "device"))
		if err != nil {
			continue
		}

		gpuCount++
		busID := entry.Name()
		productName := getProductName(vendorID, deviceID)

		var vendorName string
		switch vendorID {
		case vendorNVIDIA:
			vendorName = "NVIDIA Corporation"
		case vendorAMD:
			vendorName = "AMD/ATI"
		case vendorIntel:
			vendorName = "Intel Corporation"
		default:
			vendorName = vendorID
		}

		c.logger.Debug("Found GPU",
			"vendor", vendorName,
			"product", productName,
			"busID", busID)

		gpuMetrics = append(gpuMetrics, prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, "gpu", "info"),
				"Information about the GPU.",
				[]string{"gpu_id", "vendor", "model", "vendor_id", "device_id"}, nil,
			),
			prometheus.GaugeValue,
			1,
			busID, vendorName, productName, vendorID, deviceID,
		))
	}

	// Only expose metrics if GPUs with drivers are detected
	if gpuCount > 0 {
		for _, m := range gpuMetrics {
			ch <- m
		}
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, "gpu", "cards_total"),
				"Total number of GPU cards detected.",
				nil, nil,
			),
			prometheus.GaugeValue,
			float64(gpuCount),
		)
	}

	return nil
}
