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
	"fmt"
	"log/slog"
	"strings"

	"github.com/jaypipes/ghw"
	"github.com/prometheus/client_golang/prometheus"
)

// Known GPU vendor IDs (whitelist) - only these vendors are considered real GPUs
var gpuVendorWhitelist = map[string]bool{
	"10de": true, // NVIDIA Corporation
	"1002": true, // AMD/ATI
	"8086": true, // Intel Corporation (for discrete GPUs like Arc)
}

// Known BMC/Management graphics vendor IDs (blacklist for extra safety)
var bmcVendorBlacklist = map[string]bool{
	"1a03": true, // ASPEED Technology Inc.
	"102b": true, // Matrox Electronics Systems Ltd.
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

func (c *gpuCollector) Update(ch chan<- prometheus.Metric) error {
	// Use ghw with rootfs support
	gpu, err := ghw.GPU(ghw.WithChroot(*rootfsPath))
	if err != nil {
		c.logger.Debug("Failed to get GPU info", "error", err)
		return ErrNoData
	}

	gpuCount := 0
	var gpuMetrics []prometheus.Metric

	for _, card := range gpu.GraphicsCards {
		if card.DeviceInfo == nil {
			c.logger.Debug("Skipping GPU card with no device info",
				"address", card.Address,
				"index", card.Index)
			continue
		}

		// Get vendor ID (lowercase, no 0x prefix)
		vendorID := strings.ToLower(card.DeviceInfo.Vendor.ID)

		// Filter out BMC/Management graphics (blacklist)
		if bmcVendorBlacklist[vendorID] {
			c.logger.Debug("Skipping BMC graphics device",
				"vendor", card.DeviceInfo.Vendor.Name,
				"vendorID", vendorID,
				"address", card.Address)
			continue
		}

		// Only allow known GPU vendors (whitelist)
		if !gpuVendorWhitelist[vendorID] {
			c.logger.Debug("Skipping unknown vendor device",
				"vendor", card.DeviceInfo.Vendor.Name,
				"vendorID", vendorID,
				"address", card.Address)
			continue
		}

		gpuCount++

		// Extract information from ghw
		busID := card.Address
		deviceID := card.DeviceInfo.Product.ID
		vendorName := card.DeviceInfo.Vendor.Name
		productName := card.DeviceInfo.Product.Name

		// Fallback for empty names
		if vendorName == "" {
			vendorName = vendorID
		}
		if productName == "" {
			productName = deviceID
		}

		c.logger.Debug("Found GPU",
			"vendor", vendorName,
			"product", productName,
			"address", busID)

		gpuMetrics = append(gpuMetrics, prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, "gpu", "info"),
				"Information about the GPU.",
				[]string{"gpu_id", "vendor", "model", "vendor_id", "device_id"}, nil,
			),
			prometheus.GaugeValue,
			1,
			busID, vendorName, productName, fmt.Sprintf("0x%s", vendorID), fmt.Sprintf("0x%s", deviceID),
		))
	}

	// Only expose metrics if GPUs are detected
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
