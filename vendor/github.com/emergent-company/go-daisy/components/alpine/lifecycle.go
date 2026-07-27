package alpine

// ModalInit returns the x-init expression for modal scroll lock and focus.
func ModalInit() string {
	return `$watch('open',function(v){if(v){document.body.style.overflow='hidden';$nextTick(function(){var c=$el.querySelector('[x-ref=close]');if(c)c.focus()})}else{document.body.style.overflow=''}})`
}

// DropdownKeyboardInit returns the x-init expression for dropdown keyboard navigation.
func DropdownKeyboardInit() string {
	return `var items=$el.querySelectorAll('[role=menuitem]');var idx=-1;$watch('open',function(v){if(v){idx=-1;$nextTick(function(){var m=$el.querySelector('[role=menu]');if(m)m.focus()})}});$el.addEventListener('keydown',function(e){if(!$data.open)return;if(e.key==='ArrowDown'){e.preventDefault();idx=(idx+1)%items.length;items[idx]&&items[idx].focus()}if(e.key==='ArrowUp'){e.preventDefault();idx=(idx-1+items.length)%items.length;items[idx]&&items[idx].focus()}if(e.key==='Enter'&&idx>=0){e.preventDefault();items[idx].click();$data.open=false}if(e.key==='Escape'){$data.open=false}})`
}

// TabsHistoryInit returns the x-init expression for tabs synced with URL hash.
func TabsHistoryInit() string {
	return `var h=window.location.hash.slice(1);if(h&&$el.querySelector('[data-tab='+h+']')){tab=h};$watch('tab',function(v){history.replaceState(null,'','#'+v)})`
}

// ToastQueueInit returns the x-init expression for toast queue management:
// add(toast), dismiss(id), and auto-remove after timeout.
func ToastQueueInit() string {
	return `var queue=this;queue.add=function(t){var item=Object.assign({id:'t'+Date.now(),type:'info',message:'',duration:4000},t);queue.toasts.push(item);if(item.duration>0){setTimeout(function(){queue.dismiss(item.id)},item.duration)}};queue.dismiss=function(id){queue.toasts=queue.toasts.filter(function(t){return t.id!==id})}}`
}

// ComboboxInit returns the x-init expression for combobox keyboard navigation,
// item selection, and click-outside dismiss. multi=true for toggle-select mode.
func ComboboxInit(multi bool) string {
	selFn := `root.selectItem=function(v){root.selected=[v];root.open=false};`
	if multi {
		selFn = `root.selectItem=function(v){var i=root.selected.indexOf(v);if(i>=0)root.selected.splice(i,1);else root.selected.push(v)};`
	}
	return `var root=this;root.toggle=function(){root.open=!root.open;if(root.open){root.activeIndex=-1;var s=root.$el.querySelector('input[type=text]');if(s)s.focus()}};` +
		selFn +
		`root.clearAll=function(){root.selected=[]};` +
		`root.isSelected=function(v){return root.selected.indexOf(v)>=0};` +
		`root.getVisibleIndex=function(el){var all=root.$el.querySelectorAll('[role=option]');var c=0;for(var i=0;i<all.length;i++){if(all[i].offsetParent!==null){if(all[i]===el)return c;c++}}return -1};` +
		`root.$el.addEventListener('keydown',function(e){if(!root.open)return;var all=root.$el.querySelectorAll('[role=option]');if(e.key==='ArrowDown'){e.preventDefault();do{root.activeIndex=(root.activeIndex+1)%all.length}while(all[root.activeIndex]&&all[root.activeIndex].offsetParent===null);if(all[root.activeIndex])all[root.activeIndex].scrollIntoView({block:'nearest'})}if(e.key==='ArrowUp'){e.preventDefault();do{root.activeIndex=(root.activeIndex-1+all.length)%all.length}while(all[root.activeIndex]&&all[root.activeIndex].offsetParent===null);if(all[root.activeIndex])all[root.activeIndex].scrollIntoView({block:'nearest'})}if(e.key==='Enter'&&root.activeIndex>=0){e.preventDefault();var it=all[root.activeIndex];if(it&&it.offsetParent!==null)it.click()}if(e.key==='Escape'){root.open=false}});`
}

// StructuredInputInit returns the x-init expression for repeatable row add/remove.
func StructuredInputInit() string {
	return `var root=this;root.addRow=function(){root.rows.push({})};root.removeRow=function(idx){root.rows.splice(idx,1)};`
}

// TagListInit returns the x-init expression for tag list add/remove.
// Tags added via Enter/comma/space; Backspace on empty removes last.
func TagListInit() string {
	return `var root=this;root.addTag=function(){var v=root.input.trim();if(!v)return;var parts=v.split(',');for(var i=0;i<parts.length;i++){var t=parts[i].trim();if(t&&root.tags.indexOf(t)<0)root.tags.push(t)}root.input=''};root.removeTag=function(idx){root.tags.splice(idx,1)};root.handleKeydown=function(e){if(e.key==='Enter'||e.key===','){e.preventDefault();root.addTag()}if(e.key==='Backspace'&&!root.input&&root.tags.length){root.removeTag(root.tags.length-1)}};`
}
