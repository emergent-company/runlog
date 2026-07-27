package form

import (
	"strings"

	"github.com/a-h/templ"
)

type LazyLoadConfig struct {
	GuardName  string
	LibVarName string
	ScriptURL  string
	StyleURL   string
	Selector   string
	InitFunc   string
	FnSuffix   string
}

func LazyLoadScript(cfg LazyLoadConfig) templ.ComponentScript {
	var b strings.Builder

	b.WriteString(`if (!window.` + cfg.GuardName + `) {
  window.` + cfg.GuardName + ` = true;
  function init` + cfg.FnSuffix + `Element(el) {
    ` + cfg.InitFunc + `
    el.` + cfg.GuardName + ` = true;
  }
  window.init` + cfg.FnSuffix + ` = function(el) {
    if (el.` + cfg.GuardName + ` || el.classList.contains('` + cfg.GuardName + `-initialized')) return;
    try {
      if (typeof ` + cfg.LibVarName + ` === 'undefined') {
        var s = document.createElement('script');
        s.src = '` + cfg.ScriptURL + `';
        s.onload = function() { init` + cfg.FnSuffix + `Element(el); };
        document.head.appendChild(s);`)

	if cfg.StyleURL != "" {
		b.WriteString(`
        var l = document.createElement('link');
        l.rel = 'stylesheet';
        l.href = ` + cfg.StyleURL + `;
        document.head.appendChild(l);`)
	}

	b.WriteString(`
      } else {
        init` + cfg.FnSuffix + `Element(el);
      }
    } catch(e) { console.warn('` + cfg.LibVarName + ` init failed:', e); }
  };
  document.addEventListener('DOMContentLoaded', function() {
    document.querySelectorAll('` + cfg.Selector + `').forEach(init` + cfg.FnSuffix + `Element);
  });
  document.addEventListener('htmx:after:settle', function() {
    document.querySelectorAll('` + cfg.Selector + `:not(.` + cfg.GuardName + `-initialized)').forEach(function(el) {
      el.classList.add('` + cfg.GuardName + `-initialized');
      init` + cfg.FnSuffix + `Element(el);
    });
  });
}
`)

	return templ.ComponentScript{
		Name:     "form-lazyload-" + cfg.GuardName,
		Function: b.String(),
	}
}
