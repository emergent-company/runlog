package layout

func fillTopbarDefaults(opts TopbarVariantOpts, style string) TopbarVariantOpts {
	if opts.SearchPlaceholder == "" {
		opts.SearchPlaceholder = "Search or jump to..."
	}
	if opts.Greeting == "" {
		opts.Greeting = "Good Morning"
	}
	if opts.UserName == "" {
		opts.UserName = "User"
	}
	if opts.Subtitle == "" {
		opts.Subtitle = "Welcome back, great to see you again!"
	}
	if len(opts.NavLinks) == 0 {
		opts.NavLinks = defaultTopbarNavLinks(style)
	}
	return opts
}

func defaultTopbarNavLinks(style string) []TopbarNavLink {
	switch style {
	case "nav-menu-1":
		return []TopbarNavLink{
			{Label: "Apps", Href: "#"},
			{Label: "Components", Href: "#", Active: true},
			{Label: "Pages", Href: "#"},
		}
	default:
		return []TopbarNavLink{
			{Label: "Dashboard", Href: "#", Active: true},
			{Label: "Analytics", Href: "#"},
			{Label: "Settings", Href: "#"},
			{Label: "Users", Href: "#"},
			{Label: "Reports", Href: "#"},
			{Label: "Support", Href: "#"},
		}
	}
}
