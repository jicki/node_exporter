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

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGPUInfoResolverPrefersPCIIDsNames(t *testing.T) {
	resolver := gpuInfoResolver{
		pciProvider: newTestPCIIDProvider(t, `10de  NVIDIA Corporation
	1eb8  PCI IDs Tesla T4
1002  Advanced Micro Devices, Inc. [AMD/ATI]
	744c  Radeon RX 7900 XTX
8086  Intel Corporation
	56a0  Arc A770 Graphics
`),
	}

	tests := []struct {
		name     string
		vendorID string
		deviceID string
		want     string
	}{
		{
			name:     "nvidia pci ids name overrides hardcoded fallback",
			vendorID: "0x10de",
			deviceID: "0x1eb8",
			want:     "PCI IDs Tesla T4",
		},
		{
			name:     "amd product name comes from pci ids",
			vendorID: "0x1002",
			deviceID: "0x744c",
			want:     "Radeon RX 7900 XTX",
		},
		{
			name:     "intel product name comes from pci ids",
			vendorID: "0x8086",
			deviceID: "0x56a0",
			want:     "Arc A770 Graphics",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolver.productName(tt.vendorID, tt.deviceID); got != tt.want {
				t.Fatalf("productName(%q, %q) = %q, want %q", tt.vendorID, tt.deviceID, got, tt.want)
			}
		})
	}

	if got := resolver.vendorName("0x1002"); got != "Advanced Micro Devices, Inc. [AMD/ATI]" {
		t.Fatalf("vendorName() = %q, want %q", got, "Advanced Micro Devices, Inc. [AMD/ATI]")
	}
}

func TestGPUInfoResolverFallsBackWhenPCIIDsMissing(t *testing.T) {
	resolver := gpuInfoResolver{
		pciProvider: newTestPCIIDProvider(t, ""),
	}

	if got := resolver.productName("0x10de", "0x1eb8"); got != "NVIDIA Tesla T4" {
		t.Fatalf("productName() = %q, want %q", got, "NVIDIA Tesla T4")
	}

	if got := resolver.productName("0x1002", "0xffff"); got != "0xffff" {
		t.Fatalf("productName() = %q, want %q", got, "0xffff")
	}

	if got := resolver.vendorName("0x1002"); got != "AMD/ATI" {
		t.Fatalf("vendorName() = %q, want %q", got, "AMD/ATI")
	}
}

func TestPCIIDProviderAllowsNilLogger(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pci.ids")
	if err := os.WriteFile(path, []byte("10de  NVIDIA Corporation\n"), 0o644); err != nil {
		t.Fatalf("failed to write test pci.ids: %v", err)
	}

	provider := newPCIIDProvider(nil, nil, path)
	if got := provider.getVendorName("0x10de"); got != "NVIDIA Corporation" {
		t.Fatalf("getVendorName() = %q, want %q", got, "NVIDIA Corporation")
	}
}

func TestPCIIDProviderLogsCustomFileLoadSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pci.ids")
	if err := os.WriteFile(path, []byte("10de  NVIDIA Corporation\n\t1eb8  NVIDIA Tesla T4\n"), 0o644); err != nil {
		t.Fatalf("failed to write test pci.ids: %v", err)
	}

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	provider := newPCIIDProvider(logger, nil, path)
	if got := provider.getDeviceName(vendorNVIDIA, "0x1eb8"); got != "NVIDIA Tesla T4" {
		t.Fatalf("getDeviceName() = %q, want %q", got, "NVIDIA Tesla T4")
	}

	for _, want := range []string{
		`"level":"INFO"`,
		`"msg":"Loaded PCI IDs file"`,
		`"flag":"--collector.pcidevice.idsfile"`,
		`"configured_file":"` + path + `"`,
		`"loaded":true`,
	} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("expected log to contain %s, got %s", want, logs.String())
		}
	}
}

func TestPCIIDProviderLogsCustomFileLoadFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.ids")

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	provider := newPCIIDProvider(logger, nil, path)
	if got := provider.getVendorName(vendorNVIDIA); got != "10de" {
		t.Fatalf("getVendorName() = %q, want %q", got, "10de")
	}

	for _, want := range []string{
		`"level":"WARN"`,
		`"msg":"Failed to load PCI IDs file"`,
		`"flag":"--collector.pcidevice.idsfile"`,
		`"configured_file":"` + path + `"`,
		`"loaded":false`,
	} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("expected log to contain %s, got %s", want, logs.String())
		}
	}
}

func TestExamplePCIIDsMirrorsHardcodedNVIDIAProducts(t *testing.T) {
	provider := newPCIIDProvider(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		nil,
		filepath.Join("..", "examples", "pci.ids"),
	)

	if got := provider.getVendorName(vendorNVIDIA); got != "NVIDIA Corporation" {
		t.Fatalf("getVendorName() = %q, want %q", got, "NVIDIA Corporation")
	}

	devices := provider.pciDevices[normalizePCIID(vendorNVIDIA)]
	if got := len(devices); got != len(nvidiaProducts) {
		t.Fatalf("examples/pci.ids has %d NVIDIA products, want %d", got, len(nvidiaProducts))
	}

	for deviceID, want := range nvidiaProducts {
		if got := provider.getDeviceName(vendorNVIDIA, deviceID); got != want {
			t.Fatalf("getDeviceName(%q) = %q, want %q", deviceID, got, want)
		}
	}
}

func newTestPCIIDProvider(t *testing.T, content string) *pciIDProvider {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "pci.ids")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test pci.ids: %v", err)
	}

	return newPCIIDProvider(slog.New(slog.NewTextHandler(io.Discard, nil)), nil, path)
}
