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

type gpuCollector struct {
	logger   *slog.Logger
	resolver gpuInfoResolver
}

func init() {
	registerCollector("gpu", defaultEnabled, NewGPUCollector)
}

// NewGPUCollector returns a new Collector exposing GPU stats.
func NewGPUCollector(logger *slog.Logger) (Collector, error) {
	return &gpuCollector{
		logger: logger,
		resolver: gpuInfoResolver{
			pciProvider: newPCIIDProvider(logger, pciIdsPaths, *pciIdsFile),
		},
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

// isGPUDriverLoaded checks if a native GPU or passthrough driver is bound to the device.
func isGPUDriverLoaded(devicePath string) bool {
	driverLink := filepath.Join(devicePath, "driver")
	target, err := os.Readlink(driverLink)
	if err != nil {
		return false
	}
	driverName := filepath.Base(target)
	// Valid GPU drivers: native drivers + vfio-pci for passthrough
	validDrivers := []string{"nvidia", "nouveau", "amdgpu", "radeon", "i915", "xe", "vfio-pci"}
	for _, d := range validDrivers {
		if driverName == d {
			return true
		}
	}
	return false
}

func (c *gpuCollector) Update(ch chan<- prometheus.Metric) error {
	sysfsPath := sysFilePath("bus/pci/devices")

	entries, err := os.ReadDir(sysfsPath)
	if err != nil {
		c.logger.Debug("Failed to read PCI devices", "error", err)
		return ErrNoData
	}

	var gpuMetrics []prometheus.Metric
	modelCounts := make(map[string]int) // Track count per model

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

		busID := entry.Name()
		productName := c.resolver.productName(vendorID, deviceID)

		// Track model count
		modelCounts[productName]++

		vendorName := c.resolver.vendorName(vendorID)

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
	if len(modelCounts) > 0 {
		for _, m := range gpuMetrics {
			ch <- m
		}

		// Emit cards_total per model
		for model, count := range modelCounts {
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "gpu", "cards_total"),
					"Total number of GPU cards detected.",
					[]string{"model"}, nil,
				),
				prometheus.GaugeValue,
				float64(count),
				model,
			)
		}
	}

	return nil
}
