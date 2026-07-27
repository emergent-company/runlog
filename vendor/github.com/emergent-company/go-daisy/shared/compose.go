package shared

import (
	"context"
	"io"

	"github.com/a-h/templ"
)

func Compose(components ...templ.Component) templ.Component {
	if len(components) == 1 {
		return components[0]
	}
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		for _, c := range components {
			if err := c.Render(ctx, w); err != nil {
				return err
			}
		}
		return nil
	})
}
