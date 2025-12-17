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
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs/sysfs"
)

type gpuCollector struct {
	fs          sysfs.FS
	logger      *slog.Logger
	pciProvider *pciIDProvider
}

func init() {
	registerCollector("gpu", defaultEnabled, NewGPUCollector)
}

// NewGPUCollector returns a new Collector exposing GPU stats.
func NewGPUCollector(logger *slog.Logger) (Collector, error) {
	fs, err := sysfs.NewFS(*sysPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sysfs: %w", err)
	}

	c := &gpuCollector{
		fs:     fs,
		logger: logger,
	}

	// Initialize pciProvider for name resolution if pci.ids file is available
	// We reuse the same flags as pcidevice for consistency
	if *pciIdsFile != "" || len(pciIdsPaths) > 0 {
		c.pciProvider = newPCIIDProvider(logger, pciIdsPaths, *pciIdsFile)
	}

	return c, nil
}

func (c *gpuCollector) Update(ch chan<- prometheus.Metric) error {
	devices, err := c.fs.PciDevices()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.logger.Debug("PCI device not found, skipping")
			return ErrNoData
		}
		return fmt.Errorf("error obtaining PCI device info: %w", err)
	}

	gpuCount := 0
	for _, device := range devices {
		// Class 0x03 is Display Controller (VGA, 3D, etc.)
		if device.Class>>16 != 0x03 {
			continue
		}

		gpuCount++

		vendorID := fmt.Sprintf("0x%04x", device.Vendor)
		deviceID := fmt.Sprintf("0x%04x", device.Device)
		busID := device.Location.String()

		var vendorName, deviceName string
		if c.pciProvider != nil {
			vendorName = c.pciProvider.getVendorName(vendorID)
			deviceName = c.pciProvider.getDeviceName(vendorID, deviceID)
		} else {
			vendorName = vendorID
			deviceName = deviceID
		}

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, "gpu", "info"),
				"Information about the GPU.",
				[]string{"gpu_id", "vendor", "model", "vendor_id", "device_id"}, nil,
			),
			prometheus.GaugeValue,
			1,
			busID, vendorName, deviceName, vendorID, deviceID,
		)
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

	return nil
}
