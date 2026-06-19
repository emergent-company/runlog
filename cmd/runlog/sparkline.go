package main

import (
	"fmt"
	"strings"

	"github.com/a-h/templ"
)

// sparklineComponent returns a templ.Component that renders an SVG sparkline.
func sparklineComponent(points []trendPoint) templ.Component {
	if len(points) < 2 {
		return templ.Raw(`<span class="text-base-content/30 text-xs">insufficient data</span>`)
	}

	maxVal := points[0].DurationMS
	for _, p := range points {
		if p.DurationMS > maxVal {
			maxVal = p.DurationMS
		}
	}
	if maxVal == 0 {
		maxVal = 1
	}
	const w, h, pad = 200.0, 40.0, 2.0
	stepX := (w - pad*2) / float64(len(points)-1)

	var pts strings.Builder
	var firstCX, firstCY, lastCX, lastCY string
	for i, p := range points {
		x := pad + float64(i)*stepX
		y := h - pad - (p.DurationMS/maxVal)*(h-pad*2)
		if i > 0 {
			pts.WriteString(" ")
		}
		pts.WriteString(fmt.Sprintf("%.1f,%.1f", x, y))
		if i == 0 {
			firstCX = fmt.Sprintf("%.1f", x)
			firstCY = fmt.Sprintf("%.1f", y)
		}
		if i == len(points)-1 {
			lastCX = fmt.Sprintf("%.1f", x)
			lastCY = fmt.Sprintf("%.1f", y)
		}
	}

	dotClass := "text-base-content/70"
	if cls := trendDotClass(points); cls != "" {
		dotClass = cls
	}

	svg := fmt.Sprintf(
		`<svg width="200" height="40" viewBox="0 0 200 40" class="shrink-0"><rect width="200" height="40" fill="transparent" rx="4"/><polyline points="%s" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" class="text-base-content/70"/><circle cx="%s" cy="%s" r="2" fill="currentColor" class="text-base-content/70"/><circle cx="%s" cy="%s" r="2" fill="currentColor" class="%s"/></svg>`,
		pts.String(), firstCX, firstCY, lastCX, lastCY, dotClass,
	)
	return templ.Raw(svg)
}
