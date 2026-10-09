// Copyright 2015 The Prometheus Authors
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
	"testing"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/procfs"
)

func TestDefaultProcPath(t *testing.T) {
	if _, err := kingpin.CommandLine.Parse([]string{"--path.procfs", procfs.DefaultMountPoint}); err != nil {
		t.Fatal(err)
	}

	if got, want := procFilePath("somefile"), "/proc/somefile"; got != want {
		t.Errorf("Expected: %s, Got: %s", want, got)
	}

	if got, want := procFilePath("some/file"), "/proc/some/file"; got != want {
		t.Errorf("Expected: %s, Got: %s", want, got)
	}
}

func TestCustomProcPath(t *testing.T) {
	if _, err := kingpin.CommandLine.Parse([]string{"--path.procfs", "./../some/./place/"}); err != nil {
		t.Fatal(err)
	}

	if got, want := procFilePath("somefile"), "../some/place/somefile"; got != want {
		t.Errorf("Expected: %s, Got: %s", want, got)
	}

	if got, want := procFilePath("some/file"), "../some/place/some/file"; got != want {
		t.Errorf("Expected: %s, Got: %s", want, got)
	}
}

func TestDefaultSysPath(t *testing.T) {
	if _, err := kingpin.CommandLine.Parse([]string{"--path.sysfs", "/sys"}); err != nil {
		t.Fatal(err)
	}

	if got, want := sysFilePath("somefile"), "/sys/somefile"; got != want {
		t.Errorf("Expected: %s, Got: %s", want, got)
	}

	if got, want := sysFilePath("some/file"), "/sys/some/file"; got != want {
		t.Errorf("Expected: %s, Got: %s", want, got)
	}
}

func TestCustomSysPath(t *testing.T) {
	if _, err := kingpin.CommandLine.Parse([]string{"--path.sysfs", "./../some/./place/"}); err != nil {
		t.Fatal(err)
	}

	if got, want := sysFilePath("somefile"), "../some/place/somefile"; got != want {
		t.Errorf("Expected: %s, Got: %s", want, got)
	}

	if got, want := sysFilePath("some/file"), "../some/place/some/file"; got != want {
		t.Errorf("Expected: %s, Got: %s", want, got)
	}
}

func TestUdevDataFlag(t *testing.T) {
	flag := kingpin.CommandLine.GetFlag("path.udev.data")
	if flag == nil {
		t.Fatal("path.udev.data flag is missing")
	}
	if defaults := flag.Model().Default; len(defaults) != 1 || defaults[0] != "/run/udev/data" {
		t.Fatalf("Unexpected udev data path default: %v", defaults)
	}

	for _, test := range []struct {
		name  string
		value string
	}{
		{name: "default", value: "/run/udev/data"},
		{name: "custom", value: "../custom/udev"},
		{name: "empty", value: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			// 只解析参数，避免重置其他 collector 的全局参数值。
			context, err := kingpin.CommandLine.ParseContext([]string{"--path.udev.data=" + test.value})
			if err != nil {
				t.Fatal(err)
			}
			if len(context.Elements) != 1 {
				t.Fatalf("Expected one parsed flag, got %d", len(context.Elements))
			}
			element := context.Elements[0]
			if element.Clause != flag || element.Value == nil || *element.Value != test.value {
				t.Fatalf("Unexpected parsed udev data flag: %+v", element)
			}
		})
	}
}
