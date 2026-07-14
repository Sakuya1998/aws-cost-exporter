// Package version exposes build metadata injected by the release toolchain.
package version

import (
	"fmt"
	"runtime"
)

var (
	version   = "dev"
	revision  = "unknown"
	buildDate = "unknown"
)

// Info describes the executable version in machine-readable fields.
type Info struct {
	Version   string `json:"version"`
	Revision  string `json:"revision"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
}

// Current returns the metadata compiled into the running executable.
func Current() Info {
	return Info{
		Version:   version,
		Revision:  revision,
		BuildDate: buildDate,
		GoVersion: runtime.Version(),
	}
}

// String returns a stable single-line representation for command-line output.
func (info Info) String() string {
	return fmt.Sprintf(
		"aws-cost-exporter version=%s revision=%s build_date=%s go_version=%s",
		info.Version,
		info.Revision,
		info.BuildDate,
		info.GoVersion,
	)
}
