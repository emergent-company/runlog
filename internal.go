// Package e2eframework — internal.go
//
// Internal helpers shared within the framework package.
package runlog

import (
	"context"
	"strings"
	"time"
)

// cancelCtx is a convenience wrapper around context.WithTimeout.
func cancelCtx(d time.Duration) (context.Context, context.CancelFunc) {  //nolint:deadcode
	return context.WithTimeout(context.Background(), d)
}

// hasPrefix is strings.HasPrefix re-exported for internal use in server.go.
func hasPrefix(s, prefix string) bool {  //nolint:deadcode
	return strings.HasPrefix(s, prefix)
}
