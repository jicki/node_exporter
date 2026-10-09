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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestGPUCollector(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	c, err := NewGPUCollector(logger)
	if err != nil {
		t.Fatalf("NewGPUCollector failed: %v", err)
	}

	gpuCollector, ok := c.(*gpuCollector)
	if !ok {
		t.Fatalf("NewGPUCollector returned %T, want *gpuCollector", c)
	}
	if gpuCollector.resolver.pciProvider == nil {
		t.Fatal("NewGPUCollector did not initialize pciProvider")
	}
}

func TestGPUCollectorUUID(t *testing.T) {
	const uuid = "GPU-01234567-89ab-cdef-0123-456789abcdef"
	const information = "Model: NVIDIA Tesla T4\nGPU UUID: " + uuid + "\nBus Location: 0000:65:00.0\n"
	tests := []struct {
		name        string
		information string
		missing     bool
		directory   bool
		wantUUID    string
		wantError   bool
	}{
		{name: "valid UUID", information: information, wantUUID: uuid},
		{name: "no final newline", information: "GPU UUID: " + uuid, wantUUID: uuid},
		{name: "whitespace and uppercase hex", information: "\tGPU UUID:\t GPU-01234567-89AB-CDEF-0123-456789ABCDEF \r\n", wantUUID: "GPU-01234567-89AB-CDEF-0123-456789ABCDEF"},
		{name: "missing file", missing: true},
		{name: "read error", directory: true, wantError: true},
		{name: "empty file"},
		{name: "missing field", information: "Model: NVIDIA Tesla T4\n"},
		{name: "different field", information: "Other GPU UUID: " + uuid + "\n"},
		{name: "empty value", information: "GPU UUID: \t\n", wantError: true},
		{name: "unknown value", information: "GPU UUID: Unknown\n", wantError: true},
		{name: "unavailable value", information: "GPU UUID: N/A\n", wantError: true},
		{name: "MIG UUID", information: "GPU UUID: MIG-01234567-89ab-cdef-0123-456789abcdef\n", wantError: true},
		{name: "missing prefix", information: "GPU UUID: 01234567-89ab-cdef-0123-456789abcdef\n", wantError: true},
		{name: "lowercase prefix", information: "GPU UUID: gpu-01234567-89ab-cdef-0123-456789abcdef\n", wantError: true},
		{name: "wrong length", information: "GPU UUID: GPU-01234567-89ab-cdef-0123-456789abcde\n", wantError: true},
		{name: "non hex value", information: "GPU UUID: GPU-01234567-89ab-cdef-0123-456789abcdeg\n", wantError: true},
		{name: "wrong separators", information: "GPU UUID: GPU-01234567_89ab-cdef-0123-456789abcdef\n", wantError: true},
		{name: "trailing content", information: "GPU UUID: " + uuid + " extra\n", wantError: true},
		{name: "duplicate field", information: information + "GPU UUID: " + uuid + "\n", wantError: true},
		{name: "conflicting field", information: information + "GPU UUID: GPU-fedcba98-7654-3210-fedc-ba9876543210\n", wantError: true},
		{name: "maximum size", information: information + strings.Repeat("\n", 64*1024-len(information)), wantUUID: uuid},
		{name: "oversized information", information: information + strings.Repeat("\n", 64*1024+1-len(information)), wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newGPUCollectorFixture(t)
			var logs bytes.Buffer
			c.logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			writeGPUDeviceFixture(t, "0000:65:00.0", vendorNVIDIA, "0x1eb8", "nvidia")
			if tt.directory {
				if err := os.MkdirAll(filepath.Join(*procPath, "driver/nvidia/gpus/0000:65:00.0/information"), 0o755); err != nil {
					t.Fatal(err)
				}
			} else if !tt.missing {
				writeGPUInformationFixture(t, "0000:65:00.0", tt.information)
			}
			assertGPUCollectorMetrics(t, c, fmt.Sprintf(`
# HELP node_gpu_info Information about the GPU.
# TYPE node_gpu_info gauge
node_gpu_info{device_id="0x1eb8",gpu_id="0000:65:00.0",model="NVIDIA Tesla T4",uuid="%s",vendor="NVIDIA Corporation",vendor_id="0x10de"} 1
# HELP node_gpu_cards_total Total number of GPU cards detected.
# TYPE node_gpu_cards_total gauge
node_gpu_cards_total{model="NVIDIA Tesla T4"} 1
`, tt.wantUUID))
			decoder := json.NewDecoder(&logs)
			var errorCount int
			for {
				var record struct {
					Level  string `json:"level"`
					Device string `json:"device"`
					Error  string `json:"error"`
				}
				if err := decoder.Decode(&record); err == io.EOF {
					break
				} else if err != nil {
					t.Fatal(err)
				}
				if record.Error != "" {
					errorCount++
					if record.Level != "DEBUG" || record.Device != "0000:65:00.0" {
						t.Errorf("unexpected diagnostic context: %+v", record)
					}
				}
			}
			wantErrors := 0
			if tt.wantError {
				wantErrors = 1
			}
			if errorCount != wantErrors {
				t.Errorf("got %d error diagnostics, want %d", errorCount, wantErrors)
			}
		})
	}
}

func TestGPUCollectorUUIDMatchesDeviceAndDriver(t *testing.T) {
	c := newGPUCollectorFixture(t)
	devices := []struct {
		busID    string
		vendorID string
		deviceID string
		driver   string
		uuid     string
	}{
		{"0000:65:00.0", vendorNVIDIA, "0x1eb8", "nvidia", "GPU-01234567-89ab-cdef-0123-456789abcdef"},
		{"10000:66:00.0", vendorNVIDIA, "0x1eb8", "nvidia", "GPU-fedcba98-7654-3210-fedc-ba9876543210"},
		{"0000:67:00.0", vendorNVIDIA, "0x1eb8", "vfio-pci", "GPU-01234567-89ab-cdef-0123-456789abcdef"},
		{"0000:68:00.0", vendorNVIDIA, "0x1eb8", "nouveau", "GPU-01234567-89ab-cdef-0123-456789abcdef"},
		{"0000:69:00.0", vendorAMD, "0xffff", "amdgpu", "GPU-01234567-89ab-cdef-0123-456789abcdef"},
		{"0000:6a:00.0", vendorIntel, "0xffff", "i915", "GPU-01234567-89ab-cdef-0123-456789abcdef"},
		{"0000:6b:00.0", vendorNVIDIA, "0x1eb8", "", "GPU-01234567-89ab-cdef-0123-456789abcdef"},
	}
	for _, device := range devices {
		writeGPUDeviceFixture(t, device.busID, device.vendorID, device.deviceID, device.driver)
		// 即使其他驱动的 PCI 地址下存在同名 proc 文件，也不能将其当作 NVIDIA 原生设备。
		writeGPUInformationFixture(t, device.busID, "GPU UUID: "+device.uuid+"\n")
	}
	assertGPUCollectorMetrics(t, c, `
# HELP node_gpu_info Information about the GPU.
# TYPE node_gpu_info gauge
node_gpu_info{device_id="0x1eb8",gpu_id="0000:65:00.0",model="NVIDIA Tesla T4",uuid="GPU-01234567-89ab-cdef-0123-456789abcdef",vendor="NVIDIA Corporation",vendor_id="0x10de"} 1
node_gpu_info{device_id="0x1eb8",gpu_id="10000:66:00.0",model="NVIDIA Tesla T4",uuid="GPU-fedcba98-7654-3210-fedc-ba9876543210",vendor="NVIDIA Corporation",vendor_id="0x10de"} 1
node_gpu_info{device_id="0x1eb8",gpu_id="0000:67:00.0",model="NVIDIA Tesla T4",uuid="",vendor="NVIDIA Corporation",vendor_id="0x10de"} 1
node_gpu_info{device_id="0x1eb8",gpu_id="0000:68:00.0",model="NVIDIA Tesla T4",uuid="",vendor="NVIDIA Corporation",vendor_id="0x10de"} 1
node_gpu_info{device_id="0xffff",gpu_id="0000:69:00.0",model="0xffff",uuid="",vendor="AMD/ATI",vendor_id="0x1002"} 1
node_gpu_info{device_id="0xffff",gpu_id="0000:6a:00.0",model="0xffff",uuid="",vendor="Intel Corporation",vendor_id="0x8086"} 1
# HELP node_gpu_cards_total Total number of GPU cards detected.
# TYPE node_gpu_cards_total gauge
node_gpu_cards_total{model="NVIDIA Tesla T4"} 4
node_gpu_cards_total{model="0xffff"} 2
`)
}

func TestGPUCollectorUUIDRecovers(t *testing.T) {
	c := newGPUCollectorFixture(t)
	writeGPUDeviceFixture(t, "0000:65:00.0", vendorNVIDIA, "0x1eb8", "nvidia")
	for _, uuid := range []string{
		"GPU-01234567-89ab-cdef-0123-456789abcdef",
		"",
		"GPU-fedcba98-7654-3210-fedc-ba9876543210",
	} {
		if uuid == "" {
			if err := os.Remove(filepath.Join(*procPath, "driver/nvidia/gpus/0000:65:00.0/information")); err != nil {
				t.Fatal(err)
			}
		} else {
			writeGPUInformationFixture(t, "0000:65:00.0", "GPU UUID: "+uuid+"\n")
		}
		assertGPUCollectorMetrics(t, c, fmt.Sprintf(`
# HELP node_gpu_info Information about the GPU.
# TYPE node_gpu_info gauge
node_gpu_info{device_id="0x1eb8",gpu_id="0000:65:00.0",model="NVIDIA Tesla T4",uuid="%s",vendor="NVIDIA Corporation",vendor_id="0x10de"} 1
# HELP node_gpu_cards_total Total number of GPU cards detected.
# TYPE node_gpu_cards_total gauge
node_gpu_cards_total{model="NVIDIA Tesla T4"} 1
`, uuid))
	}
}

func newGPUCollectorFixture(t *testing.T) *gpuCollector {
	t.Helper()
	dir := t.TempDir()
	oldSysPath, oldProcPath, oldRootfsPath := *sysPath, *procPath, *rootfsPath
	*sysPath = filepath.Join(dir, "sys")
	*procPath = filepath.Join(dir, "proc")
	*rootfsPath = filepath.Join(dir, "rootfs")
	t.Cleanup(func() {
		*sysPath, *procPath, *rootfsPath = oldSysPath, oldProcPath, oldRootfsPath
	})
	// 固定使用内置型号解析，避免读取宿主机 pci.ids 改变预期结果。
	return &gpuCollector{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func writeGPUDeviceFixture(t *testing.T, busID, vendorID, deviceID, driver string) {
	t.Helper()
	devicePath := filepath.Join(*sysPath, "bus/pci/devices", busID)
	if err := os.MkdirAll(devicePath, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"class": "0x030200", "vendor": vendorID, "device": deviceID} {
		if err := os.WriteFile(filepath.Join(devicePath, name), []byte(value+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if driver != "" {
		if err := os.MkdirAll(filepath.Join(*sysPath, "bus/pci/drivers", driver), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join("../../drivers", driver), filepath.Join(devicePath, "driver")); err != nil {
			t.Fatal(err)
		}
	}
}

func writeGPUInformationFixture(t *testing.T, busID, information string) {
	t.Helper()
	path := filepath.Join(*procPath, "driver/nvidia/gpus", busID, "information")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(information), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertGPUCollectorMetrics(t *testing.T, c *gpuCollector, want string) {
	t.Helper()
	reg := prometheus.NewRegistry()
	reg.MustRegister(&NodeCollector{Collectors: map[string]Collector{"gpu": c}, logger: c.logger})
	want += `
# HELP node_scrape_collector_success node_exporter: Whether a collector succeeded.
# TYPE node_scrape_collector_success gauge
node_scrape_collector_success{collector="gpu"} 1
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want), "node_gpu_info", "node_gpu_cards_total", "node_scrape_collector_success"); err != nil {
		t.Fatal(err)
	}
}

func TestGPUCollectorUsesPCIIDsFileFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pci.ids")
	if err := os.WriteFile(path, []byte("10de  NVIDIA Corporation\n\t1eb8  Flag Tesla T4\n"), 0o644); err != nil {
		t.Fatalf("failed to write pci.ids fixture: %v", err)
	}

	oldPCIIdsFile := *pciIdsFile
	*pciIdsFile = path
	t.Cleanup(func() {
		*pciIdsFile = oldPCIIdsFile
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	c, err := NewGPUCollector(logger)
	if err != nil {
		t.Fatalf("NewGPUCollector failed: %v", err)
	}

	gpuCollector := c.(*gpuCollector)
	if got := gpuCollector.resolver.productName(vendorNVIDIA, "0x1eb8"); got != "Flag Tesla T4" {
		t.Fatalf("productName() = %q, want %q", got, "Flag Tesla T4")
	}
}

func TestIsGPUDriverLoaded(t *testing.T) {
	dir := t.TempDir()
	driversDir := filepath.Join(dir, "drivers")
	if err := os.MkdirAll(driversDir, 0o755); err != nil {
		t.Fatalf("failed to create drivers dir: %v", err)
	}

	tests := []struct {
		name   string
		driver string
		want   bool
	}{
		{
			name:   "native nvidia driver",
			driver: "nvidia",
			want:   true,
		},
		{
			name:   "passthrough vfio driver",
			driver: "vfio-pci",
			want:   true,
		},
		{
			name:   "non gpu driver",
			driver: "snd_hda_intel",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driverPath := filepath.Join(driversDir, tt.driver)
			if err := os.MkdirAll(driverPath, 0o755); err != nil {
				t.Fatalf("failed to create driver dir: %v", err)
			}

			devicePath := filepath.Join(dir, tt.name)
			if err := os.MkdirAll(devicePath, 0o755); err != nil {
				t.Fatalf("failed to create device dir: %v", err)
			}
			if err := os.Symlink(driverPath, filepath.Join(devicePath, "driver")); err != nil {
				t.Fatalf("failed to create driver symlink: %v", err)
			}

			if got := isGPUDriverLoaded(devicePath); got != tt.want {
				t.Fatalf("isGPUDriverLoaded() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("missing driver link", func(t *testing.T) {
		devicePath := filepath.Join(dir, "missing-driver")
		if err := os.MkdirAll(devicePath, 0o755); err != nil {
			t.Fatalf("failed to create device dir: %v", err)
		}

		if isGPUDriverLoaded(devicePath) {
			t.Fatal("isGPUDriverLoaded() = true, want false")
		}
	})
}
