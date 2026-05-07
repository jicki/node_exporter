// Copyright 2026 The Prometheus Authors
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

import "strings"

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

// NVIDIA device ID to product name mapping (fallback for systems without pci.ids)
var nvidiaProducts = map[string]string{
	// Data Center - Tesla
	"0x1eb8": "NVIDIA Tesla T4",
	"0x1db4": "NVIDIA Tesla V100-PCIE-16GB",
	"0x1db5": "NVIDIA Tesla V100-PCIE-32GB",
	"0x1db6": "NVIDIA Tesla V100-SXM2-16GB",
	"0x1db7": "NVIDIA Tesla V100-SXM2-32GB",
	"0x1df6": "NVIDIA Tesla V100S-PCIE-32GB",
	"0x1e30": "NVIDIA Tesla T4",
	"0x1e37": "NVIDIA Tesla T4",

	// Data Center - Ampere (A-series)
	"0x20b0": "NVIDIA A100-SXM4-40GB",
	"0x20b2": "NVIDIA A100-SXM4-40GB",
	"0x20b5": "NVIDIA A100-SXM4-80GB",
	"0x20b7": "NVIDIA A30",
	"0x20f1": "NVIDIA A100-SXM4-80GB",
	"0x20f3": "NVIDIA A800-SXM4-80GB",
	"0x20f5": "NVIDIA A800-SXM4-80GB",
	"0x2236": "NVIDIA A10",
	"0x2237": "NVIDIA A10G",
	"0x25b6": "NVIDIA A16",

	// Data Center - Hopper (H-series)
	"0x2330": "NVIDIA H100-PCIE",
	"0x2331": "NVIDIA H100-SXM5-80GB",
	"0x2339": "NVIDIA H100-NVL",
	"0x233a": "NVIDIA H800-PCIE",
	"0x233d": "NVIDIA H800-SXM5",

	// Data Center - Ada Lovelace (L-series)
	"0x26b5": "NVIDIA L40",
	"0x26b9": "NVIDIA L40S",
	"0x26ba": "NVIDIA L20",
	"0x27b0": "NVIDIA L4",
	"0x27b6": "NVIDIA L2",

	// GeForce RTX 50 Series (Blackwell)
	"0x2b85": "NVIDIA GeForce RTX 5090", // Verified
	"0x2c02": "NVIDIA GeForce RTX 5080", // Verified
	"0x2980": "NVIDIA GeForce RTX 5090",
	"0x29c0": "NVIDIA GeForce RTX 5090",
	"0x2c18": "NVIDIA GeForce RTX 5090",
	"0x2c58": "NVIDIA GeForce RTX 5090",
	"0x2c19": "NVIDIA GeForce RTX 5080",
	"0x2c2c": "NVIDIA GeForce RTX 5080",
	"0x2c59": "NVIDIA GeForce RTX 5080",

	// GeForce RTX 40 Series (Ada Lovelace)
	"0x2684": "NVIDIA GeForce RTX 4090",
	"0x2685": "NVIDIA GeForce RTX 4090",
	"0x2702": "NVIDIA GeForce RTX 4080 SUPER",
	"0x2704": "NVIDIA GeForce RTX 4080",
	"0x2705": "NVIDIA GeForce RTX 4070 Ti SUPER",
	"0x2709": "NVIDIA GeForce RTX 4070",
	"0x2782": "NVIDIA GeForce RTX 4070 Ti",
	"0x2783": "NVIDIA GeForce RTX 4070 SUPER",
	"0x2786": "NVIDIA GeForce RTX 4060",
	"0x2803": "NVIDIA GeForce RTX 4060 Ti",
	"0x2882": "NVIDIA GeForce RTX 4060",
	"0x28a0": "NVIDIA GeForce RTX 4080",
	"0x28a1": "NVIDIA GeForce RTX 4090",

	// GeForce RTX 30 Series (Ampere)
	"0x2204": "NVIDIA GeForce RTX 3090",
	"0x2205": "NVIDIA GeForce RTX 3090 Ti",
	"0x2206": "NVIDIA GeForce RTX 3080",
	"0x2207": "NVIDIA GeForce RTX 3080",
	"0x2208": "NVIDIA GeForce RTX 3080 Ti",
	"0x220a": "NVIDIA GeForce RTX 3080 12GB",
	"0x2216": "NVIDIA GeForce RTX 3080",
	"0x2230": "NVIDIA GeForce RTX 3080 Ti",
	"0x2414": "NVIDIA GeForce RTX 3060 Ti",
	"0x2420": "NVIDIA GeForce RTX 3080",
	"0x2482": "NVIDIA GeForce RTX 3070 Ti",
	"0x2484": "NVIDIA GeForce RTX 3070",
	"0x2486": "NVIDIA GeForce RTX 3060 Ti",
	"0x2488": "NVIDIA GeForce RTX 3070",
	"0x2503": "NVIDIA GeForce RTX 3060",
	"0x2504": "NVIDIA GeForce RTX 3060",
	"0x2507": "NVIDIA GeForce RTX 3050",
	"0x2520": "NVIDIA GeForce RTX 3060",
	"0x2560": "NVIDIA GeForce RTX 3060",
	"0x2563": "NVIDIA GeForce RTX 3050 Ti",

	// GeForce RTX 20 Series (Turing)
	"0x1e04": "NVIDIA GeForce RTX 2080 Ti",
	"0x1e07": "NVIDIA GeForce RTX 2080 Ti",
	"0x1e82": "NVIDIA GeForce RTX 2080",
	"0x1e84": "NVIDIA GeForce RTX 2080 SUPER",
	"0x1e87": "NVIDIA GeForce RTX 2080",
	"0x1e89": "NVIDIA GeForce RTX 2060",
	"0x1f02": "NVIDIA GeForce RTX 2070",
	"0x1f06": "NVIDIA GeForce RTX 2060 SUPER",
	"0x1f07": "NVIDIA GeForce RTX 2070",
	"0x1f08": "NVIDIA GeForce RTX 2060",
	"0x1f10": "NVIDIA GeForce RTX 2070",
	"0x1f11": "NVIDIA GeForce RTX 2060",
	"0x1f42": "NVIDIA GeForce RTX 2060",
	"0x1f47": "NVIDIA GeForce RTX 2060 SUPER",

	// GeForce GTX 16 Series (Turing)
	"0x1f82": "NVIDIA GeForce GTX 1650",
	"0x1f91": "NVIDIA GeForce GTX 1650",
	"0x1f92": "NVIDIA GeForce GTX 1650",
	"0x2182": "NVIDIA GeForce GTX 1660 Ti",
	"0x2184": "NVIDIA GeForce GTX 1660",
	"0x2187": "NVIDIA GeForce GTX 1650 SUPER",
	"0x2188": "NVIDIA GeForce GTX 1650",
	"0x21c4": "NVIDIA GeForce GTX 1660 SUPER",

	// Quadro/Professional
	"0x1e36": "NVIDIA Quadro RTX 6000",
	"0x1e78": "NVIDIA Quadro RTX 6000",
	"0x1eb0": "NVIDIA Quadro RTX 5000",
	"0x1eb1": "NVIDIA Quadro RTX 4000",
	"0x1fb0": "NVIDIA Quadro T1000",
	"0x1fb8": "NVIDIA Quadro T2000",
	"0x2231": "NVIDIA RTX A6000",
	"0x2232": "NVIDIA RTX A5000",
	"0x2233": "NVIDIA RTX A5500",
	"0x2235": "NVIDIA RTX A40",
	"0x2571": "NVIDIA RTX A2000",
	"0x25a2": "NVIDIA RTX A5000",
	"0x25a5": "NVIDIA RTX A4000",
	"0x25b8": "NVIDIA RTX A4000",
	"0x26b1": "NVIDIA RTX 6000 Ada",
	"0x26b2": "NVIDIA RTX 5000 Ada",
}

type gpuInfoResolver struct {
	pciProvider *pciIDProvider
}

func (r gpuInfoResolver) vendorName(vendorID string) string {
	if r.pciProvider != nil {
		if name := r.pciProvider.getVendorName(vendorID); pciIDLookupMatched(name, vendorID) {
			return name
		}
	}

	switch vendorID {
	case vendorNVIDIA:
		return "NVIDIA Corporation"
	case vendorAMD:
		return "AMD/ATI"
	case vendorIntel:
		return "Intel Corporation"
	default:
		return vendorID
	}
}

func (r gpuInfoResolver) productName(vendorID, deviceID string) string {
	if r.pciProvider != nil {
		if name := r.pciProvider.getDeviceName(vendorID, deviceID); pciIDLookupMatched(name, deviceID) {
			return name
		}
	}

	if vendorID == vendorNVIDIA {
		if name, ok := nvidiaProducts[deviceID]; ok {
			return name
		}
	}

	return deviceID
}

func pciIDLookupMatched(name, id string) bool {
	return name != normalizePCIID(id)
}

func normalizePCIID(id string) string {
	return strings.TrimPrefix(strings.ToLower(id), "0x")
}
