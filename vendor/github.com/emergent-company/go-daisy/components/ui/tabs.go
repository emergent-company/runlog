package ui

import "github.com/a-h/templ"

type TabsStyle string

const (
	TabsLift   TabsStyle = "tabs-lift"
	TabsBorder TabsStyle = "tabs-border"
	TabsBox    TabsStyle = "tabs-box"
)

type TabsSize string

const (
	TabsXS TabsSize = "tabs-xs"
	TabsSM TabsSize = "tabs-sm"
	TabsMD TabsSize = "tabs-md"
	TabsLG TabsSize = "tabs-lg"
	TabsXL TabsSize = "tabs-xl"
)

type TabMode string

const (
	ServerTab TabMode = ""
	ClientTab TabMode = "client"
)

type TabItem struct {
	Label    string
	Icon     string
	Active   bool
	Disabled bool
	Content  templ.Component
	ColorCSS string
}

type TabsProps struct {
	Style      TabsStyle
	Size       TabsSize
	Bottom     bool
	Items      []TabItem
	Name       string
	ExtraClass string
	Mode       TabMode
	Driver     string // "" = vanilla (HTMX/Alpine via Mode), "stimulus" = StimulusJS
}
