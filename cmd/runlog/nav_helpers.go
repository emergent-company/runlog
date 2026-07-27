package main

import "github.com/a-h/templ"

func NavRowAttrs(url string) templ.Attributes {
	return templ.Attributes{
		"hx-get":    url,
		"hx-target": "#main-content",
		"hx-swap":   "innerHTML",
		"hx-push-url": "true",
		"hx-sync":   "#main-content:replace",
	}
}

func NavTriggerAttrs(url string) templ.Attributes {
	return templ.Attributes{
		"hx-get":    url,
		"hx-target": "#main-content",
		"hx-swap":   "innerHTML",
		"hx-push-url": "true",
		"hx-sync":   "#main-content:replace",
		"hx-trigger": "click[!target.closest('.dropdown')]",
	}
}
