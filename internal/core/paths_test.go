package core_test

import (
	"reflect"
	"testing"

	"appimage-manager/internal/core"
)

func TestXDGPathsFollowTheXDGEnvironment(t *testing.T) {
	env := map[string]string{
		"XDG_DATA_HOME":   "/data",
		"XDG_CONFIG_HOME": "/config",
		"XDG_CACHE_HOME":  "/cache",
		"XDG_DATA_DIRS":   "/usr/local/share:/opt/share",
	}

	got := core.XDGPaths("/home/jakub", func(k string) string { return env[k] })

	if got.Data != "/data/aim" || got.Applications != "/data/applications" || got.Icons != "/data/icons" ||
		got.Config != "/config/aim" || got.Units != "/config/systemd/user" || got.Autostart != "/config/autostart" ||
		got.Cache != "/cache/aim" || got.Apps != "/home/jakub/Applications" ||
		!reflect.DeepEqual(got.SystemApplications, []string{"/usr/local/share/applications", "/opt/share/applications"}) {
		t.Fatalf("got %+v", got)
	}
}

func TestXDGPathsDefaultToTheStandardLocations(t *testing.T) {
	got := core.XDGPaths("/home/jakub", func(string) string { return "" })

	if !reflect.DeepEqual(got, core.HomePaths("/home/jakub")) {
		t.Fatalf("got %+v\nwant %+v", got, core.HomePaths("/home/jakub"))
	}
}
