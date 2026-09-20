package shared

import (
	"context"
	"io"

	"github.com/a-h/templ"
)

func StrComp(s string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, s)
		return err
	})
}

func RenderInto(parent templ.Component, child templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return parent.Render(templ.WithChildren(ctx, child), w)
	})
}

func ActiveKV(cond bool) templ.KeyValue[string, bool] {
	return templ.KV("active", cond)
}
