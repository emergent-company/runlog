package form

import (
	"github.com/a-h/templ"
	"github.com/emergent-company/go-daisy/devmode"
	"github.com/emergent-company/go-daisy/shared"
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
func SearchInputWithBoundary(name string, value string, placeholder string, hxTarget string, hxGet string, class string) templ.Component {
	return devmode.ComponentBoundary("SearchInput", SearchInput(name, value, placeholder, hxTarget, hxGet, class), map[string]any{
		"name":        name,
		"placeholder": placeholder,
		"hxTarget":    hxTarget,
	})
}

// FilterSelectWithBoundary wraps FilterSelect with a dev-mode component boundary annotation.
func FilterSelectWithBoundary(name string, hxTarget string, hxGet string, class string) templ.Component {
	return devmode.ComponentBoundary("FilterSelect", FilterSelect(name, hxTarget, hxGet, class), map[string]any{
		"name": name,
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
func RadioGroupWithBoundary(name string, selected string, options []SelectOption, color string) templ.Component {
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
	inner := shared.RenderInto(FormControl(name, label, labelPosition, hint, errMsg, nil), children)
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
	inner := shared.RenderInto(Label(props), children)
	return devmode.ComponentBoundary("Label", inner, map[string]any{
		"text":    props.Text,
		"altText": props.AltText,
		"for":     props.For,
	})
}

// ValidatorInputWithBoundary wraps ValidatorInput with a dev-mode component boundary annotation.
func ValidatorInputWithBoundary(children templ.Component) templ.Component {
	inner := shared.RenderInto(ValidatorInput(), children)
	return devmode.ComponentBoundary("ValidatorInput", inner, map[string]any{})
}

// ValidatorHintWithBoundary wraps ValidatorHint with a dev-mode component boundary annotation.
func ValidatorHintWithBoundary(text string) templ.Component {
	return devmode.ComponentBoundary("ValidatorHint", ValidatorHint(text), map[string]any{"text": text})
}

// ValidatedFieldWithBoundary wraps ValidatedField with a dev-mode component boundary annotation.
func ValidatedFieldWithBoundary(labelText string, hintText string, inputName string, children templ.Component) templ.Component {
	inner := shared.RenderInto(ValidatedField(labelText, hintText, inputName), children)
	return devmode.ComponentBoundary("ValidatedField", inner, map[string]any{
		"labelText": labelText,
		"hintText":  hintText,
		"inputName": inputName,
	})
}

// PasswordFieldWithBoundary wraps PasswordField with a dev-mode component boundary annotation.
// gallery:token label,required,showToggle
// gallery:hint label:default(Password)
func PasswordFieldWithBoundary(props PasswordFieldProps) templ.Component {
	return devmode.ComponentBoundary("PasswordField", PasswordField(props), map[string]any{
		"label":      props.Label,
		"required":   props.Required,
		"showToggle": props.ShowToggle,
	})
}

// PasswordMeterWithBoundary wraps PasswordMeter with a dev-mode component boundary annotation.
// gallery:token label,required,showToggle,minLength
// gallery:hint label:default(Password)
// gallery:hint minLength:range(0,32,1)
func PasswordMeterWithBoundary(props PasswordMeterProps) templ.Component {
	return devmode.ComponentBoundary("PasswordMeter", PasswordMeter(props), map[string]any{
		"label":      props.Label,
		"required":   props.Required,
		"showToggle": props.ShowToggle,
		"minLength":  props.MinLength,
	})
}

// CalendarWrapperWithBoundary wraps CalendarWrapper with a dev-mode component boundary annotation.
func CalendarWrapperWithBoundary(variant CalendarVariant, children templ.Component) templ.Component {
	inner := shared.RenderInto(CalendarWrapper(variant), children)
	return devmode.ComponentBoundary("CalendarWrapper", inner, map[string]any{
		"variant": string(variant),
	})
}

// EnhancedSelectWithBoundary wraps EnhancedSelect with a dev-mode component boundary annotation.
// gallery:token label,multiple,searchable,removeItems,required
// gallery:hint label:default(Select an option)
func EnhancedSelectWithBoundary(props EnhancedSelectProps) templ.Component {
	return devmode.ComponentBoundary("EnhancedSelect", EnhancedSelect(props), map[string]any{
		"label":       props.Label,
		"multiple":    props.Multiple,
		"searchable":  props.Searchable,
		"removeItems": props.RemoveItems,
		"required":    props.Required,
	})
}

// DatePickerWithBoundary wraps DatePicker with a dev-mode component boundary annotation.
// gallery:token label,mode,inline,time24h
// gallery:hint label:default(Select date)
func DatePickerWithBoundary(props DatePickerProps) templ.Component {
	return devmode.ComponentBoundary("DatePicker", DatePicker(props), map[string]any{
		"label":   props.Label,
		"mode":    string(props.Mode),
		"inline":  props.Inline,
		"time24h": props.Time24h,
	})
}

// RichTextEditorWithBoundary wraps RichTextEditor with a dev-mode component boundary annotation.
// gallery:token label,theme,readOnly
// gallery:hint label:default(Content)
func RichTextEditorWithBoundary(props RichTextEditorProps) templ.Component {
	return devmode.ComponentBoundary("RichTextEditor", RichTextEditor(props), map[string]any{
		"label":    props.Label,
		"theme":    string(props.Theme),
		"readOnly": props.ReadOnly,
	})
}

// FileUploadWithBoundary wraps FileUpload with a dev-mode component boundary annotation.
// gallery:token label,multiple,style
// gallery:hint label:default(Upload files)
func FileUploadWithBoundary(props FileUploadProps) templ.Component {
	return devmode.ComponentBoundary("FileUpload", FileUpload(props), map[string]any{
		"label":    props.Label,
		"multiple": props.Multiple,
		"style":    string(props.Style),
	})
}

// FormValidationWithBoundary wraps FormValidation with a dev-mode component boundary annotation.
// gallery:token submitText
// gallery:hint submitText:default(Submit)
func FormValidationWithBoundary(props FormValidationProps, children templ.Component) templ.Component {
	inner := shared.RenderInto(FormValidation(props), children)
	return devmode.ComponentBoundary("FormValidation", inner, map[string]any{
		"submitText": props.SubmitText,
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

// OTPInputWithBoundary wraps OTPInput with a dev-mode component boundary annotation.
func OTPInputWithBoundary(id string, digits int) templ.Component {
	return devmode.ComponentBoundary("OTPInput", OTPInput(id, digits), map[string]any{
		"id":     id,
		"digits": digits,
	})
}

// ColorInputWithBoundary wraps ColorInput with a dev-mode component boundary annotation.
func ColorInputWithBoundary(id string, value string) templ.Component {
	return devmode.ComponentBoundary("ColorInput", ColorInput(id, value), map[string]any{
		"id":    id,
		"value": value,
	})
}

// PaletteWithBoundary wraps Palette with a dev-mode component boundary annotation.
// gallery:token showHex,hideNeutral,hideReset
func PaletteWithBoundary(props PaletteProps) templ.Component {
	return devmode.ComponentBoundary("Palette", Palette(props), map[string]any{
		"hueCount":   len(tailwindHues(props.Hues)),
		"shadeCount": len(tailwindShades(props.Shades)),
		"showHex":    props.ShowHex,
	})
}

// DatalistInputWithBoundary wraps DatalistInput with a dev-mode component boundary annotation.
func DatalistInputWithBoundary(id string, placeholder string, options []DatalistOption) templ.Component {
	return devmode.ComponentBoundary("DatalistInput", DatalistInput(id, placeholder, options), map[string]any{
		"id":           id,
		"optionCount":  len(options),
	})
}

// FieldsetWithBoundary wraps Fieldset with a dev-mode component boundary annotation.
// gallery:token legend
// gallery:hint legend:default(Settings)
func FieldsetWithBoundary(props FieldsetProps) templ.Component {
	return devmode.ComponentBoundary("Fieldset", Fieldset(props), map[string]any{
		"legend": props.Legend,
	})
}

// SelectShellWithBoundary wraps SelectShell with a dev-mode component boundary annotation.
// gallery:token triggerLabel
// gallery:hint triggerLabel:default(Select...)
func SelectShellWithBoundary(props SelectShellProps) templ.Component {
	return devmode.ComponentBoundary("SelectShell", SelectShell(props), map[string]any{
		"label":        props.Label,
		"triggerLabel": props.TriggerLabel,
	})
}

// FormActionsWithBoundary wraps FormActions with a dev-mode component boundary annotation.
func FormActionsWithBoundary() templ.Component {
	inner := shared.RenderInto(FormActions(), shared.StrComp(`<button class="btn btn-primary">Save</button>`))
	return devmode.ComponentBoundary("FormActions", inner, nil)
}

// FormActionsWithLoadingWithBoundary wraps FormActionsWithLoading with a dev-mode component boundary annotation.
// gallery:token submitText
// gallery:hint submitText:default(Save Changes)
func FormActionsWithLoadingWithBoundary(submitText string) templ.Component {
	return devmode.ComponentBoundary("FormActionsWithLoading", FormActionsWithLoading(submitText, ""), map[string]any{
		"submitText": submitText,
	})
}

// DateInputWithBoundary wraps DateInput with a dev-mode component boundary annotation.
// gallery:token name,value
func DateInputWithBoundary(name string, value string) templ.Component {
	return devmode.ComponentBoundary("DateInput", DateInput(name, value, false), map[string]any{
		"name":  name,
		"value": value,
	})
}

// ComboboxWithBoundary wraps Combobox.
// gallery:token mode,enableSearch,enableClearAll
func ComboboxWithBoundary(props ComboboxProps) templ.Component {
	return devmode.ComponentBoundary("Combobox", Combobox(props), map[string]any{
		"mode":           string(props.Mode),
		"enableSearch":   props.EnableSearch,
		"enableClearAll": props.EnableClearAll,
	})
}

// StructuredInputWithBoundary wraps StructuredInput.
// gallery:token addActionLabel
// gallery:hint addActionLabel:default(Add row)
func StructuredInputWithBoundary(props StructuredInputProps) templ.Component {
	return devmode.ComponentBoundary("StructuredInput", StructuredInput(props), map[string]any{
		"columns": len(props.Columns),
		"entries": len(props.Entries),
	})
}

// TagListWithBoundary wraps TagList with a dev-mode component boundary annotation.
// gallery:token values
func TagListWithBoundary(props TagListProps) templ.Component {
	return devmode.ComponentBoundary("TagList", TagList(props), map[string]any{
		"values": len(props.Values),
	})
}
