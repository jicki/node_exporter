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
	"testing"

	"github.com/alecthomas/kingpin/v2"
)

func TestUdevDataFilePath(t *testing.T) {
	flag := kingpin.CommandLine.GetFlag("path.udev.data")
	if flag == nil {
		t.Fatal("path.udev.data flag is missing")
	}
	value := flag.Model().Value
	if value == nil {
		t.Fatal("path.udev.data flag has no value binding")
	}
	original := value.String()
	t.Cleanup(func() {
		if err := value.Set(original); err != nil {
			t.Errorf("Restore udev data path: %v", err)
		}
	})

	for _, test := range []struct {
		name string
		path string
		want string
	}{
		{name: "default", path: "/run/udev/data", want: "/run/udev/data/b8:0"},
		{name: "custom", path: "/host/run/udev/data", want: "/host/run/udev/data/b8:0"},
		{name: "relative", path: "../custom/./data/", want: "../custom/data/b8:0"},
		{name: "empty", path: "", want: "b8:0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := value.Set(test.path); err != nil {
				t.Fatal(err)
			}
			if got := udevDataFilePath("b8:0"); got != test.want {
				t.Errorf("Expected %q, got %q", test.want, got)
			}
		})
	}
}
