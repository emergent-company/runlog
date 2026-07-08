package main

import (
	"testing"
)

func TestURLs_LinterPaths(t *testing.T) {
	cases := []struct {
		name        string
		url         string
		wantSegment string
	}{
		{name: "LinterDetailURL(core/gofmt)", url: LinterDetailURL("core/gofmt"), wantSegment: "core--gofmt"},
		{name: "LinterDetailURL(e2e/no-secrets)", url: LinterDetailURL("e2e/no-secrets"), wantSegment: "e2e--no-secrets"},
		{name: "LinterRunDetailURL(core/go-build, 42)", url: LinterRunDetailURL("core/go-build", 42), wantSegment: "core--go-build"},
		{name: "RunLinterURL(cmd/runlog/gofmt)", url: RunLinterURL("cmd/runlog/gofmt"), wantSegment: "cmd--runlog--gofmt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := "/ui/linters/" + tc.wantSegment
			if !contains(tc.url, want) && !contains(tc.url, want+"/") {
				t.Errorf("got %s, want to contain %s", tc.url, want)
			}
		})
	}
}

func TestURLs_StaticPaths(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"HomeURL", HomeURL(), "/ui/"},
		{"TestsURL", TestsURL(), "/ui/tests"},
		{"CatalogURL", CatalogURL(), "/ui/catalog"},
		{"AllRunsURL", AllRunsURL(), "/ui/runs"},
		{"LintersURL", LintersURL(), "/ui/linters"},
		{"EnvironmentsURL", EnvironmentsURL(), "/ui/environments"},
		{"ExperimentsURL", ExperimentsURL(), "/ui/experiments"},
		{"SDKReferenceURL", SDKReferenceURL(), "/ui/events"},
		{"SearchURL", SearchURL(), "/ui/search"},
		{"FooterStatusURL", FooterStatusURL(), "/ui/footer-status"},
		{"RunAllLintersURL", RunAllLintersURL(), "/ui/linters/run-all"},
		{"LinterEventsSSEURL", LinterEventsSSEURL(), "/ui/linters/events"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %s, want %s", tc.got, tc.want)
			}
		})
	}
}

func TestURLs_ParameterizedPaths(t *testing.T) {
	t.Run("RunDetailURL", func(t *testing.T) {
		if RunDetailURL(42) != "/ui/runs/42" {
			t.Errorf("got %s", RunDetailURL(42))
		}
	})
	t.Run("TestDetailURL escape", func(t *testing.T) {
		if TestDetailURL("TestFoo_Bar") != "/ui/tests/TestFoo_Bar" {
			t.Errorf("got %s", TestDetailURL("TestFoo_Bar"))
		}
	})
	t.Run("LaunchURL", func(t *testing.T) {
		if LaunchURL("TestFoo_Bar") != "/ui/launch/TestFoo_Bar" {
			t.Errorf("got %s", LaunchURL("TestFoo_Bar"))
		}
	})
	t.Run("EnvironmentDetailURL", func(t *testing.T) {
		if EnvironmentDetailURL("mcj-emergent") != "/ui/environments/mcj-emergent" {
			t.Errorf("got %s", EnvironmentDetailURL("mcj-emergent"))
		}
	})
	t.Run("ExperimentDetailURL", func(t *testing.T) {
		if ExperimentDetailURL("baseline") != "/ui/experiments/baseline" {
			t.Errorf("got %s", ExperimentDetailURL("baseline"))
		}
	})
	t.Run("LinterRunDetailURL", func(t *testing.T) {
		got := LinterRunDetailURL("core/gofmt", 12)
		if got != "/ui/linters/core--gofmt/runs/12" {
			t.Errorf("got %s", got)
		}
	})
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchSubstr(s, sub)
}

func searchSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
