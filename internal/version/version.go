// Package version holds the build version, set by the release build.
package version

import (
	"runtime"
	"runtime/debug"
	"strings"
)

// Version and Commit are set with -ldflags at release time.
var (
	Version = "dev"
	Commit  = "unknown"
)

func init() {
	// A binary from `go install ...@v0.1.0` has no -ldflags, but Go records
	// the module version and, for a local build, the commit.
	if Version != "dev" {
		return
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	// Only a clean release tag counts; local builds get a long pseudo
	// version that says less than "dev" and the commit.
	if v := info.Main.Version; strings.HasPrefix(v, "v") && !strings.ContainsAny(v, "-+") {
		Version = strings.TrimPrefix(v, "v")
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" && len(setting.Value) >= 7 && Commit == "unknown" {
			Commit = setting.Value[:7]
		}
	}
}

// String describes the build for --version.
func String() string { return Version + " (commit " + Commit + ", " + runtime.Version() + ")" }
