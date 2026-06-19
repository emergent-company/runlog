package form

import (
	"context"
	"io"

	"github.com/a-h/templ"
	"github.com/emergent-company/go-daisy/devmode"
)

// TextInputWithBoundary wraps TextInput with a dev-mode component boundary annotation.
// gallery:token label,required
// gallery:hint label:default(Full Name)
func TextInputWithBoundary(name string, label string, value string, errMsg string, required bool) templ.Component {
	return devmode.ComponentBoundary("TextInput", TextInput(name, label, value, errMsg, required), map[string]any{
		"name":     name,
		"label":    label,
		"required": required,
	})
}

// TextareaInputWithBoundary wraps TextareaInput with a dev-mode component boundary annotation.
// gallery:token label,rows,required
// gallery:hint rows:range(2,10,1)
// gallery:hint label:default(Description)
func TextareaInputWithBoundary(name string, label string, value string, errMsg string, rows int, required bool) templ.Component {
	return devmode.ComponentBoundary("TextareaInput", TextareaInput(name, label, value, errMsg, rows, required), map[string]any{
		"name":     name,
		"label":    label,
		"rows":     rows,
		"required": required,
	})
}

// CheckboxInputWithBoundary wraps CheckboxInput with a dev-mode component boundary annotation.
// gallery:token label,checked
// gallery:hint label:default(Accept terms and conditions)
func CheckboxInputWithBoundary(name string, label string, checked bool, errMsg string) templ.Component {
	return devmode.ComponentBoundary("CheckboxInput", CheckboxInput(name, label, checked, errMsg), map[string]any{
		"name":    name,
		"label":   label,
		"checked": checked,
	})
}

// SelectInputWithBoundary wraps SelectInput with a dev-mode component boundary annotation.
// gallery:token label,required
// gallery:hint label:default(Country)
func SelectInputWithBoundary(name string, label string, selected string, options [][2]string, errMsg string, required bool) templ.Component {
	return devmode.ComponentBoundary("SelectInput", SelectInput(name, label, selected, options, errMsg, required), map[string]any{
		"name":     name,
		"label":    label,
		"selected": selected,
		"required": required,
	})
}

// SearchInputWithBoundary wraps SearchInput with a dev-mode component boundary annotation.
func SearchInputWithBoundary(name string, value string, placeholder string, hxTarget string, hxGet string) templ.Component {
	return devmode.ComponentBoundary("SearchInput", SearchInput(name, value, placeholder, hxTarget, hxGet), map[string]any{
		"name":        name,
		"placeholder": placeholder,
		"hxTarget":    hxTarget,
	})
}

// FormFieldWithBoundary wraps FormField with a dev-mode component boundary annotation.
func FormFieldWithBoundary(props FormFieldProps) templ.Component {
	return devmode.ComponentBoundary("FormField", FormField(props), props)
}

// RangeInputWithBoundary wraps RangeInput with a dev-mode component boundary annotation.
// gallery:token label,value,color
// gallery:hint value:range(0,100,1)
// gallery:hint label:default(Volume)
// gallery:hint color:default(range-primary)
// gallery:hint value:default(50)
func RangeInputWithBoundary(name string, label string, value int, min int, max int, step int, color string) templ.Component {
	return devmode.ComponentBoundary("RangeInput", RangeInput(name, label, value, min, max, step, color), map[string]any{
		"name":  name,
		"label": label,
		"min":   min,
		"max":   max,
		"step":  step,
	})
}

// RadioGroupWithBoundary wraps RadioGroup with a dev-mode component boundary annotation.
// gallery:token color
// gallery:hint color:default(radio-primary)
func RadioGroupWithBoundary(name string, selected string, options [][2]string, color string) templ.Component {
	return devmode.ComponentBoundary("RadioGroup", RadioGroup(name, selected, options, color), map[string]any{
		"name":     name,
		"selected": selected,
		"color":    color,
	})
}

// RatingWithBoundary wraps Rating with a dev-mode component boundary annotation.
// gallery:token value,max,shape,color,size
// gallery:hint value:range(1,10,1)
// gallery:hint value:default(3)
// gallery:hint max:range(1,10,1)
// gallery:hint max:default(5)
// gallery:hint color:default(bg-orange-400)
func RatingWithBoundary(name string, value int, max int, shape RatingShape, color string, size string) templ.Component {
	return devmode.ComponentBoundary("Rating", Rating(name, value, max, shape, color, size), map[string]any{
		"name":  name,
		"value": value,
		"max":   max,
		"shape": string(shape),
		"color": color,
		"size":  size,
	})
}

// FileInputWithBoundary wraps FileInput with a dev-mode component boundary annotation.
// gallery:token label,accept
// gallery:hint label:default(Upload file)
func FileInputWithBoundary(name string, label string, accept string) templ.Component {
	return devmode.ComponentBoundary("FileInput", FileInput(name, label, accept, ""), map[string]any{
		"name":   name,
		"label":  label,
		"accept": accept,
	})
}

// CheckboxWithBoundary wraps Checkbox with a dev-mode component boundary annotation.
// gallery:token label,checked
// gallery:hint label:default(Accept terms and conditions)
func CheckboxWithBoundary(name string, checked bool, label string) templ.Component {
	return devmode.ComponentBoundary("Checkbox", Checkbox(name, checked, label), map[string]any{
		"name":    name,
		"checked": checked,
		"label":   label,
	})
}

// ToggleWithBoundary wraps Toggle with a dev-mode component boundary annotation.
// gallery:token label,checked
// gallery:hint label:default(Enable notifications)
func ToggleWithBoundary(name string, checked bool, label string) templ.Component {
	return devmode.ComponentBoundary("Toggle", Toggle(name, checked, label), map[string]any{
		"name":    name,
		"checked": checked,
		"label":   label,
	})
}

// FormControlWithBoundary wraps FormControl with a dev-mode component boundary annotation.
func FormControlWithBoundary(name string, label string, labelPosition LabelPosition, hint string, errMsg string, children templ.Component) templ.Component {
	inner := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return FormControl(name, label, labelPosition, hint, errMsg, nil).Render(templ.WithChildren(ctx, children), w)
	})
	return devmode.ComponentBoundary("FormControl", inner, map[string]any{
		"name":          name,
		"label":         label,
		"labelPosition": string(labelPosition),
	})
}

// FormInputWithBoundary wraps FormInput with a dev-mode component boundary annotation.
// gallery:token label,labelPosition,placeholder
// gallery:hint label:default(Full Name)
// gallery:hint labelPosition:default(above)
func FormInputWithBoundary(name string, label string, value string, placeholder string, labelPosition LabelPosition, hint string, errMsg string, attrs templ.Attributes) templ.Component {
	return devmode.ComponentBoundary("FormInput", FormInput(name, label, value, placeholder, labelPosition, hint, errMsg, attrs), map[string]any{
		"name":          name,
		"label":         label,
		"value":         value,
		"placeholder":   placeholder,
		"labelPosition": string(labelPosition),
	})
}

// FormSelectWithBoundary wraps FormSelect with a dev-mode component boundary annotation.
// gallery:token label,labelPosition,placeholder
// gallery:hint label:default(Country)
// gallery:hint labelPosition:default(above)
func FormSelectWithBoundary(name string, label string, selected string, options [][2]string, placeholder string, labelPosition LabelPosition, hint string, errMsg string, attrs templ.Attributes) templ.Component {
	return devmode.ComponentBoundary("FormSelect", FormSelect(name, label, selected, options, placeholder, labelPosition, hint, errMsg, attrs), map[string]any{
		"name":          name,
		"label":         label,
		"selected":      selected,
		"placeholder":   placeholder,
		"labelPosition": string(labelPosition),
	})
}

// FormCheckboxWithBoundary wraps FormCheckbox with a dev-mode component boundary annotation.
// gallery:token label,checked
// gallery:hint label:default(Accept terms)
func FormCheckboxWithBoundary(name string, label string, checked bool, errMsg string, attrs templ.Attributes) templ.Component {
	return devmode.ComponentBoundary("FormCheckbox", FormCheckbox(name, label, checked, errMsg, attrs), map[string]any{
		"name":    name,
		"label":   label,
		"checked": checked,
	})
}

// FormToggleWithBoundary wraps FormToggle with a dev-mode component boundary annotation.
// gallery:token label,checked
// gallery:hint label:default(Enable notifications)
func FormToggleWithBoundary(name string, label string, checked bool, errMsg string, attrs templ.Attributes) templ.Component {
	return devmode.ComponentBoundary("FormToggle", FormToggle(name, label, checked, errMsg, attrs), map[string]any{
		"name":    name,
		"label":   label,
		"checked": checked,
	})
}

// PromptBarWithBoundary wraps PromptBar with a dev-mode component boundary annotation.
func PromptBarWithBoundary(props PromptBarProps) templ.Component {
	return devmode.ComponentBoundary("PromptBar", PromptBar(props), props)
}

// PromptBarActionWithBoundary wraps PromptBarAction with a dev-mode component boundary annotation.
func PromptBarActionWithBoundary(placeholder string, actions []PromptBarActionItem) templ.Component {
	return devmode.ComponentBoundary("PromptBarAction", PromptBarAction(placeholder, actions, false, nil), map[string]any{
		"placeholder": placeholder,
		"actionCount": len(actions),
	})
}

// PromptBarModelSelectorWithBoundary wraps PromptBarModelSelector with a dev-mode component boundary annotation.
func PromptBarModelSelectorWithBoundary(props PromptBarModelSelectorProps) templ.Component {
	return devmode.ComponentBoundary("PromptBarModelSelector", PromptBarModelSelector(props), props)
}

// PromptBarAbilityWithBoundary wraps PromptBarAbility with a dev-mode component boundary annotation.
func PromptBarAbilityWithBoundary(props PromptBarAbilityProps) templ.Component {
	return devmode.ComponentBoundary("PromptBarAbility", PromptBarAbility(props), props)
}

// InputSpinnerWithBoundary wraps InputSpinner with a dev-mode component boundary annotation.
func InputSpinnerWithBoundary(id string, value, min, max int, hasMinMax bool, btnClass, inputClass string) templ.Component {
	return devmode.ComponentBoundary("InputSpinner", InputSpinner(id, value, min, max, hasMinMax, btnClass, inputClass), map[string]any{
		"id":         id,
		"value":      value,
		"min":        min,
		"max":        max,
		"hasMinMax":  hasMinMax,
		"btnClass":   btnClass,
		"inputClass": inputClass,
	})
}

// WizardStepperWithBoundary wraps WizardStepper with a dev-mode component boundary annotation.
func WizardStepperWithBoundary(id string, steps []WizardStep, panels []WizardStepPanel) templ.Component {
	return devmode.ComponentBoundary("WizardStepper", WizardStepper(id, steps, panels), map[string]any{
		"id":         id,
		"stepCount":  len(steps),
		"panelCount": len(panels),
	})
}

// LabelWithBoundary wraps Label with a dev-mode component boundary annotation.
func LabelWithBoundary(props LabelProps, children templ.Component) templ.Component {
	inner := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return Label(props).Render(templ.WithChildren(ctx, children), w)
	})
	return devmode.ComponentBoundary("Label", inner, map[string]any{
		"text":    props.Text,
		"altText": props.AltText,
		"for":     props.For,
	})
}

// ValidatorInputWithBoundary wraps ValidatorInput with a dev-mode component boundary annotation.
func ValidatorInputWithBoundary(children templ.Component) templ.Component {
	inner := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return ValidatorInput().Render(templ.WithChildren(ctx, children), w)
	})
	return devmode.ComponentBoundary("ValidatorInput", inner, map[string]any{})
}

// ValidatorHintWithBoundary wraps ValidatorHint with a dev-mode component boundary annotation.
func ValidatorHintWithBoundary(text string) templ.Component {
	return devmode.ComponentBoundary("ValidatorHint", ValidatorHint(text), map[string]any{"text": text})
}

// ValidatedFieldWithBoundary wraps ValidatedField with a dev-mode component boundary annotation.
func ValidatedFieldWithBoundary(labelText string, hintText string, inputName string, children templ.Component) templ.Component {
	inner := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return ValidatedField(labelText, hintText, inputName).Render(templ.WithChildren(ctx, children), w)
	})
	return devmode.ComponentBoundary("ValidatedField", inner, map[string]any{
		"labelText": labelText,
		"hintText":  hintText,
		"inputName": inputName,
	})
}

// CalendarWrapperWithBoundary wraps CalendarWrapper with a dev-mode component boundary annotation.
func CalendarWrapperWithBoundary(variant CalendarVariant, children templ.Component) templ.Component {
	inner := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return CalendarWrapper(variant).Render(templ.WithChildren(ctx, children), w)
	})
	return devmode.ComponentBoundary("CalendarWrapper", inner, map[string]any{
		"variant": string(variant),
	})
}

// CalendarDemoWithBoundary wraps CalendarDemo with a dev-mode component boundary annotation.
func CalendarDemoWithBoundary(month string, year string, startWeekday int, daysInMonth int, today int, selected int) templ.Component {
	return devmode.ComponentBoundary("CalendarDemo", CalendarDemo(month, year, startWeekday, daysInMonth, today, selected), map[string]any{
		"month":        month,
		"year":         year,
		"startWeekday": startWeekday,
		"daysInMonth":  daysInMonth,
		"today":        today,
		"selected":     selected,
	})
}
