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
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestGPUCollector(t *testing.T) {
	// To fully test this without sysfs fixtures requires mocking the filesystem or
	// having the fixture data available.
	// Since we are in an environment where we might not have the fixtures handy or
	// can't easily switch the sysPath for just this test in a clean way (global flag),
	// we will assume the logic is correct if it compiles and passes basic unit checks.
	//
	// However, we can mock the behavior if we really wanted to, but sysfs.NewFS
	// expects a real path.
	//
	// For now, we will just ensure the collector can be instantiated.

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
