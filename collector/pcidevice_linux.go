// Copyright 2017-2019 The Prometheus Authors
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

//go:build !nopcidevice

package collector

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs/sysfs"
)

const (
	pcideviceSubsystem = "pcidevice"
)

var (
	pciIdsPaths = []string{
		"/usr/share/misc/pci.ids",
		"/usr/share/hwdata/pci.ids",
	}
	pciIdsFile = kingpin.Flag("collector.pcidevice.idsfile", "Path to pci.ids file to use for PCI device identification.").String()
	pciNames   = kingpin.Flag("collector.pcidevice.names", "Enable PCI device name resolution (requires pci.ids file).").Default("false").Bool()

	pcideviceLabelNames = []string{"segment", "bus", "device", "function"}

	pcideviceMaxLinkTSDesc = typedDesc{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, pcideviceSubsystem, "max_link_transfers_per_second"),
			"Value of maximum link's transfers per second (T/s)",
			pcideviceLabelNames, nil,
		),
		valueType: prometheus.GaugeValue,
	}

	pcideviceMaxLinkWidthDesc = typedDesc{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, pcideviceSubsystem, "max_link_width"),
			"Value of maximum link's width (number of lanes)",
			pcideviceLabelNames, nil,
		),
		valueType: prometheus.GaugeValue,
	}

	pcideviceCurrentLinkTSDesc = typedDesc{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, pcideviceSubsystem, "current_link_transfers_per_second"),
			"Value of current link's transfers per second (T/s)",
			pcideviceLabelNames, nil,
		),
		valueType: prometheus.GaugeValue,
	}
	pcideviceCurrentLinkWidthDesc = typedDesc{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, pcideviceSubsystem, "current_link_width"),
			"Value of current link's width (number of lanes)",
			pcideviceLabelNames, nil,
		),
		valueType: prometheus.GaugeValue,
	}

	pcidevicePowerStateDesc = typedDesc{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, pcideviceSubsystem, "power_state"),
			"PCIe device power state, one of: D0, D1, D2, D3hot, D3cold, unknown or error.",
			append(pcideviceLabelNames, "state"), nil,
		),
		valueType: prometheus.GaugeValue,
	}

	pcideviceD3coldAllowedDesc = typedDesc{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, pcideviceSubsystem, "d3cold_allowed"),
			"Whether the PCIe device supports D3cold power state (0/1).",
			pcideviceLabelNames, nil,
		),
		valueType: prometheus.GaugeValue,
	}

	pcideviceSriovDriversAutoprobeDesc = typedDesc{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, pcideviceSubsystem, "sriov_drivers_autoprobe"),
			"Whether SR-IOV drivers autoprobe is enabled for the device (0/1).",
			pcideviceLabelNames, nil,
		),
		valueType: prometheus.GaugeValue,
	}

	pcideviceSriovNumvfsDesc = typedDesc{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, pcideviceSubsystem, "sriov_numvfs"),
			"Number of Virtual Functions (VFs) currently enabled for SR-IOV.",
			pcideviceLabelNames, nil,
		),
		valueType: prometheus.GaugeValue,
	}

	pcideviceSriovTotalvfsDesc = typedDesc{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, pcideviceSubsystem, "sriov_totalvfs"),
			"Total number of Virtual Functions (VFs) supported by the device.",
			pcideviceLabelNames, nil,
		),
		valueType: prometheus.GaugeValue,
	}

	pcideviceSriovVfTotalMsixDesc = typedDesc{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, pcideviceSubsystem, "sriov_vf_total_msix"),
			"Total number of MSI-X vectors for Virtual Functions.",
			pcideviceLabelNames, nil,
		),
		valueType: prometheus.GaugeValue,
	}

	pcideviceNumaNodeDesc = typedDesc{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, pcideviceSubsystem, "numa_node"),
			"NUMA node number for the PCI device. -1 indicates unknown or not available.",
			pcideviceLabelNames, nil,
		),
		valueType: prometheus.GaugeValue,
	}
)

type pcideviceCollector struct {
	fs          sysfs.FS
	infoDesc    typedDesc
	logger      *slog.Logger
	pciProvider *pciIDProvider
	pciNames    bool
}

func init() {
	registerCollector("pcidevice", defaultDisabled, NewPcideviceCollector)
}

// NewPcideviceCollector returns a new Collector exposing PCI devices stats.
func NewPcideviceCollector(logger *slog.Logger) (Collector, error) {
	fs, err := sysfs.NewFS(*sysPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sysfs: %w", err)
	}

	c := &pcideviceCollector{
		fs:       fs,
		logger:   logger,
		pciNames: *pciNames,
	}

	// Build label names based on whether name resolution is enabled
	labelNames := append(pcideviceLabelNames,
		[]string{"parent_segment", "parent_bus", "parent_device", "parent_function",
			"class_id", "vendor_id", "device_id", "subsystem_vendor_id", "subsystem_device_id", "revision"}...)

	if c.pciNames {
		c.pciProvider = newPCIIDProvider(logger, pciIdsPaths, *pciIdsFile)
		// Add name labels when name resolution is enabled
		labelNames = append(labelNames, "vendor_name", "device_name", "subsystem_vendor_name", "subsystem_device_name", "class_name")
	}

	c.infoDesc = typedDesc{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, pcideviceSubsystem, "info"),
			"Non-numeric data from /sys/bus/pci/devices/<location>, value is always 1.",
			labelNames,
			nil,
		),
		valueType: prometheus.GaugeValue,
	}

	return c, nil
}

func (c *pcideviceCollector) Update(ch chan<- prometheus.Metric) error {
	devices, err := c.fs.PciDevices()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.logger.Debug("PCI device not found, skipping")
			return ErrNoData
		}
		return fmt.Errorf("error obtaining PCI device info: %w", err)
	}

	for _, device := range devices {
		// The device location is represented in separated format.
		values := device.Location.Strings()
		if device.ParentLocation != nil {
			values = append(values, device.ParentLocation.Strings()...)
		} else {
			values = append(values, []string{"*", "*", "*", "*"}...)
		}

		// Add basic device information
		classID := fmt.Sprintf("0x%06x", device.Class)
		vendorID := fmt.Sprintf("0x%04x", device.Vendor)
		deviceID := fmt.Sprintf("0x%04x", device.Device)
		subsysVendorID := fmt.Sprintf("0x%04x", device.SubsystemVendor)
		subsysDeviceID := fmt.Sprintf("0x%04x", device.SubsystemDevice)

		values = append(values, classID, vendorID, deviceID, subsysVendorID, subsysDeviceID, fmt.Sprintf("0x%02x", device.Revision))

		// Add name values if name resolution is enabled
		if c.pciNames && c.pciProvider != nil {
			vendorName := c.pciProvider.getVendorName(vendorID)
			deviceName := c.pciProvider.getDeviceName(vendorID, deviceID)
			subsysVendorName := c.pciProvider.getVendorName(subsysVendorID)
			subsysDeviceName := c.pciProvider.getSubsystemName(vendorID, deviceID, subsysVendorID, subsysDeviceID)
			className := c.pciProvider.getClassName(classID)

			values = append(values, vendorName, deviceName, subsysVendorName, subsysDeviceName, className)
		}

		ch <- c.infoDesc.mustNewConstMetric(1.0, values...)

		// MaxLinkSpeed and CurrentLinkSpeed are represented in GT/s
		var maxLinkSpeedTS float64
		if device.MaxLinkSpeed != nil {
			maxLinkSpeedTS = (*device.MaxLinkSpeed) * 1e9
		} else {
			maxLinkSpeedTS = -1
		}

		var currentLinkSpeedTS float64
		if device.CurrentLinkSpeed != nil {
			currentLinkSpeedTS = (*device.CurrentLinkSpeed) * 1e9
		} else {
			currentLinkSpeedTS = -1
		}

		// Get power state information directly from device object
		var currentPowerState string
		var hasPowerState bool
		if device.PowerState != nil {
			currentPowerState = device.PowerState.String()
			hasPowerState = true
		}

		var d3coldAllowed float64
		if device.D3coldAllowed != nil {
			if *device.D3coldAllowed {
				d3coldAllowed = 1
			} else {
				d3coldAllowed = 0
			}
		}

		// Get SR-IOV information directly from device object
		var sriovDriversAutoprobe float64
		if device.SriovDriversAutoprobe != nil {
			if *device.SriovDriversAutoprobe {
				sriovDriversAutoprobe = 1
			} else {
				sriovDriversAutoprobe = 0
			}
		}

		var sriovNumvfs float64
		if device.SriovNumvfs != nil {
			sriovNumvfs = float64(*device.SriovNumvfs)
		}

		var sriovTotalvfs float64
		if device.SriovTotalvfs != nil {
			sriovTotalvfs = float64(*device.SriovTotalvfs)
		}

		var sriovVfTotalMsix float64
		if device.SriovVfTotalMsix != nil {
			sriovVfTotalMsix = float64(*device.SriovVfTotalMsix)
		}

		// Handle numa_node with nil safety
		var numaNode float64
		if device.NumaNode != nil {
			numaNode = float64(*device.NumaNode)
		} else {
			numaNode = -1
		}

		// Handle link width fields with nil safety
		var maxLinkWidth float64
		if device.MaxLinkWidth != nil {
			maxLinkWidth = float64(*device.MaxLinkWidth)
		} else {
			maxLinkWidth = -1
		}

		var currentLinkWidth float64
		if device.CurrentLinkWidth != nil {
			currentLinkWidth = float64(*device.CurrentLinkWidth)
		} else {
			currentLinkWidth = -1
		}

		// Emit metrics for all fields except numa_node and power_state
		ch <- pcideviceMaxLinkTSDesc.mustNewConstMetric(maxLinkSpeedTS, device.Location.Strings()...)
		ch <- pcideviceMaxLinkWidthDesc.mustNewConstMetric(maxLinkWidth, device.Location.Strings()...)
		ch <- pcideviceCurrentLinkTSDesc.mustNewConstMetric(currentLinkSpeedTS, device.Location.Strings()...)
		ch <- pcideviceCurrentLinkWidthDesc.mustNewConstMetric(currentLinkWidth, device.Location.Strings()...)
		ch <- pcideviceD3coldAllowedDesc.mustNewConstMetric(d3coldAllowed, device.Location.Strings()...)
		ch <- pcideviceSriovDriversAutoprobeDesc.mustNewConstMetric(sriovDriversAutoprobe, device.Location.Strings()...)
		ch <- pcideviceSriovNumvfsDesc.mustNewConstMetric(sriovNumvfs, device.Location.Strings()...)
		ch <- pcideviceSriovTotalvfsDesc.mustNewConstMetric(sriovTotalvfs, device.Location.Strings()...)
		ch <- pcideviceSriovVfTotalMsixDesc.mustNewConstMetric(sriovVfTotalMsix, device.Location.Strings()...)

		// Emit power state metrics with state labels only if power state is available
		if hasPowerState {
			powerStates := []string{"D0", "D1", "D2", "D3hot", "D3cold", "unknown", "error"}
			deviceLabels := device.Location.Strings()
			for _, state := range powerStates {
				var value float64
				if state == currentPowerState {
					value = 1
				} else {
					value = 0
				}
				stateLabels := append(deviceLabels, state)
				ch <- pcidevicePowerStateDesc.mustNewConstMetric(value, stateLabels...)
			}
		}

		// Only emit numa_node metric if the value is available (not -1)
		if numaNode != -1 {
			ch <- pcideviceNumaNodeDesc.mustNewConstMetric(numaNode, device.Location.Strings()...)
		}
	}

	return nil
}
