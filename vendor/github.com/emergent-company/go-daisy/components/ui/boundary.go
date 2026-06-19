package ui

import (
	"context"
	"io"

	"github.com/a-h/templ"
	"github.com/emergent-company/go-daisy/devmode"
)

// ButtonWithBoundary wraps Button with a dev-mode component boundary annotation.
// gallery:token variant,size,typ,shape,icon,loading
// gallery:hint href:default(#)
func ButtonWithBoundary(href string, variant ButtonVariant, size ButtonSize, typ ButtonType, shape ButtonShape, icon string, loading bool) templ.Component {
	return devmode.ComponentBoundary("Button", Button(href, variant, size, typ, shape, icon, loading, nil), map[string]any{
		"href":    href,
		"variant": string(variant),
		"size":    string(size),
		"type":    string(typ),
		"shape":   string(shape),
		"icon":    icon,
		"loading": loading,
	})
}

// BadgeWithBoundary wraps Badge with a dev-mode component boundary annotation.
// gallery:token variant,style,size,dot,icon,label
// gallery:hint label:default(Active)
func BadgeWithBoundary(variant BadgeIntent, style BadgeStyle, size BadgeSize, dot bool, icon string, label string) templ.Component {
	props := BadgeProps{Label: label, Variant: variant, Style: style, Size: size, Dot: dot, Icon: icon}
	return devmode.ComponentBoundary("Badge", Badge(props), props)
}

// StatusBadgeWithBoundary wraps StatusBadge with a dev-mode component boundary annotation.
// gallery:token status
// gallery:hint status:default(active)
func StatusBadgeWithBoundary(status string) templ.Component {
	return devmode.ComponentBoundary("StatusBadge", StatusBadge(status), map[string]any{"status": status})
}

// AvatarWithBoundary wraps Avatar with a dev-mode component boundary annotation.
// gallery:token name,icon,size
// gallery:hint name:default(Jane Smith)
// gallery:hint icon:default()
func AvatarWithBoundary(name string, src string, icon string, size AvatarSize) templ.Component {
	return devmode.ComponentBoundary("Avatar", Avatar(name, src, icon, size, nil), map[string]any{
		"name": name,
		"src":  src,
		"icon": icon,
		"size": string(size),
	})
}

// CardWithBoundary wraps Card with a dev-mode component boundary annotation.
// gallery:token title
// gallery:hint title:default(Card Title)
func CardWithBoundary(title string) templ.Component {
	return devmode.ComponentBoundary("Card", Card(title, nil), map[string]any{"title": title})
}

// AlertWithBoundary wraps Alert with a dev-mode component boundary annotation.
// gallery:token typ,icon,message
// gallery:hint message:default(Operation completed successfully.)
// gallery:hint icon:default(lucide--circle-check)
func AlertWithBoundary(typ AlertType, icon string, message string) templ.Component {
	return devmode.ComponentBoundary("Alert", Alert(typ, icon, message, nil), map[string]any{
		"type":    string(typ),
		"icon":    icon,
		"message": message,
	})
}

// AlertWithIconBoundary is a backwards-compatible alias for AlertWithBoundary.
// Deprecated: use AlertWithBoundary directly.
func AlertWithIconBoundary(typ AlertType, icon string, message string) templ.Component {
	return AlertWithBoundary(typ, icon, message)
}

// ToastWithBoundary wraps Toast with a dev-mode component boundary annotation.
// gallery:token typ,message
// gallery:hint message:default(Action completed successfully.)
func ToastWithBoundary(typ ToastType, message string) templ.Component {
	return devmode.ComponentBoundary("Toast", Toast(typ, message), map[string]any{
		"type":    string(typ),
		"message": message,
	})
}

// PaginationWithBoundary wraps Pagination with a dev-mode component boundary annotation.
// gallery:token currentPage,totalPages
// gallery:hint currentPage:range(1,20,1)
// gallery:hint totalPages:range(1,20,1)
func PaginationWithBoundary(currentPage int, totalPages int, baseURL string, targetID string) templ.Component {
	return devmode.ComponentBoundary("Pagination", Pagination(currentPage, totalPages, baseURL, targetID), map[string]any{
		"currentPage": currentPage,
		"totalPages":  totalPages,
		"baseURL":     baseURL,
		"targetID":    targetID,
	})
}

// StatCardWithBoundary wraps StatCard with a dev-mode component boundary annotation.
func StatCardWithBoundary(p StatCardProps) templ.Component {
	return devmode.ComponentBoundary("StatCard", StatCard(p), p)
}

// EmptyWithBoundary wraps Empty with a dev-mode component boundary annotation.
// gallery:token title,description
// gallery:hint title:default(Nothing here yet)
// gallery:hint description:default(Add some items to get started.)
func EmptyWithBoundary(icon string, title string, description string) templ.Component {
	return devmode.ComponentBoundary("Empty", Empty(icon, title, description), map[string]any{
		"icon":        icon,
		"title":       title,
		"description": description,
	})
}

// LoaderWithBoundary wraps Loader with a dev-mode component boundary annotation.
// gallery:token variant
func LoaderWithBoundary(variant LoaderVariant) templ.Component {
	return devmode.ComponentBoundary("Loader", Loader(variant), map[string]any{"variant": string(variant)})
}

// ActionMenuWithBoundary wraps ActionMenu with a dev-mode component boundary annotation.
// gallery:token items
// gallery:hint items:slice(3)
func ActionMenuWithBoundary(items []ActionMenuItem) templ.Component {
	return devmode.ComponentBoundary("ActionMenu", ActionMenu(items), map[string]any{"itemCount": len(items)})
}

// FilterCardWithBoundary wraps FilterCard with a dev-mode component boundary annotation.
func FilterCardWithBoundary(props FilterCardProps) templ.Component {
	return devmode.ComponentBoundary("FilterCard", FilterCard(props), props)
}

// ProgressWithBoundary wraps Progress with a dev-mode component boundary annotation.
// gallery:token color,value,max
// gallery:hint value:range(0,100,1)
// gallery:hint value:default(70)
// gallery:hint max:range(1,200,1)
// gallery:hint max:default(100)
func ProgressWithBoundary(color ProgressColor, value int, max int) templ.Component {
	return devmode.ComponentBoundary("Progress", Progress(color, value, max, nil), map[string]any{
		"color": string(color),
		"value": value,
		"max":   max,
	})
}

// SkeletonWithBoundary wraps Skeleton with a dev-mode component boundary annotation.
// gallery:token classes
// gallery:hint classes:default(h-4 w-full)
func SkeletonWithBoundary(classes string) templ.Component {
	return devmode.ComponentBoundary("Skeleton", Skeleton(classes), map[string]any{"classes": classes})
}

// SectionHeaderWithBoundary wraps SectionHeader with a dev-mode component boundary annotation.
// gallery:token title
// gallery:hint title:default(Personal Information)
func SectionHeaderWithBoundary(title string) templ.Component {
	return devmode.ComponentBoundary("SectionHeader", SectionHeader(title), map[string]any{"title": title})
}

// NoPermissionsWithBoundary wraps NoPermissions with a dev-mode component boundary annotation.
func NoPermissionsWithBoundary() templ.Component {
	return devmode.ComponentBoundary("NoPermissions", NoPermissions())
}

// StatusDotWithBoundary wraps StatusDot with a dev-mode component boundary annotation.
// gallery:token color,animate
// gallery:hint color:default(status-success)
func StatusDotWithBoundary(color StatusColor, animate bool) templ.Component {
	return devmode.ComponentBoundary("StatusDot", StatusDot(color, animate), map[string]any{
		"color":   string(color),
		"animate": animate,
	})
}

// DividerWithBoundary wraps Divider with a dev-mode component boundary annotation.
// gallery:token color,vertical
// gallery:hint color:default()
func DividerWithBoundary(color DividerColor, vertical bool, label string) templ.Component {
	child := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, label)
		return err
	})
	inner := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return Divider(color, vertical).Render(templ.WithChildren(ctx, child), w)
	})
	return devmode.ComponentBoundary("Divider", inner, map[string]any{
		"color":    string(color),
		"vertical": vertical,
		"label":    label,
	})
}

// KbdWithBoundary wraps Kbd with a dev-mode component boundary annotation.
// gallery:token size,key
// gallery:hint key:default(⌘K)
func KbdWithBoundary(size KbdSize, key string) templ.Component {
	child := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, key)
		return err
	})
	inner := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return Kbd(size).Render(templ.WithChildren(ctx, child), w)
	})
	return devmode.ComponentBoundary("Kbd", inner, map[string]any{
		"size": string(size),
		"key":  key,
	})
}

// CountdownWithBoundary wraps Countdown with a dev-mode component boundary annotation.
// gallery:token days,hours,minutes,seconds
// gallery:hint days:range(0,99,1)
// gallery:hint hours:range(0,23,1)
// gallery:hint minutes:range(0,59,1)
// gallery:hint seconds:range(0,59,1)
// gallery:hint days:default(2)
// gallery:hint hours:default(10)
// gallery:hint minutes:default(24)
// gallery:hint seconds:default(45)
func CountdownWithBoundary(days, hours, minutes, seconds int) templ.Component {
	return devmode.ComponentBoundary("Countdown", Countdown(days, hours, minutes, seconds), map[string]any{
		"days":    days,
		"hours":   hours,
		"minutes": minutes,
		"seconds": seconds,
	})
}

// TagWithBoundary wraps Tag with a dev-mode component boundary annotation.
// gallery:token label
// gallery:hint label:default(Contract Law)
func TagWithBoundary(label string, removeHref string) templ.Component {
	return devmode.ComponentBoundary("Tag", Tag(label, removeHref), map[string]any{
		"label":      label,
		"removeHref": removeHref,
	})
}

// ChatBubbleWithBoundary wraps ChatBubble with a dev-mode component boundary annotation.
// gallery:token sent,author,timestamp,message
// gallery:hint author:default(Alice)
// gallery:hint timestamp:default(10:32 AM)
// gallery:hint message:default(Hey! How are you doing?)
func ChatBubbleWithBoundary(sent bool, author, timestamp, avatarSrc string, botIcon bool, bubbleClass string, showActions bool, message string) templ.Component {
	child := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, message)
		return err
	})
	inner := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return ChatBubble(sent, author, timestamp, avatarSrc, botIcon, bubbleClass, showActions, nil).Render(templ.WithChildren(ctx, child), w)
	})
	return devmode.ComponentBoundary("ChatBubble", inner, map[string]any{
		"sent":        sent,
		"author":      author,
		"timestamp":   timestamp,
		"avatarSrc":   avatarSrc,
		"botIcon":     botIcon,
		"bubbleClass": bubbleClass,
		"showActions": showActions,
		"message":     message,
	})
}

// AIThinkingIndicatorWithBoundary wraps AIThinkingIndicator with a dev-mode component boundary annotation.
func AIThinkingIndicatorWithBoundary() templ.Component {
	return devmode.ComponentBoundary("AIThinkingIndicator", AIThinkingIndicator(), nil)
}

// ChatWindowWithBoundary wraps ChatWindow with a dev-mode component boundary annotation.
func ChatWindowWithBoundary(heightClass string, children templ.Component) templ.Component {
	inner := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return ChatWindow(heightClass, nil).Render(templ.WithChildren(ctx, children), w)
	})
	return devmode.ComponentBoundary("ChatWindow", inner, map[string]any{"heightClass": heightClass})
}

// ChatInputWithBoundary wraps ChatInput with a dev-mode component boundary annotation.
// gallery:token placeholder
// gallery:hint placeholder:default(Type a message...)
func ChatInputWithBoundary(showAttach bool, placeholder string) templ.Component {
	return devmode.ComponentBoundary("ChatInput", ChatInput(showAttach, placeholder, nil), map[string]any{
		"showAttach":  showAttach,
		"placeholder": placeholder,
	})
}

// MockupBrowserWithBoundary wraps MockupBrowser with a dev-mode component boundary annotation.
// gallery:token url
// gallery:hint url:default(https://go-daisy.dev)
func MockupBrowserWithBoundary(url string) templ.Component {
	inner := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return MockupBrowser(url).Render(templ.WithChildren(ctx, MockupBrowserPlaceholder()), w)
	})
	return devmode.ComponentBoundary("MockupBrowser", inner, map[string]any{"url": url})
}

// MockupPhoneWithBoundary wraps MockupPhone with a dev-mode component boundary annotation.
func MockupPhoneWithBoundary() templ.Component {
	inner := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return MockupPhone().Render(templ.WithChildren(ctx, MockupPhonePlaceholder()), w)
	})
	return devmode.ComponentBoundary("MockupPhone", inner)
}

// MockupWindowWithBoundary wraps MockupWindow with a dev-mode component boundary annotation.
func MockupWindowWithBoundary() templ.Component {
	inner := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return MockupWindow().Render(templ.WithChildren(ctx, MockupWindowPlaceholder()), w)
	})
	return devmode.ComponentBoundary("MockupWindow", inner)
}

// AccordionWithBoundary wraps Accordion + AccordionItem with a dev-mode component boundary annotation.
func AccordionWithBoundary(items []AccordionItemProps) templ.Component {
	children := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		for _, item := range items {
			it := item
			inner := templ.ComponentFunc(func(ctx2 context.Context, w2 io.Writer) error {
				return AccordionItem(it.Title, it.Open).Render(templ.WithChildren(ctx2, it.Content), w2)
			})
			if err := inner.Render(ctx, w); err != nil {
				return err
			}
		}
		return nil
	})
	outer := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return Accordion().Render(templ.WithChildren(ctx, children), w)
	})
	return devmode.ComponentBoundary("Accordion", outer, map[string]any{"itemCount": len(items)})
}

// AccordionItemProps holds props for a single accordion item.
type AccordionItemProps struct {
	Title   string
	Content templ.Component
	Open    bool
}

// StepsWithBoundary wraps Steps + Step with a dev-mode component boundary annotation.
// gallery:token steps
func StepsWithBoundary(steps []StepProps) templ.Component {
	children := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		for _, s := range steps {
			if err := Step(s.Label, s.Done).Render(ctx, w); err != nil {
				return err
			}
		}
		return nil
	})
	outer := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return Steps().Render(templ.WithChildren(ctx, children), w)
	})
	return devmode.ComponentBoundary("Steps", outer, map[string]any{"stepCount": len(steps)})
}

// StepProps holds props for a single step.
type StepProps struct {
	Label string
	Done  bool
}

// SwapWithBoundary wraps Swap with a dev-mode component boundary annotation.
// gallery:token rotate
func SwapWithBoundary(rotate bool, onContent templ.Component, offContent templ.Component) templ.Component {
	return devmode.ComponentBoundary("Swap", Swap(rotate, onContent, offContent), map[string]any{
		"rotate": rotate,
	})
}

// HeroWithBoundary wraps Hero + HeroContent with a dev-mode component boundary annotation.
// gallery:token title,subtitle,ctaLabel,minHeight
// gallery:hint title:default(go-daisy)
// gallery:hint subtitle:default(Type-safe Templ components styled with DaisyUI for HTMX apps.)
// gallery:hint ctaLabel:default(Get Started)
// gallery:hint minHeight:default(min-h-56)
func HeroWithBoundary(minHeight string, title string, subtitle string, ctaLabel string) templ.Component {
	body := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return HeroBody(title, subtitle, ctaLabel).Render(ctx, w)
	})
	content := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return HeroContent(true).Render(templ.WithChildren(ctx, body), w)
	})
	outer := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return HeroSection(minHeight).Render(templ.WithChildren(ctx, content), w)
	})
	return devmode.ComponentBoundary("Hero", outer, map[string]any{
		"title":     title,
		"subtitle":  subtitle,
		"ctaLabel":  ctaLabel,
		"minHeight": minHeight,
	})
}

// TooltipWithBoundary wraps Tooltip with a dev-mode component boundary annotation.
// gallery:token tip,position
// gallery:hint tip:default(Helpful hint)
// gallery:hint position:default()
func TooltipWithBoundary(tip string, position string, trigger templ.Component) templ.Component {
	inner := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return TooltipPositioned(tip, position).Render(templ.WithChildren(ctx, trigger), w)
	})
	return devmode.ComponentBoundary("Tooltip", inner, map[string]any{
		"tip":      tip,
		"position": position,
	})
}

// DropdownWithBoundary wraps Dropdown with a dev-mode component boundary annotation.
// gallery:token align
// gallery:hint align:default()
func DropdownWithBoundary(align DropdownAlign, trigger templ.Component, items []DropdownItemProps) templ.Component {
	content := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if err := trigger.Render(ctx, w); err != nil {
			return err
		}
		menu := templ.ComponentFunc(func(ctx2 context.Context, w2 io.Writer) error {
			for _, item := range items {
				if item.Divider {
					if _, err := io.WriteString(w2, `<li class="divider my-0.5"></li>`); err != nil {
						return err
					}
					continue
				}
				it := item
				li := templ.ComponentFunc(func(_ context.Context, w3 io.Writer) error {
					_, err := io.WriteString(w3, it.Label)
					return err
				})
				if err := DropdownItem(false, it.Danger, nil).Render(templ.WithChildren(ctx2, li), w2); err != nil {
					return err
				}
			}
			return nil
		})
		return DropdownMenu(nil).Render(templ.WithChildren(ctx, menu), w)
	})
	outer := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return Dropdown(align, nil).Render(templ.WithChildren(ctx, content), w)
	})
	return devmode.ComponentBoundary("Dropdown", outer, map[string]any{
		"align": string(align),
	})
}

// DropdownItemProps holds props for a single dropdown menu item.
type DropdownItemProps struct {
	Label   string
	Divider bool
	Danger  bool
}

// JoinWithBoundary wraps Join with a dev-mode component boundary annotation.
// gallery:token vertical
func JoinWithBoundary(vertical bool, children ...templ.Component) templ.Component {
	content := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		for _, c := range children {
			if err := c.Render(ctx, w); err != nil {
				return err
			}
		}
		return nil
	})
	outer := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return Join(vertical).Render(templ.WithChildren(ctx, content), w)
	})
	return devmode.ComponentBoundary("Join", outer, map[string]any{"vertical": vertical})
}

// IndicatorWithBoundary wraps IndicatorWrapper with a dev-mode component boundary annotation.
func IndicatorWithBoundary(badgeClass string, badgeContent templ.Component, content templ.Component) templ.Component {
	inner := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		badge := templ.ComponentFunc(func(ctx2 context.Context, w2 io.Writer) error {
			return badgeContent.Render(ctx2, w2)
		})
		if err := IndicatorBadge("", badgeClass).Render(templ.WithChildren(ctx, badge), w); err != nil {
			return err
		}
		return content.Render(ctx, w)
	})
	outer := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return IndicatorWrapper().Render(templ.WithChildren(ctx, inner), w)
	})
	return devmode.ComponentBoundary("Indicator", outer, map[string]any{
		"badgeClass": badgeClass,
	})
}

// StackWithBoundary wraps Stack with a dev-mode component boundary annotation.
func StackWithBoundary(children ...templ.Component) templ.Component {
	content := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		for _, c := range children {
			if err := c.Render(ctx, w); err != nil {
				return err
			}
		}
		return nil
	})
	outer := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return Stack().Render(templ.WithChildren(ctx, content), w)
	})
	return devmode.ComponentBoundary("Stack", outer)
}

// DiffWithBoundary wraps Diff with a dev-mode component boundary annotation.
// gallery:token before,after
// gallery:hint before:default(Before: Old content here)
// gallery:hint after:default(After: New content here)
func DiffWithBoundary(before string, after string) templ.Component {
	inner := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if err := DiffItem1().Render(templ.WithChildren(ctx, DiffItemContent(before, false)), w); err != nil {
			return err
		}
		if err := DiffItem2().Render(templ.WithChildren(ctx, DiffItemContent(after, true)), w); err != nil {
			return err
		}
		return DiffResizer().Render(ctx, w)
	})
	outer := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return DiffContainer().Render(templ.WithChildren(ctx, inner), w)
	})
	return devmode.ComponentBoundary("Diff", outer, map[string]any{
		"before": before,
		"after":  after,
	})
}

// MaskWithBoundary wraps Mask with a dev-mode component boundary annotation.
// gallery:token shape
// gallery:hint shape:default(mask-squircle)
func MaskWithBoundary(shape MaskShape, content templ.Component) templ.Component {
	outer := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return Mask(shape).Render(templ.WithChildren(ctx, content), w)
	})
	return devmode.ComponentBoundary("Mask", outer, map[string]any{"shape": string(shape)})
}

// CarouselWithBoundary wraps Carousel with a dev-mode component boundary annotation.
// gallery:token snap,vertical,width
func CarouselWithBoundary(snap CarouselSnap, vertical bool, width string, items []CarouselItemProps) templ.Component {
	children := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		for _, item := range items {
			it := item
			inner := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
				return CarouselItem(it.ID, it.ItemWidth).Render(templ.WithChildren(ctx, it.Content), w)
			})
			itemBoundary := devmode.ComponentBoundary("CarouselItem", inner, map[string]any{"id": it.ID, "itemWidth": it.ItemWidth})
			if err := itemBoundary.Render(ctx, w); err != nil {
				return err
			}
		}
		return nil
	})
	outer := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return Carousel(snap, vertical, width).Render(templ.WithChildren(ctx, children), w)
	})
	return devmode.ComponentBoundary("Carousel", outer, map[string]any{"snap": string(snap), "vertical": vertical, "width": width, "itemCount": len(items)})
}

// CarouselItemProps holds props for a single carousel slide.
type CarouselItemProps struct {
	ID        string
	ItemWidth string // optional Tailwind width class, e.g. "w-full", "w-1/2"
	Content   templ.Component
}

// TimelineWithBoundary wraps Timeline with a dev-mode component boundary annotation.
func TimelineWithBoundary(items []TimelineItemProps) templ.Component {
	inner := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		for i, item := range items {
			isLast := i == len(items)-1
			if err := TimelineItem(item, i == 0, isLast).Render(ctx, w); err != nil {
				return err
			}
		}
		return nil
	})
	outer := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return Timeline().Render(templ.WithChildren(ctx, inner), w)
	})
	return devmode.ComponentBoundary("Timeline", outer, map[string]any{"itemCount": len(items)})
}

// MockupCodeWithBoundary wraps MockupCode with a dev-mode component boundary annotation.
func MockupCodeWithBoundary(lines []MockupCodeLineProps) templ.Component {
	children := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		for _, line := range lines {
			text := templ.ComponentFunc(func(_ context.Context, w2 io.Writer) error {
				_, err := io.WriteString(w2, line.Code)
				return err
			})
			if err := MockupCodeLine(line.Prefix, line.ColorClass).Render(templ.WithChildren(ctx, text), w); err != nil {
				return err
			}
		}
		return nil
	})
	outer := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return MockupCode().Render(templ.WithChildren(ctx, children), w)
	})
	return devmode.ComponentBoundary("MockupCode", outer, map[string]any{"lineCount": len(lines)})
}

// MockupCodeLineProps holds props for a single code mockup line.
type MockupCodeLineProps struct {
	Prefix     string
	Code       string
	ColorClass string
}

// ListWithBoundary wraps List with a dev-mode component boundary annotation.
func ListWithBoundary(props ListProps, items []ListRowProps) templ.Component {
	children := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		for _, item := range items {
			if err := ListRow(item).Render(ctx, w); err != nil {
				return err
			}
		}
		return nil
	})
	outer := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return List(props).Render(templ.WithChildren(ctx, children), w)
	})
	return devmode.ComponentBoundary("List", outer, map[string]any{"itemCount": len(items), "header": props.Header})
}

// FilterTabsWithBoundary wraps FilterTabs with a dev-mode component boundary annotation.
// gallery:token selected
func FilterTabsWithBoundary(name string, selected string, tabs []string) templ.Component {
	return devmode.ComponentBoundary("FilterTabs", FilterTabs(name, selected, tabs), map[string]any{
		"name":     name,
		"selected": selected,
	})
}

// FieldsetWithBoundary wraps Fieldset with a dev-mode component boundary annotation.
// gallery:token legend
// gallery:hint legend:default(Account Settings)
func FieldsetWithBoundary(legend string, content templ.Component) templ.Component {
	outer := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return Fieldset(legend).Render(templ.WithChildren(ctx, content), w)
	})
	return devmode.ComponentBoundary("Fieldset", outer, map[string]any{"legend": legend})
}

// ProgressCardWithBoundary wraps ProgressCard with a dev-mode component boundary annotation.
func ProgressCardWithBoundary(props ProgressCardProps) templ.Component {
	return devmode.ComponentBoundary("ProgressCard", ProgressCard(props), props)
}

// StatCardMinimalWithBoundary wraps StatCardMinimal with a dev-mode component boundary annotation.
// gallery:token label,value,trend,trendLabel
// gallery:hint label:default(Total Users)
// gallery:hint value:default(12,430)
func StatCardMinimalWithBoundary(item StatCardMinimalItem) templ.Component {
	return devmode.ComponentBoundary("StatCardMinimal", StatCardMinimal(item), map[string]any{
		"label":      item.Label,
		"value":      item.Value,
		"trend":      string(item.Trend),
		"trendLabel": item.TrendLabel,
	})
}

// StatCardIconCornerWithBoundary wraps StatCardMinimal (icon-corner style) with a dev-mode component boundary annotation.
// Deprecated: use StatCardMinimalWithBoundary with Icon/IconColor set instead.
// gallery:token label,value,icon,iconColor,trend,trendLabel
// gallery:hint label:default(Revenue)
// gallery:hint value:default($48,290)
func StatCardIconCornerWithBoundary(item StatCardIconCornerItem) templ.Component {
	return devmode.ComponentBoundary("StatCardMinimal", StatCardMinimal(item), map[string]any{
		"label":      item.Label,
		"value":      item.Value,
		"icon":       item.Icon,
		"iconColor":  item.IconColor,
		"trend":      string(item.Trend),
		"trendLabel": item.TrendLabel,
	})
}

// PersonCellWithBoundary wraps PersonCell with a dev-mode component boundary annotation.
// gallery:token name,subtitle,size
// gallery:hint name:default(Alice Johnson)
// gallery:hint subtitle:default(alice@example.com)
func PersonCellWithBoundary(p PersonCellProps) templ.Component {
	return devmode.ComponentBoundary("PersonCell", PersonCell(p), map[string]any{
		"name":     p.Name,
		"subtitle": p.Subtitle,
		"src":      p.Src,
		"icon":     p.Icon,
		"size":     string(p.Size),
	})
}

// PersonChipWithBoundary wraps PersonChip with a dev-mode component boundary annotation.
// gallery:token name,avatarColor,textColor
// gallery:hint name:default(Jane Smith)
func PersonChipWithBoundary(name string, avatarColor string, textColor string, gradientFrom string, gradientTo string, contact PersonChipContact) templ.Component {
	return devmode.ComponentBoundary("PersonChip", PersonChip(name, avatarColor, textColor, gradientFrom, gradientTo, contact), map[string]any{
		"name":         name,
		"avatarColor":  avatarColor,
		"textColor":    textColor,
		"gradientFrom": gradientFrom,
		"gradientTo":   gradientTo,
	})
}

// NotificationRowWithBoundary wraps NotificationRow with a dev-mode component boundary annotation.
func NotificationRowWithBoundary(item NotificationItem) templ.Component {
	return devmode.ComponentBoundary("NotificationRow", NotificationRow(item), map[string]any{
		"title":  item.Title,
		"unread": item.Unread,
	})
}

// NotificationPanelWithBoundary wraps NotificationPanel with a dev-mode component boundary annotation.
// It renders items as children so the panel can also be composed manually via children slot.
func NotificationPanelWithBoundary(items []NotificationItem, unreadCount int, viewAllHref string) templ.Component {
	children := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		for _, item := range items {
			if err := NotificationRow(item).Render(ctx, w); err != nil {
				return err
			}
		}
		return nil
	})
	outer := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return NotificationPanel(unreadCount, viewAllHref).Render(templ.WithChildren(ctx, children), w)
	})
	return devmode.ComponentBoundary("NotificationPanel", outer, map[string]any{
		"itemCount":   len(items),
		"unreadCount": unreadCount,
		"viewAllHref": viewAllHref,
	})
}

// FABWithBoundary wraps FAB with a dev-mode component boundary annotation.
func FABWithBoundary(icon string, actions []FABAction) templ.Component {
	return devmode.ComponentBoundary("FAB", FAB(icon, actions), map[string]any{
		"icon":        icon,
		"actionCount": len(actions),
	})
}

// IconSpanColoredWithBoundary wraps IconSpanColored with a dev-mode component boundary annotation.
func IconSpanColoredWithBoundary(name string, size string, color string) templ.Component {
	return devmode.ComponentBoundary("IconSpanColored", IconSpanColored(name, size, color), map[string]any{
		"name":  name,
		"size":  size,
		"color": color,
	})
}

// RadialProgressWithBoundary wraps RadialProgress with a dev-mode component boundary annotation.
func RadialProgressWithBoundary(color ProgressColor, value int, size string, thickness string) templ.Component {
	return devmode.ComponentBoundary("RadialProgress", RadialProgress(color, value, size, thickness), map[string]any{
		"color":     string(color),
		"value":     value,
		"size":      size,
		"thickness": thickness,
	})
}

// DrawerWithBoundary wraps Drawer with a dev-mode component boundary annotation.
func DrawerWithBoundary(id string, side DrawerSide, content templ.Component, sidebarContent templ.Component, sidebarWidth string) templ.Component {
	return devmode.ComponentBoundary("Drawer", Drawer(id, side, content, sidebarContent, sidebarWidth), map[string]any{
		"id":           id,
		"side":         string(side),
		"sidebarWidth": sidebarWidth,
	})
}

// DrawerToggleWithBoundary wraps DrawerToggle with a dev-mode component boundary annotation.
func DrawerToggleWithBoundary(drawerID string, label string, variant string) templ.Component {
	return devmode.ComponentBoundary("DrawerToggle", DrawerToggle(drawerID, label, variant), map[string]any{
		"drawerID": drawerID,
		"label":    label,
		"variant":  variant,
	})
}

// ThemeControllerWithBoundary wraps ThemeController with a dev-mode component boundary annotation.
func ThemeControllerWithBoundary(theme string, inputType ThemeInputType, label string, checked bool) templ.Component {
	return devmode.ComponentBoundary("ThemeController", ThemeController(theme, inputType, label, checked), map[string]any{
		"theme":     theme,
		"inputType": string(inputType),
		"label":     label,
		"checked":   checked,
	})
}

// ThemeControllerBtnWithBoundary wraps ThemeControllerBtn with a dev-mode component boundary annotation.
func ThemeControllerBtnWithBoundary(theme string, checked bool) templ.Component {
	return devmode.ComponentBoundary("ThemeControllerBtn", ThemeControllerBtn(theme, checked), map[string]any{
		"theme":   theme,
		"checked": checked,
	})
}

// TextRotateWithBoundary wraps TextRotate with a dev-mode component boundary annotation.
func TextRotateWithBoundary(items []string, duration string) templ.Component {
	return devmode.ComponentBoundary("TextRotate", TextRotate(items, duration), map[string]any{
		"itemCount": len(items),
		"duration":  duration,
	})
}

// Hover3DCardWithBoundary wraps Hover3DCard with a dev-mode component boundary annotation.
func Hover3DCardWithBoundary(extraClass string, children templ.Component) templ.Component {
	inner := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return Hover3DCard(extraClass).Render(templ.WithChildren(ctx, children), w)
	})
	return devmode.ComponentBoundary("Hover3DCard", inner, map[string]any{"extraClass": extraClass})
}

// HoverGalleryWithBoundary wraps HoverGallery with a dev-mode component boundary annotation.
func HoverGalleryWithBoundary(images []HoverGalleryImage) templ.Component {
	return devmode.ComponentBoundary("HoverGallery", HoverGallery(images), map[string]any{
		"imageCount": len(images),
	})
}
