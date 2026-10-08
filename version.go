package main

import (
	"runtime/debug"
	"strings"
)

// Version задаётся при релизе через ldflags (-X main.Version=…).
// Для go install / локальной сборки без ldflags берётся версия модуля из VCS-тега.
var Version = "0.1.0"

// readBuildInfo подменяется в тестах.
var readBuildInfo = debug.ReadBuildInfo

func displayVersion() string {
	if v := strings.TrimSpace(Version); v != "" && v != "dev" {
		return v
	}
	if bi, ok := readBuildInfo(); ok {
		if v := strings.TrimPrefix(bi.Main.Version, "v"); v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}
