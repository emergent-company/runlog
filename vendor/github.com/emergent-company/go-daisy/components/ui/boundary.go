package ui

import (
	"context"
	"io"

	"github.com/a-h/templ"
	"github.com/emergent-company/go-daisy/components/alpine"
	"github.com/emergent-company/go-daisy/devmode"
	"github.com/emergent-company/go-daisy/shared"
)

// ButtonWithBoundary wraps Button with a dev-mode component boundary annotation.
// gallery:token variant,size,style,typ,shape,icon,loading,block,glow
// gallery:hint href:default(#)
func ButtonWithBoundary(href string, variant ButtonVariant, size ButtonSize, style ButtonStyle, typ ButtonType, shape ButtonShape, icon string, loading bool, block bool, glow bool) templ.Component {
	props := ButtonProps{
		Href:    href,
		Variant: variant,
		Size:    size,
		Style:   style,
		Type:    typ,
		Shape:   shape,
		Icon:    icon,
		Loading: loading,
		Block:   block,
		Glow:    glow,
	}
	return devmode.ComponentBoundary("Button", Button(props), props)
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
	return devmode.ComponentBoundary("Avatar", Avatar(name, src, icon, size, "", nil), map[string]any{
		"name": name,
		"src":  src,
		"icon": icon,
		"size": string(size),
	})
}

// AvatarFullWithBoundary wraps AvatarFull with a dev-mode component boundary annotation.
func AvatarFullWithBoundary(props AvatarProps) templ.Component {
	return devmode.ComponentBoundary("Avatar", AvatarFull(props), map[string]any{
		"name":     props.Name,
		"src":      props.Src,
		"icon":     props.Icon,
		"size":     string(props.Size),
		"tone":     string(props.Tone),
		"fallback": string(props.Fallback),
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
	return devmode.ComponentBoundary("Alert", Alert(AlertProps{Type: typ, Icon: icon, Message: message}), map[string]any{
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

// AlertStyledWithBoundary wraps AlertStyled with a dev-mode component boundary annotation.
// gallery:token typ,style,icon,message
// gallery:hint message:default(New software update available.)
// gallery:hint icon:default(lucide--info)
func AlertStyledWithBoundary(typ AlertType, style AlertStyle, icon string, message string) templ.Component {
	return devmode.ComponentBoundary("AlertStyled", AlertStyled(typ, style, icon, message, nil), map[string]any{
		"type":    string(typ),
		"style":   string(style),
		"icon":    icon,
		"message": message,
	})
}

// ToastWithBoundary wraps Toast with a dev-mode component boundary annotation.
// gallery:token typ,message,driver
// gallery:hint message:default(Action completed successfully.)
// gallery:hint driver:default()
func ToastWithBoundary(typ ToastType, message string, driver string) templ.Component {
	return devmode.ComponentBoundary("Toast", Toast(ToastProps{Type: typ, Message: message, Driver: driver}), map[string]any{
		"type":    string(typ),
		"message": message,
		"driver":  driver,
	})
}

// ToastQueueWithBoundary wraps ToastQueue with a dev-mode component boundary annotation.
func ToastQueueWithBoundary(seed ...alpine.ToastItem) templ.Component {
	return devmode.ComponentBoundary("ToastQueue", ToastQueue(seed...))
}

// BannerWithBoundary wraps Banner with a dev-mode component boundary annotation.
// gallery:token variant,persistent,cookieBanner
func BannerWithBoundary(props BannerProps) templ.Component {
	return devmode.ComponentBoundary("Banner", Banner(props), map[string]any{
		"variant":    string(props.Variant),
		"persistent": props.Persistent,
		"cookie":     props.CookieBanner,
	})
}

// SkeletonCardWithBoundary wraps SkeletonCard.
func SkeletonCardWithBoundary() templ.Component {
	return devmode.ComponentBoundary("SkeletonCard", SkeletonCard())
}

// SkeletonTableRowWithBoundary wraps SkeletonTableRow.
// gallery:hint cols:range(2,8,1)
func SkeletonTableRowWithBoundary(cols int) templ.Component {
	return devmode.ComponentBoundary("SkeletonTableRow", SkeletonTableRow(cols), map[string]any{
		"cols": cols,
	})
}

// SkeletonTextWithBoundary wraps SkeletonText.
func SkeletonTextWithBoundary(widths ...string) templ.Component {
	return devmode.ComponentBoundary("SkeletonText", SkeletonText(widths...), map[string]any{
		"lines": len(widths),
	})
}

// SkeletonAvatarWithBoundary wraps SkeletonAvatar.
// gallery:token size
// gallery:hint size:default(size-10)
func SkeletonAvatarWithBoundary(size string) templ.Component {
	return devmode.ComponentBoundary("SkeletonAvatar", SkeletonAvatar(size), map[string]any{
		"size": size,
	})
}

// SkeletonButtonWithBoundary wraps SkeletonButton.
func SkeletonButtonWithBoundary() templ.Component {
	return devmode.ComponentBoundary("SkeletonButton", SkeletonButton())
}

// SkeletonStatsWithBoundary wraps SkeletonStats.
func SkeletonStatsWithBoundary() templ.Component {
	return devmode.ComponentBoundary("SkeletonStats", SkeletonStats())
}

// SkeletonH1WithBoundary wraps SkeletonH1.
func SkeletonH1WithBoundary() templ.Component {
	return devmode.ComponentBoundary("SkeletonH1", SkeletonH1())
}

// SkeletonFormFieldWithBoundary wraps SkeletonFormField.
func SkeletonFormFieldWithBoundary() templ.Component {
	return devmode.ComponentBoundary("SkeletonFormField", SkeletonFormField())
}

// CodeBlockWithBoundary wraps CodeBlock with a dev-mode component boundary annotation.
// gallery:token language,label
// gallery:hint language:default(go)
// gallery:hint label:default(main.go)
func CodeBlockWithBoundary(props CodeBlockProps) templ.Component {
	return devmode.ComponentBoundary("CodeBlock", CodeBlock(props), map[string]any{
		"language": props.Language,
		"label":    props.Label,
	})
}

// PaginationWithBoundary wraps PaginationWithProps with a dev-mode component boundary annotation.
// gallery:token currentPage,totalPages,style
// gallery:hint currentPage:range(1,20,1)
// gallery:hint totalPages:range(1,20,1)
func PaginationWithBoundary(currentPage int, totalPages int, baseURL string, targetID string, style PaginationStyle) templ.Component {
	props := PaginationProps{
		CurrentPage: currentPage,
		TotalPages:  totalPages,
		BaseURL:     baseURL,
		TargetID:    targetID,
		Style:       style,
	}
	return devmode.ComponentBoundary("Pagination", PaginationWithProps(props), props)
}

// PaginationCircleWithBoundary is a deprecated alias for PaginationWithBoundary with circle style.
// Deprecated: use PaginationWithBoundary(..., PaginationStyleCircle) instead.
func PaginationCircleWithBoundary(currentPage int, totalPages int, baseURL string, targetID string) templ.Component {
	return PaginationWithBoundary(currentPage, totalPages, baseURL, targetID, PaginationStyleCircle)
}

// StatCardWithBoundary wraps StatCard with a dev-mode component boundary annotation.
func StatCardWithBoundary(p StatCardProps) templ.Component {
	return devmode.ComponentBoundary("StatCard", StatCard(p), p)
}

// StatCardFeaturedWithBoundary wraps StatCardFeatured with a dev-mode component boundary annotation.
func StatCardFeaturedWithBoundary(p StatCardFeaturedProps) templ.Component {
	return devmode.ComponentBoundary("StatCardFeatured", StatCardFeatured(p), p)
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

// EmptyStateWithBoundary wraps EmptyState with a dev-mode component boundary annotation.
// gallery:token variant,iconStyle,heading,bordered,title,description,actionlabel,actionicon
// gallery:hint variant:default(hero)
// gallery:hint iconStyle:default(tile)
// gallery:hint heading:default(h2)
// gallery:hint title:default(Connect your first provider)
// gallery:hint description:default(LLM providers power your agents — add one to get started.)
// gallery:hint actionlabel:default(Add your first provider)
// gallery:hint actionicon:default(lucide--plus)
// gallery:hint actionhref:default(#)
func EmptyStateWithBoundary(props EmptyStateProps) templ.Component {
	return devmode.ComponentBoundary("EmptyState", EmptyState(props), map[string]any{
		"icon":        props.Icon,
		"title":       props.Title,
		"description": props.Description,
		"variant":     string(props.Variant),
		"iconStyle":   string(props.IconStyle),
		"heading":     string(props.Heading),
		"bordered":    props.Bordered,
		"actionHref":  props.ActionHref,
		"actionLabel": props.ActionLabel,
		"actionIcon":  props.ActionIcon,
		"class":       props.Class,
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
	inner := shared.RenderInto(Divider(color, vertical), shared.StrComp(label))
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
	inner := shared.RenderInto(Kbd(size), shared.StrComp(key))
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
	inner := shared.RenderInto(ChatBubble(sent, author, timestamp, avatarSrc, botIcon, bubbleClass, showActions, nil), shared.StrComp(message))
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
	inner := shared.RenderInto(ChatWindow(heightClass, nil), children)
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
	inner := shared.RenderInto(MockupBrowser(url), MockupBrowserPlaceholder())
	return devmode.ComponentBoundary("MockupBrowser", inner, map[string]any{"url": url})
}

// MockupPhoneWithBoundary wraps MockupPhone with a dev-mode component boundary annotation.
func MockupPhoneWithBoundary() templ.Component {
	inner := shared.RenderInto(MockupPhone(), MockupPhonePlaceholder())
	return devmode.ComponentBoundary("MockupPhone", inner)
}

// MockupWindowWithBoundary wraps MockupWindow with a dev-mode component boundary annotation.
func MockupWindowWithBoundary() templ.Component {
	inner := shared.RenderInto(MockupWindow(), MockupWindowPlaceholder())
	return devmode.ComponentBoundary("MockupWindow", inner)
}

// AccordionWithBoundary wraps Accordion + AccordionItem with a dev-mode component boundary annotation.
func AccordionWithBoundary(items []AccordionItemProps) templ.Component {
	children := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		for _, item := range items {
			it := item
			inner := shared.RenderInto(AccordionItem(it.Title, it.Open), it.Content)
			if err := inner.Render(ctx, w); err != nil {
				return err
			}
		}
		return nil
	})
	outer := shared.RenderInto(Accordion(), children)
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
	outer := shared.RenderInto(Steps(), children)
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
	body := HeroBody(title, subtitle, ctaLabel)
	content := shared.RenderInto(HeroContent(true), body)
	outer := shared.RenderInto(HeroSection(minHeight), content)
	return devmode.ComponentBoundary("Hero", outer, map[string]any{
		"title":     title,
		"subtitle":  subtitle,
		"ctaLabel":  ctaLabel,
		"minHeight": minHeight,
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
				li := shared.StrComp(it.Label)
				if err := DropdownItem(false, it.Danger, nil).Render(templ.WithChildren(ctx2, li), w2); err != nil {
					return err
				}
			}
			return nil
		})
		return DropdownMenu(nil).Render(templ.WithChildren(ctx, menu), w)
	})
	outer := shared.RenderInto(Dropdown(align, nil), content)
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
	outer := shared.RenderInto(Join(vertical), content)
	return devmode.ComponentBoundary("Join", outer, map[string]any{"vertical": vertical})
}

// IndicatorWithBoundary wraps IndicatorWrapper with a dev-mode component boundary annotation.
func IndicatorWithBoundary(badgeClass string, badgeContent templ.Component, content templ.Component) templ.Component {
	inner := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if err := IndicatorBadge("", badgeClass).Render(templ.WithChildren(ctx, badgeContent), w); err != nil {
			return err
		}
		return content.Render(ctx, w)
	})
	outer := shared.RenderInto(IndicatorWrapper(), inner)
	return devmode.ComponentBoundary("IndicatorWrapper", outer, map[string]any{
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
	outer := shared.RenderInto(Stack(), content)
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
	outer := shared.RenderInto(DiffContainer(), inner)
	return devmode.ComponentBoundary("Diff", outer, map[string]any{
		"before": before,
		"after":  after,
	})
}

// MaskWithBoundary wraps Mask with a dev-mode component boundary annotation.
// gallery:token shape
// gallery:hint shape:default(mask-squircle)
func MaskWithBoundary(shape MaskShape, content templ.Component) templ.Component {
	outer := shared.RenderInto(Mask(shape), content)
	return devmode.ComponentBoundary("Mask", outer, map[string]any{"shape": string(shape)})
}

// CarouselWithBoundary wraps Carousel with a dev-mode component boundary annotation.
// gallery:token snap,vertical,width
func CarouselWithBoundary(snap CarouselSnap, vertical bool, width string, items []CarouselItemProps) templ.Component {
	children := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		for _, item := range items {
			it := item
			inner := shared.RenderInto(CarouselItem(it.ID, it.ItemWidth), it.Content)
			itemBoundary := devmode.ComponentBoundary("CarouselItem", inner, map[string]any{"id": it.ID, "itemWidth": it.ItemWidth})
			if err := itemBoundary.Render(ctx, w); err != nil {
				return err
			}
		}
		return nil
	})
	outer := shared.RenderInto(Carousel(snap, vertical, width), children)
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
	outer := shared.RenderInto(Timeline(), inner)
	return devmode.ComponentBoundary("Timeline", outer, map[string]any{"itemCount": len(items)})
}

// MockupCodeWithBoundary wraps MockupCode with a dev-mode component boundary annotation.
func MockupCodeWithBoundary(lines []MockupCodeLineProps) templ.Component {
	children := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		for _, line := range lines {
			text := shared.StrComp(line.Code)
			if err := MockupCodeLine(line.Prefix, line.ColorClass).Render(templ.WithChildren(ctx, text), w); err != nil {
				return err
			}
		}
		return nil
	})
	outer := shared.RenderInto(MockupCode(), children)
	return devmode.ComponentBoundary("MockupCode", outer, map[string]any{"lineCount": len(lines)})
}

// MockupCodeLineProps holds props for a single code mockup line.
type MockupCodeLineProps struct {
	Prefix     string
	Code       string
	ColorClass string
}

// ListWithBoundary wraps List with a dev-mode component boundary annotation.
// gallery:token layout
// gallery:hint layout:default(default)
func ListWithBoundary(props ListProps, items []ListRowProps) templ.Component {
	children := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		for _, item := range items {
			if err := ListRow(item).Render(ctx, w); err != nil {
				return err
			}
		}
		return nil
	})
	outer := shared.RenderInto(List(props), children)
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
	outer := shared.RenderInto(Fieldset(legend), content)
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
		"label":           item.Label,
		"value":           item.Value,
		"trend":           string(item.Trend),
		"trendLabel":      item.TrendLabel,
		"comparisonLabel": item.ComparisonLabel,
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
// Deprecated: use PersonCellWithBoundary with PersonCellProps{Compact: true} instead.
// gallery:token name,avatarColor,textColor
// gallery:hint name:default(Jane Smith)
func PersonChipWithBoundary(name string, avatarColor string, textColor string, gradientFrom string, gradientTo string, contact PersonChipContact) templ.Component {
	props := PersonCellProps{
		Name:         name,
		Subtitle:     contact.Role,
		Compact:      true,
		AvatarColor:  avatarColor,
		TextColor:    textColor,
		GradientFrom: gradientFrom,
		GradientTo:   gradientTo,
		Email:        contact.Email,
		BadgeLabel:   contact.BadgeLabel,
		BadgeClass:   contact.BadgeClass,
		ProfileHref:  contact.ProfileHref,
	}
	return PersonCellWithBoundary(props)
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
	outer := shared.RenderInto(NotificationPanel(unreadCount, viewAllHref), children)
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

// ButtonGlowWithBoundary wraps ButtonGlow with a dev-mode component boundary annotation.
// gallery:token variant,size,style,typ,shape,icon,loading,block
// gallery:hint href:default(#)
// ButtonGlowWithBoundary is a deprecated alias for ButtonWithBoundary with glow enabled.
// Deprecated: use ButtonWithBoundary(..., glow: true) directly.
func ButtonGlowWithBoundary(href string, variant ButtonVariant, size ButtonSize, style ButtonStyle, typ ButtonType, shape ButtonShape, icon string, loading bool, block bool) templ.Component {
	props := ButtonProps{
		Href:    href,
		Variant: variant,
		Size:    size,
		Style:   style,
		Type:    typ,
		Shape:   shape,
		Icon:    icon,
		Loading: loading,
		Block:   block,
		Glow:    true,
	}
	return devmode.ComponentBoundary("ButtonGlow", Button(props), props)
}

// AnimatedGradientTextWithBoundary wraps AnimatedGradientText with a dev-mode component boundary annotation.
// gallery:token text,fromColor,toColor,size,weight
// gallery:hint text:default(go-daisy)
// gallery:hint fromColor:default(from-primary)
// gallery:hint toColor:default(to-secondary)
// AnimatedGradientTextWithBoundary is a deprecated alias for GradientTextWithBoundary with animation enabled.
// Deprecated: use GradientTextWithBoundary and pass an animated GradientTextProps.
func AnimatedGradientTextWithBoundary(text string, fromColor string, toColor string, size string, weight string) templ.Component {
	props := GradientTextProps{Text: text, FromColor: fromColor, ToColor: toColor, Size: size, Weight: weight, Animate: true}
	return devmode.ComponentBoundary("AnimatedGradientText", GradientText(props), props)
}

// GradientTextWithBoundary wraps GradientText with a dev-mode component boundary annotation.
// gallery:token text,fromColor,toColor,size,weight
// gallery:hint text:default(go-daisy)
// gallery:hint fromColor:default(from-primary)
// gallery:hint toColor:default(to-secondary)
func GradientTextWithBoundary(text string, fromColor string, toColor string, size string, weight string) templ.Component {
	props := GradientTextProps{Text: text, FromColor: fromColor, ToColor: toColor, Size: size, Weight: weight}
	return devmode.ComponentBoundary("GradientText", GradientText(props), props)
}

// TestimonialCardWithBoundary wraps TestimonialCard with a dev-mode component boundary annotation.
// gallery:token quote,name,role,rating
// gallery:hint quote:default(Amazing product that transformed our workflow!)
// gallery:hint name:default(John Doe)
// gallery:hint role:default(CEO, Acme Corp)
// gallery:hint rating:range(0,5,1)
func TestimonialCardWithBoundary(props TestimonialCardProps) templ.Component {
	return devmode.ComponentBoundary("TestimonialCard", TestimonialCard(props), map[string]any{
		"quote":  props.Quote,
		"name":   props.Name,
		"role":   props.Role,
		"rating": props.Rating,
	})
}

// StatCardSparklineWithBoundary wraps StatCardSparkline with a dev-mode component boundary annotation.
// gallery:token label,value,trend
// gallery:hint label:default(Revenue)
// gallery:hint value:default($48,290)
func StatCardSparklineWithBoundary(props StatCardSparklineProps) templ.Component {
	return devmode.ComponentBoundary("StatCardSparkline", StatCardSparkline(props), map[string]any{
		"label":      props.Label,
		"value":      props.Value,
		"trend":      string(props.Trend),
		"trendLabel": props.TrendLabel,
	})
}

// ThemeToggleWithBoundary wraps ThemeToggle with a dev-mode component boundary annotation.
// gallery:token driver
// gallery:hint driver:default()
func ThemeToggleWithBoundary(driver string) templ.Component {
	return devmode.ComponentBoundary("ThemeToggle", ThemeToggle(ThemeToggleProps{Driver: driver}), map[string]any{
		"driver": driver,
	})
}

// IconSpanColoredWithBoundary wraps IconSpan (with color) for dev-mode component boundary annotation.
// Deprecated: call IconSpan(name, size, color) directly.
func IconSpanColoredWithBoundary(name string, size string, color string) templ.Component {
	inner := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return IconSpan(name, size, color).Render(ctx, w)
	})
	return devmode.ComponentBoundary("IconSpanColored", inner, map[string]any{
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
	props := ThemeControllerProps{Theme: theme, InputType: inputType, Label: label, Checked: checked}
	return devmode.ComponentBoundary("ThemeController", ThemeController(props), props)
}

// ThemeControllerBtnWithBoundary wraps ThemeControllerBtn with a dev-mode component boundary annotation.
// ThemeControllerBtnWithBoundary is a deprecated alias for ThemeController button variant.
// Deprecated: use ThemeController(ThemeControllerProps{Theme: theme, Checked: checked}) directly.
func ThemeControllerBtnWithBoundary(theme string, checked bool) templ.Component {
	props := ThemeControllerProps{Theme: theme, Checked: checked}
	return devmode.ComponentBoundary("ThemeControllerBtn", ThemeController(props), map[string]any{
		"theme":   theme,
		"checked": checked,
	})
}

// ThemeSwitcherWithBoundary wraps ThemeSwitcher with a dev-mode boundary annotation.
func ThemeSwitcherWithBoundary() templ.Component {
	return devmode.ComponentBoundary("ThemeSwitcher", ThemeSwitcher(), nil)
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
	inner := shared.RenderInto(Hover3DCard(Hover3DCardProps{ExtraClass: extraClass}), children)
	return devmode.ComponentBoundary("Hover3DCard", inner, map[string]any{"extraClass": extraClass})
}

// SortableListWithBoundary wraps SortableList with a dev-mode component boundary annotation.
// gallery:token animation,direction
// gallery:hint animation:range(0,500,10)
func SortableListWithBoundary(id string, opts SortableOptions, children templ.Component) templ.Component {
	inner := shared.RenderInto(SortableList(id, opts), children)
	return devmode.ComponentBoundary("SortableList", inner, map[string]any{
		"animation": opts.Animation,
		"direction": opts.Direction,
	})
}

// SwiperCarouselWithBoundary wraps SwiperCarousel with a dev-mode component boundary annotation.
// gallery:token effect,navigation,pagination,autoplay,loop,slidesPerView
// gallery:hint slidesPerView:range(1,6,1)
func SwiperCarouselWithBoundary(props SwiperCarouselProps) templ.Component {
	return devmode.ComponentBoundary("SwiperCarousel", SwiperCarousel(props), map[string]any{
		"effect":        string(props.Effect),
		"navigation":    props.Navigation,
		"pagination":    props.Pagination,
		"autoplay":      props.Autoplay,
		"loop":          props.Loop,
		"slidesPerView": props.SlidesPerView,
	})
}

// ChartWithBoundary wraps Chart with a dev-mode component boundary annotation.
// gallery:token type,title,sparkline,stacked,fillType,monochrome,legendPosition
// gallery:hint title:default(Sales Overview)
func ChartWithBoundary(props ChartProps) templ.Component {
	return devmode.ComponentBoundary("Chart", Chart(props), map[string]any{
		"type":      string(props.Type),
		"title":     props.Title,
		"sparkline": props.Sparkline,
	})
}

// CommandPaletteWithBoundary wraps CommandPalette with a dev-mode component boundary annotation.
// gallery:token placeholder
// gallery:hint placeholder:default(Type a command...)
func CommandPaletteWithBoundary(props CommandPaletteProps) templ.Component {
	return devmode.ComponentBoundary("CommandPalette", CommandPalette(props), map[string]any{
		"placeholder": props.Placeholder,
		"itemCount":   len(props.Items),
	})
}

// NotificationDropdownWithBoundary wraps NotificationDropdown with a dev-mode component boundary annotation.
// gallery:token unreadCount
func NotificationDropdownWithBoundary(props NotificationDropdownProps) templ.Component {
	return devmode.ComponentBoundary("NotificationDropdown", NotificationDropdown(props), map[string]any{
		"unreadCount": props.UnreadCount,
		"itemCount":   len(props.Items),
	})
}

// HoverGalleryWithBoundary wraps HoverGallery with a dev-mode component boundary annotation.
func HoverGalleryWithBoundary(images []HoverGalleryImage) templ.Component {
	return devmode.ComponentBoundary("HoverGallery", HoverGallery(images), map[string]any{
		"imageCount": len(images),
	})
}

// TabsWithBoundary wraps Tabs with a dev-mode component boundary annotation.
// gallery:token style,size,bottom,mode,driver
// gallery:hint style:default(lift)
// gallery:hint size:default(md)
// gallery:hint mode:default(server)
// gallery:hint driver:default()
func TabsWithBoundary(props TabsProps) templ.Component {
	return devmode.ComponentBoundary("Tabs", Tabs(props), map[string]any{
		"style":     string(props.Style),
		"size":      string(props.Size),
		"bottom":    props.Bottom,
		"mode":      string(props.Mode),
		"driver":    string(props.Driver),
		"itemCount": len(props.Items),
	})
}

// AuraWithBoundary wraps Aura with a dev-mode component boundary annotation.
func AuraWithBoundary(children templ.Component) templ.Component {
	inner := shared.RenderInto(Aura(), children)
	return devmode.ComponentBoundary("Aura", inner, map[string]any{})
}

// CodePreviewWithBoundary wraps CodePreview with a dev-mode component boundary annotation.
// gallery:token tabs
// gallery:hint tabs:slice(1)
func CodePreviewWithBoundary(tabs []CodeTab) templ.Component {
	return devmode.ComponentBoundary("CodePreview", CodePreview(tabs), map[string]any{"tabCount": len(tabs)})
}

// FrameWithBoundary wraps Frame with a dev-mode component boundary annotation.
// gallery:token id,src,loading
func FrameWithBoundary(id string, src string, loading string) templ.Component {
	return devmode.ComponentBoundary("Frame", Frame(id, src, loading), map[string]any{
		"id":      id,
		"src":     src,
		"loading": loading,
	})
}

// FrameIndicatorWithBoundary wraps FrameIndicator with a dev-mode component boundary annotation.
func FrameIndicatorWithBoundary() templ.Component {
	return devmode.ComponentBoundary("FrameIndicator", FrameIndicator())
}

// DashboardCardWithBoundary wraps DashboardCard with a dev-mode component boundary annotation.
// gallery:token title,subtitle
// gallery:hint title:default(Widget Title)
// gallery:hint subtitle:default()
func DashboardCardWithBoundary(title string, subtitle string) templ.Component {
	return devmode.ComponentBoundary("DashboardCard", DashboardCard(title, subtitle), map[string]any{
		"title":    title,
		"subtitle": subtitle,
	})
}

// DashboardGridWithBoundary wraps DashboardGrid with a dev-mode component boundary annotation.
// gallery:token cols
// gallery:hint cols:range(1,12,1)
// gallery:hint cols:default(4)
func DashboardGridWithBoundary(cols int) templ.Component {
	return devmode.ComponentBoundary("DashboardGrid", DashboardGrid(cols), map[string]any{"cols": cols})
}

// DashboardRowWithBoundary wraps DashboardRow with a dev-mode component boundary annotation.
func DashboardRowWithBoundary() templ.Component {
	return devmode.ComponentBoundary("DashboardRow", DashboardRow())
}

// DashboardColumnWithBoundary wraps DashboardColumn with a dev-mode component boundary annotation.
// gallery:token width
// gallery:hint width:default(1/2)
func DashboardColumnWithBoundary(width string) templ.Component {
	return devmode.ComponentBoundary("DashboardColumn", DashboardColumn(width), map[string]any{"width": width})
}

// DashboardSectionWithBoundary wraps DashboardSection with a dev-mode component boundary annotation.
// gallery:token title
// gallery:hint title:default(Dashboard)
func DashboardSectionWithBoundary(title string) templ.Component {
	return devmode.ComponentBoundary("DashboardSection", DashboardSection(title), map[string]any{"title": title})
}

// LayoutCustomizerWithBoundary wraps LayoutCustomizer with a dev-mode component boundary annotation.
func LayoutCustomizerWithBoundary() templ.Component {
	return devmode.ComponentBoundary("LayoutCustomizer", LayoutCustomizer(), nil)
}

// SearchDropdownWithBoundary wraps SearchDropdown with a dev-mode component boundary annotation.
// gallery:token placeholder,sections
// gallery:hint placeholder:default(Search...)
// gallery:hint sections:slice(2)
func SearchDropdownWithBoundary(placeholder string, sections []SearchDropdownSection) templ.Component {
	return devmode.ComponentBoundary("SearchDropdown", SearchDropdown(placeholder, sections), map[string]any{
		"placeholder":   placeholder,
		"sectionCount": len(sections),
	})
}

// StatsGroupWithBoundary wraps StatsGroup with a dev-mode component boundary annotation.
// gallery:token items
// gallery:hint items:slice(3)
func StatsGroupWithBoundary(items []StatsGroupItem) templ.Component {
	return devmode.ComponentBoundary("StatsGroup", StatsGroup(items), map[string]any{"itemCount": len(items)})
}

// StatCardsWithBoundary wraps StatCards with a dev-mode component boundary annotation.
// gallery:token cards
// gallery:hint cards:slice(4)
func StatCardsWithBoundary(cards []StatCardProps) templ.Component {
	return devmode.ComponentBoundary("StatCards", StatCards(cards), map[string]any{"cardCount": len(cards)})
}

// ProgressBarWithBoundary wraps ProgressBar with a dev-mode component boundary annotation.
func ProgressBarWithBoundary() templ.Component {
	return devmode.ComponentBoundary("ProgressBar", ProgressBar(), nil)
}

// Hover3DCardLayeredWithBoundary wraps Hover3DCardLayered with a dev-mode component boundary annotation.
// gallery:token extraClass
// gallery:hint extraClass:default(rounded-2xl w-64 h-40 bg-primary)
func Hover3DCardLayeredWithBoundary(extraClass string, layers []templ.Component) templ.Component {
	return devmode.ComponentBoundary("Hover3DCardLayered", Hover3DCardLayered(extraClass, layers), map[string]any{
		"extraClass": extraClass,
		"layerCount": len(layers),
	})
}

// HTMXIndicatorWithBoundary wraps HTMXIndicator with a dev-mode component boundary annotation.
// gallery:token id
// gallery:hint id:default(spinner)
func HTMXIndicatorWithBoundary(id string) templ.Component {
	return devmode.ComponentBoundary("HTMXIndicator", HTMXIndicator(id), map[string]any{"id": id})
}

// SimpleButtonWithBoundary wraps SimpleButton with a dev-mode component boundary annotation.
// gallery:token variant,size
func SimpleButtonWithBoundary(label string, variant ButtonVariant, size ButtonSize) templ.Component {
	return devmode.ComponentBoundary("SimpleButton", SimpleButton(label, variant, size, false, nil), map[string]any{
		"label":   label,
		"variant": string(variant),
		"size":    string(size),
	})
}

// SimpleButtonGlowWithBoundary wraps SimpleButtonGlow with a dev-mode component boundary annotation.
// gallery:token variant,size
// SimpleButtonGlowWithBoundary is a deprecated alias for SimpleButtonWithBoundary with glow enabled.
// Deprecated: use SimpleButtonWithBoundary with a glow-enabled call directly.
func SimpleButtonGlowWithBoundary(label string, variant ButtonVariant, size ButtonSize) templ.Component {
	return devmode.ComponentBoundary("SimpleButtonGlow", SimpleButton(label, variant, size, true, nil), map[string]any{
		"label":   label,
		"variant": string(variant),
		"size":    string(size),
	})
}

// CardCompactWithBoundary wraps CardCompact with a dev-mode component boundary annotation.
func CardCompactWithBoundary() templ.Component {
	inner := shared.RenderInto(CardCompact(nil), Skeleton("w-32 h-6"))
	return devmode.ComponentBoundary("CardCompact", inner, nil)
}

// PopoverWithBoundary wraps Popover with a dev-mode component boundary annotation.
// gallery:token placement,showArrow,triggerType
// gallery:hint placement:default(bottom)
// gallery:hint showArrow:default(true)
// gallery:hint triggerType:default(click)
func PopoverWithBoundary(placement PopoverPlacement, showArrow bool, triggerType PopoverTriggerType) templ.Component {
	popoverTrigger := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return PopoverTrigger(PopoverTriggerProps{TriggerType: triggerType}).Render(
			templ.WithChildren(ctx, shared.StrComp("Open Popover")), w)
	})
	cardContent := CardCompact(nil)
	cardWithText := shared.RenderInto(cardContent, shared.StrComp("Hello! This is a popover."))
	popoverContent := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return PopoverContent(PopoverContentProps{Placement: placement, ShowArrow: showArrow}).Render(
			templ.WithChildren(ctx, cardWithText), w)
	})
	children := shared.Compose(popoverTrigger, popoverContent)
	outer := shared.RenderInto(Popover(PopoverRootProps{}), children)
	withScript := shared.Compose(outer, PopoverScript())
	return devmode.ComponentBoundary("Popover", withScript, map[string]any{
		"placement":   string(placement),
		"showArrow":   showArrow,
		"triggerType": string(triggerType),
	})
}

// AspectRatioWithBoundary wraps AspectRatio with a dev-mode component boundary annotation.
// gallery:token ratio
// gallery:hint ratio:default(16/9)
func AspectRatioWithBoundary(ratio string) templ.Component {
	inner := shared.RenderInto(AspectRatio(AspectRatioProps{Ratio: ratio}), Skeleton("w-full h-full rounded-none"))
	return devmode.ComponentBoundary("AspectRatio", inner, map[string]any{"ratio": ratio})
}

// SeparatorWithBoundary wraps Separator with a dev-mode component boundary annotation.
// gallery:token orientation
// gallery:hint orientation:default(horizontal)
func SeparatorWithBoundary(orientation string) templ.Component {
	return devmode.ComponentBoundary("Separator", Separator(SeparatorProps{Orientation: orientation}), map[string]any{"orientation": orientation})
}

// TooltipWithBoundary wraps Tooltip with a dev-mode component boundary annotation.
// gallery:token tip,position
// gallery:hint tip:default(Helpful hint)
// gallery:hint position:default(top)
func TooltipWithBoundary(tip string, position string) templ.Component {
	btn := SimpleButton("Hover me", ButtonPrimary, ButtonSM, false, nil)
	inner := shared.RenderInto(Tooltip(TooltipProps{Tip: tip, Position: position}), btn)
	withScript := shared.Compose(inner, PopoverScript())
	return devmode.ComponentBoundary("Tooltip", withScript, map[string]any{"tip": tip, "position": position})
}

// HoverCardWithBoundary wraps HoverCard with a dev-mode component boundary annotation.
// gallery:token side
// gallery:hint side:default(bottom)
func HoverCardWithBoundary(side string) templ.Component {
	btn := SimpleButton("Hover me", ButtonPrimary, ButtonSM, false, nil)
	inner := shared.RenderInto(HoverCard(HoverCardProps{Side: side}), btn)
	withScript := shared.Compose(inner, PopoverScript())
	return devmode.ComponentBoundary("HoverCard", withScript, map[string]any{"side": side})
}

// CollapsibleWithBoundary wraps Collapsible with a dev-mode component boundary annotation.
// gallery:token title,open
// gallery:hint title:default(Collapsible Section)
// gallery:hint open:default(false)
func CollapsibleWithBoundary(title string, open bool) templ.Component {
	inner := shared.RenderInto(Collapsible(CollapsibleProps{ID: "demo-collapse", Title: title, Open: open, Icon: "lucide:info"}), shared.StrComp("Content that expands and collapses."))
	return devmode.ComponentBoundary("Collapsible", inner, map[string]any{"title": title, "open": open})
}

// IconWithBoundary wraps Icon with a dev-mode component boundary annotation.
// gallery:token name,size
// gallery:hint name:default(lucide:star)
// gallery:hint size:default(md)
func IconWithBoundary(name string, size string) templ.Component {
	return devmode.ComponentBoundary("Icon", Icon(IconProps{Name: name, Size: size}), map[string]any{"name": name, "size": size})
}

// TypographyTypeWithBoundary wraps TypographyType with a dev-mode component boundary annotation.
func TypographyTypeWithBoundary() templ.Component {
	return devmode.ComponentBoundary("TypographyType", TypographyType(), map[string]any{})
}

// TypographyLayoutExampleWithBoundary wraps TypographyLayoutExample with a dev-mode component boundary annotation.
func TypographyLayoutExampleWithBoundary() templ.Component {
	return devmode.ComponentBoundary("TypographyLayoutExample", TypographyLayoutExample(), map[string]any{})
}

// SheetWithBoundary wraps Sheet with a dev-mode component boundary annotation.
// gallery:token side,open
// gallery:hint side:default(left)
// gallery:hint open:default(true)
func SheetWithBoundary(side string, open bool) templ.Component {
	panelTitle := shared.StrComp("Sheet Panel")
	panelBody := shared.StrComp("Content slides in from the edge.")
	panelContent := shared.Compose(
		templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
			_, err := io.WriteString(w, `<div class="p-4"><h3 class="text-lg font-semibold mb-2">`)
			if err != nil { return err }
			if err := panelTitle.Render(ctx, w); err != nil { return err }
			_, err = io.WriteString(w, `</h3><p class="text-sm text-base-content/70">`)
			if err != nil { return err }
			if err := panelBody.Render(ctx, w); err != nil { return err }
			_, err = io.WriteString(w, `</p></div>`)
			return err
		}),
	)
	inner := shared.RenderInto(Sheet(SheetProps{ID: "demo-sheet", Side: SheetSide(side), Open: open}), panelContent)
	return devmode.ComponentBoundary("Sheet", inner, map[string]any{"side": side, "open": open})
}

// StimulusTabsWithBoundary wraps Tabs with Stimulus driver.
//
// Deprecated: use TabsWithBoundary(TabsProps{Driver: "stimulus", ...}) instead.
func StimulusTabsWithBoundary(props TabsProps) templ.Component {
	props.Driver = "stimulus"
	return TabsWithBoundary(props)
}

// AlpineThemeToggleWithBoundary wraps ThemeToggle with Alpine driver.
//
// Deprecated: use ThemeToggleWithBoundary("alpine") instead.
func AlpineThemeToggleWithBoundary() templ.Component {
	return ThemeToggleWithBoundary("alpine")
}

// StimulusThemeToggleWithBoundary wraps ThemeToggle with Stimulus driver.
//
// Deprecated: use ThemeToggleWithBoundary("stimulus") instead.
func StimulusThemeToggleWithBoundary() templ.Component {
	return ThemeToggleWithBoundary("stimulus")
}

// AlpineToastWithBoundary wraps Toast with Alpine driver.
//
// Deprecated: use ToastWithBoundary(typ, message, "alpine") instead.
func AlpineToastWithBoundary(typ ToastType, message string) templ.Component {
	return ToastWithBoundary(typ, message, "alpine")
}

