package layout

import (
	"fmt"
	"strconv"

	"github.com/a-h/templ"
)

func sprintTrial(days int) string {
	if days <= 0 {
		return "5"
	}
	return strconv.Itoa(days)
}

func sprintTrialProgress(days int) string {
	if days <= 0 {
		return "70"
	}
	pct := (float64(days) / 30.0) * 100
	if pct > 100 {
		pct = 100
	}
	return strconv.Itoa(int(pct))
}

func sprintRadialClass(used, max int) string {
	if max <= 0 {
		return "text-primary"
	}
	pct := float64(used) / float64(max) * 100
	switch {
	case pct >= 90:
		return "text-error"
	case pct >= 70:
		return "text-warning"
	default:
		return "text-primary"
	}
}

func sprintRadialStyle(used, max int) templ.SafeCSS {
	if max <= 0 {
		return templ.SafeCSS("--value:0")
	}
	pct := float64(used) / float64(max) * 100
	if pct > 100 {
		pct = 100
	}
	return templ.SafeCSS("--value:" + strconv.Itoa(int(pct)))
}

func sprintTokenPct(used, max int) string {
	if max <= 0 {
		return "0%"
	}
	pct := float64(used) / float64(max) * 100
	if pct > 100 {
		pct = 100
	}
	return strconv.Itoa(int(pct)) + "%"
}

func sprintTokenFmt(n int) string {
	switch {
	case n >= 1000000:
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	case n >= 1000:
		return fmt.Sprintf("%.0fK", float64(n)/1000)
	default:
		return strconv.Itoa(n)
	}
}

func sprintVersions(current string, versions []string) []string {
	if len(versions) > 0 {
		return versions
	}
	return []string{"v3.0.0", "v2.1.0", "v1.8.3"}
}
