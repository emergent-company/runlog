package shared

import twmerge "github.com/Oudwins/tailwind-merge-go"

// TwMerge resolves Tailwind utility CSS class conflicts. When multiple
// mutually exclusive utility classes appear (e.g. "w-full w-auto"),
// the last one wins. Call TwMerge on the final class string to collapse
// conflicting utilities into a single resolved value.
//
// Usage in component .templ files via thin wrapper in each package.
func TwMerge(classes ...string) string {
	return twmerge.Merge(classes...)
}
