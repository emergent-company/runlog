package main

import (
	"testing"

	"github.com/a-h/templ"
)

func TestNavRowAttrs_Basic(t *testing.T) {
	a := NavRowAttrs("/ui/tests")
	if a["hx-get"] != "/ui/tests" {
		t.Errorf("hx-get: got %v, want /ui/tests", a["hx-get"])
	}
	if a["hx-target"] != "#main-content" {
		t.Errorf("hx-target: got %v", a["hx-target"])
	}
	if a["hx-swap"] != "innerHTML" {
		t.Errorf("hx-swap: got %v", a["hx-swap"])
	}
	if a["hx-push-url"] != "true" {
		t.Errorf("hx-push-url: got %v", a["hx-push-url"])
	}
	if a["hx-sync"] != "#main-content:replace" {
		t.Errorf("hx-sync: got %v", a["hx-sync"])
	}
	if _, hasTrigger := a["hx-trigger"]; hasTrigger {
		t.Error("NavRowAttrs should not have hx-trigger")
	}
	if len(a) != 5 {
		t.Errorf("expected 5 attrs, got %d", len(a))
	}
}

func TestNavTriggerAttrs_IncludesTrigger(t *testing.T) {
	a := NavTriggerAttrs("/ui/linters/core--gofmt")
	if a["hx-trigger"] != "click[!target.closest('.dropdown')]" {
		t.Errorf("hx-trigger: got %v", a["hx-trigger"])
	}
	if len(a) != 6 {
		t.Errorf("expected 6 attrs (5 nav + trigger), got %d", len(a))
	}
}

func TestNavRowAttrs_UsedAsTemplAttributes(t *testing.T) {
	a := NavRowAttrs("/ui/runs/42")
	_ = templ.Attributes(a)
}

func TestNavTriggerAttrs_UsedAsTemplAttributes(t *testing.T) {
	a := NavTriggerAttrs("/ui/linters/test")
	_ = templ.Attributes(a)
}
