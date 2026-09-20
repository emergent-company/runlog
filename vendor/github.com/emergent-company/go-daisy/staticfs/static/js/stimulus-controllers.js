// go-daisy Stimulus controllers — pre-built for common interactions.
// Register with: import { Application } from "@hotwired/stimulus"
//                 import { ModalController, DropdownController, ... } from "./go-daisy-controllers.js"
//
// Or load from /static/js/stimulus-controllers.js and register globally.

// ModalController — focus trap, scroll lock, Escape to close.
class ModalController extends Stimulus.Controller {
  static targets = ["dialog"];
  static values = { open: { type: Boolean, default: false } };

  connect() {
    if (this.openValue) this.show();
  }

  disconnect() {
    document.body.style.overflow = "";
  }

  show() {
    this.openValue = true;
    this.dialogTarget.showModal();
    document.body.style.overflow = "hidden";
    this.dialogTarget.focus();
  }

  close() {
    this.openValue = false;
    this.dialogTarget.close();
    document.body.style.overflow = "";
  }

  backdropClose(event) {
    if (event.target === this.dialogTarget) this.close();
  }

  escapeClose(event) {
    if (event.key === "Escape") { event.preventDefault(); this.close(); }
  }
}

// DropdownController — toggle, outside-click dismiss, keyboard navigation.
class DropdownController extends Stimulus.Controller {
  static targets = ["menu", "trigger"];
  static values = { open: { type: Boolean, default: false } };

  toggle() { this.openValue = !this.openValue; }
  close() { this.openValue = false; }

  openValueChanged() {
    if (this.openValue) {
      this.menuTarget.style.display = "";
      this.triggerTarget.setAttribute("aria-expanded", "true");
      this.menuTarget.querySelector("[role=menuitem]")?.focus();
    } else {
      this.menuTarget.style.display = "none";
      this.triggerTarget.setAttribute("aria-expanded", "false");
    }
  }

  nextItem(event) {
    event.preventDefault();
    const items = this.menuTarget.querySelectorAll("[role=menuitem]");
    const idx = Array.from(items).indexOf(document.activeElement);
    items[(idx + 1) % items.length]?.focus();
  }

  prevItem(event) {
    event.preventDefault();
    const items = this.menuTarget.querySelectorAll("[role=menuitem]");
    const idx = Array.from(items).indexOf(document.activeElement);
    items[(idx - 1 + items.length) % items.length]?.focus();
  }

  selectItem(event) {
    event.preventDefault();
    document.activeElement?.click();
    this.close();
  }

  outsideClick(event) {
    if (!this.element.contains(event.target)) this.close();
  }

  escapeClose(event) {
    if (event.key === "Escape") this.close();
  }
}

// TabsController — instant tab switching with optional URL hash sync.
class TabsController extends Stimulus.Controller {
  static targets = ["tab", "panel"];
  static values = { active: String, history: { type: Boolean, default: false } };

  connect() {
    if (this.historyValue) {
      const hash = window.location.hash.slice(1);
      if (hash && this.element.querySelector(`[data-tab="${hash}"]`)) {
        this.activeValue = hash;
      }
    }
    if (!this.activeValue && this.tabTargets.length > 0) {
      this.activeValue = this.tabTargets[0].dataset.tab;
    }
    this.switch();
  }

  select(event) {
    this.activeValue = event.currentTarget.dataset.tab;
    this.switch();
  }

  switch() {
    this.tabTargets.forEach(t => {
      t.classList.toggle("tab-active", t.dataset.tab === this.activeValue);
      t.setAttribute("aria-selected", t.dataset.tab === this.activeValue ? "true" : "false");
    });
    this.panelTargets.forEach(p => {
      p.style.display = p.dataset.tab === this.activeValue ? "" : "none";
    });
    if (this.historyValue) {
      history.replaceState(null, "", "#" + this.activeValue);
    }
  }
}

// AccordionController — single/multi toggle with smooth height animation.
class AccordionController extends Stimulus.Controller {
  static targets = ["item"];
  static values = { open: String, multiple: { type: Boolean, default: false } };

  toggle(event) {
    const key = event.currentTarget.dataset.key;
    if (this.multipleValue) {
      const open = this.openValue ? this.openValue.split(",") : [];
      const idx = open.indexOf(key);
      if (idx >= 0) open.splice(idx, 1); else open.push(key);
      this.openValue = open.join(",");
    } else {
      this.openValue = this.openValue === key ? "" : key;
    }
    this.update();
  }

  update() {
    const openKeys = this.openValue ? this.openValue.split(",") : [];
    this.itemTargets.forEach(item => {
      const key = item.dataset.key;
      const isOpen = openKeys.includes(key);
      item.classList.toggle("collapse-open", isOpen);
      item.classList.toggle("collapse-close", !isOpen);
    });
  }
}

// ThemeController — dark/light toggle with localStorage persistence.
class ThemeController extends Stimulus.Controller {
  static values = { dark: { type: Boolean, default: false } };

  connect() {
    const stored = localStorage.getItem("go-daisy-theme");
    if (stored) {
      this.darkValue = stored === "dracula";
    }
    this.apply();
  }

  toggle() {
    this.darkValue = !this.darkValue;
    this.apply();
  }

  apply() {
    const theme = this.darkValue ? "dracula" : "nord";
    document.documentElement.setAttribute("data-theme", theme);
    localStorage.setItem("go-daisy-theme", theme);
  }
}

// ClipboardController — copy text to clipboard with feedback.
class ClipboardController extends Stimulus.Controller {
  static values = { text: String, feedback: { type: String, default: "Copied!" } };

  copy() {
    const text = this.textValue || this.element.textContent || "";
    navigator.clipboard.writeText(text).then(() => {
      const orig = this.element.textContent;
      this.element.textContent = this.feedbackValue;
      setTimeout(() => { this.element.textContent = orig; }, 2000);
    });
  }
}

// Export all controllers.
export {
  ModalController,
  DropdownController,
  TabsController,
  AccordionController,
  ThemeController,
  ClipboardController
};
