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

package collector

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

type pciIDProvider struct {
	pciVendors    map[string]string
	pciDevices    map[string]map[string]string
	pciSubsystems map[string]map[string]string
	pciClasses    map[string]string
	pciSubclasses map[string]string
	pciProgIfs    map[string]string
	logger        *slog.Logger
}

func newPCIIDProvider(logger *slog.Logger, paths []string, customPath string) *pciIDProvider {
	p := &pciIDProvider{
		logger:        logger,
		pciVendors:    make(map[string]string),
		pciDevices:    make(map[string]map[string]string),
		pciSubsystems: make(map[string]map[string]string),
		pciClasses:    make(map[string]string),
		pciSubclasses: make(map[string]string),
		pciProgIfs:    make(map[string]string),
	}
	p.load(paths, customPath)
	return p
}

func (p *pciIDProvider) load(paths []string, customPath string) {
	var file *os.File
	var err error

	// Use custom pci.ids file if specified
	if customPath != "" {
		file, err = os.Open(customPath)
		if err != nil {
			p.logger.Debug("Failed to open PCI IDs file", "file", customPath, "error", err)
			return
		}
		p.logger.Debug("Loading PCI IDs from", "file", customPath)
	} else {
		// Try each possible default path
		for _, path := range paths {
			file, err = os.Open(path)
			if err == nil {
				p.logger.Debug("Loading PCI IDs from default path", "path", path)
				break
			}
		}
		if err != nil {
			p.logger.Debug("Failed to open any default PCI IDs file", "error", err)
			return
		}
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var currentVendor, currentDevice, currentBaseClass, currentSubclass string
	var inClassContext bool

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Handle class lines (starts with 'C')
		if strings.HasPrefix(line, "C ") {
			parts := strings.SplitN(line, "  ", 2)
			if len(parts) >= 2 {
				classID := strings.TrimSpace(parts[0][1:]) // Remove 'C' prefix
				className := strings.TrimSpace(parts[1])
				p.pciClasses[classID] = className
				currentBaseClass = classID
				inClassContext = true
			}
			continue
		}

		// Handle subclass lines (single tab after class)
		if strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "\t\t") && inClassContext {
			line = strings.TrimPrefix(line, "\t")
			parts := strings.SplitN(line, "  ", 2)
			if len(parts) >= 2 && currentBaseClass != "" {
				subclassID := strings.TrimSpace(parts[0])
				subclassName := strings.TrimSpace(parts[1])
				// Store as base class + subclass
				fullClassID := currentBaseClass + subclassID
				p.pciSubclasses[fullClassID] = subclassName
				currentSubclass = fullClassID
			}
			continue
		}

		// Handle programming interface lines (double tab after subclass)
		if strings.HasPrefix(line, "\t\t") && !strings.HasPrefix(line, "\t\t\t") && inClassContext {
			line = strings.TrimPrefix(line, "\t\t")
			parts := strings.SplitN(line, "  ", 2)
			if len(parts) >= 2 && currentSubclass != "" {
				progIfID := strings.TrimSpace(parts[0])
				progIfName := strings.TrimSpace(parts[1])
				// Store as base class + subclass + programming interface
				fullClassID := currentSubclass + progIfID
				p.pciProgIfs[fullClassID] = progIfName
			}
			continue
		}

		// Handle vendor lines (no leading whitespace, not starting with 'C')
		if !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "C ") {
			parts := strings.SplitN(line, "  ", 2)
			if len(parts) >= 2 {
				currentVendor = strings.TrimSpace(parts[0])
				p.pciVendors[currentVendor] = strings.TrimSpace(parts[1])
				currentDevice = ""
				inClassContext = false
			}
			continue
		}

		// Handle device lines (single tab)
		if strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "\t\t") {
			line = strings.TrimPrefix(line, "\t")
			parts := strings.SplitN(line, "  ", 2)
			if len(parts) >= 2 && currentVendor != "" {
				currentDevice = strings.TrimSpace(parts[0])
				if p.pciDevices[currentVendor] == nil {
					p.pciDevices[currentVendor] = make(map[string]string)
				}
				p.pciDevices[currentVendor][currentDevice] = strings.TrimSpace(parts[1])
			}
			continue
		}

		// Handle subsystem lines (double tab)
		if strings.HasPrefix(line, "\t\t") {
			line = strings.TrimPrefix(line, "\t\t")
			parts := strings.SplitN(line, "  ", 2)
			if len(parts) >= 2 && currentVendor != "" && currentDevice != "" {
				subsysID := strings.TrimSpace(parts[0])
				subsysName := strings.TrimSpace(parts[1])
				key := fmt.Sprintf("%s:%s", currentVendor, currentDevice)
				if p.pciSubsystems[key] == nil {
					p.pciSubsystems[key] = make(map[string]string)
				}
				// Convert subsystem ID from "vendor device" format to "vendor:device" format
				subsysParts := strings.Fields(subsysID)
				if len(subsysParts) == 2 {
					subsysKey := fmt.Sprintf("%s:%s", subsysParts[0], subsysParts[1])
					p.pciSubsystems[key][subsysKey] = subsysName
				}
			}
		}
	}

	// Debug summary
	totalDevices := 0
	for _, devices := range p.pciDevices {
		totalDevices += len(devices)
	}
	totalSubsystems := 0
	for _, subsystems := range p.pciSubsystems {
		totalSubsystems += len(subsystems)
	}

	p.logger.Debug("Loaded PCI device data",
		"vendors", len(p.pciVendors),
		"devices", totalDevices,
		"subsystems", totalSubsystems,
		"classes", len(p.pciClasses),
		"subclasses", len(p.pciSubclasses),
		"progIfs", len(p.pciProgIfs),
	)
}

func (p *pciIDProvider) getVendorName(vendorID string) string {
	vendorID = strings.ToLower(strings.TrimPrefix(vendorID, "0x"))
	if name, ok := p.pciVendors[vendorID]; ok {
		return name
	}
	return vendorID
}

func (p *pciIDProvider) getDeviceName(vendorID, deviceID string) string {
	vendorID = strings.ToLower(strings.TrimPrefix(vendorID, "0x"))
	deviceID = strings.ToLower(strings.TrimPrefix(deviceID, "0x"))

	if devices, ok := p.pciDevices[vendorID]; ok {
		if name, ok := devices[deviceID]; ok {
			return name
		}
	}
	return deviceID
}

func (p *pciIDProvider) getSubsystemName(vendorID, deviceID, subsysVendorID, subsysDeviceID string) string {
	vendorID = strings.ToLower(strings.TrimPrefix(vendorID, "0x"))
	deviceID = strings.ToLower(strings.TrimPrefix(deviceID, "0x"))
	subsysVendorID = strings.ToLower(strings.TrimPrefix(subsysVendorID, "0x"))
	subsysDeviceID = strings.ToLower(strings.TrimPrefix(subsysDeviceID, "0x"))

	key := fmt.Sprintf("%s:%s", vendorID, deviceID)
	subsysKey := fmt.Sprintf("%s:%s", subsysVendorID, subsysDeviceID)

	if subsystems, ok := p.pciSubsystems[key]; ok {
		if name, ok := subsystems[subsysKey]; ok {
			return name
		}
	}
	return subsysDeviceID
}

func (p *pciIDProvider) getClassName(classID string) string {
	classID = strings.ToLower(strings.TrimPrefix(classID, "0x"))

	// Try to find the programming interface first (6 digits)
	if len(classID) >= 6 {
		progIf := classID[:6]
		if className, exists := p.pciProgIfs[progIf]; exists {
			return className
		}
	}

	// Try to find the subclass (4 digits)
	if len(classID) >= 4 {
		subclass := classID[:4]
		if className, exists := p.pciSubclasses[subclass]; exists {
			return className
		}
	}

	// If not found, try with just the base class (first 2 digits)
	if len(classID) >= 2 {
		baseClass := classID[:2]
		if className, exists := p.pciClasses[baseClass]; exists {
			return className
		}
	}

	return "Unknown class (" + classID + ")"
}
