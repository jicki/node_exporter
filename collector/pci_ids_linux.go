// Copyright The Prometheus Authors
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
	"strings"

	"github.com/alecthomas/kingpin/v2"
)

var (
	pciIdsPaths = []string{
		"/usr/share/misc/pci.ids",
		"/usr/share/hwdata/pci.ids",
		"/var/lib/pciutils/pci.ids",
	}
	pciIdsFile = kingpin.Flag("collector.pcidevice.idsfile", "Path to pci.ids file to use for PCI and GPU device identification.").String()
)

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
