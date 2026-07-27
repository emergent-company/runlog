// go-daisy-calendar — lightweight date picker custom element
// Usage: <go-daisy-calendar value="2025-01-15" min="2025-01-01" locale="en-US"></go-daisy-calendar>
//
// Attributes:
//   value   — selected date as ISO string (YYYY-MM-DD)
//   min     — earliest selectable date (YYYY-MM-DD)
//   max     — latest selectable date (YYYY-MM-DD)
//   locale  — BCP 47 locale tag (default "en-US")
//
// Events:
//   change  — fired when user selects a date; event.detail.value = ISO string

(function () {
  const MONTHS = ['January','February','March','April','May','June','July','August','September','October','November','December'];
  const DAYS = ['Sun','Mon','Tue','Wed','Thu','Fri','Sat'];

  class GoDaisyCalendar extends HTMLElement {
    constructor() {
      super();
      this._value = null;
      this._viewDate = new Date();
      this._min = null;
      this._max = null;
      this._locale = 'en-US';
      this.attachShadow({ mode: 'open' });
    }

    static get observedAttributes() {
      return ['value', 'min', 'max', 'locale'];
    }

    connectedCallback() {
      this._parseAttrs();
      this._render();
    }

    attributeChangedCallback() {
      this._parseAttrs();
      this._render();
    }

    _parseAttrs() {
      if (this.hasAttribute('value')) this._value = new Date(this.getAttribute('value') + 'T00:00:00');
      if (this.hasAttribute('min')) this._min = new Date(this.getAttribute('min') + 'T00:00:00');
      if (this.hasAttribute('max')) this._max = new Date(this.getAttribute('max') + 'T00:00:00');
      if (this.hasAttribute('locale')) this._locale = this.getAttribute('locale');
      if (this._value && !isNaN(this._value)) this._viewDate = new Date(this._value);
    }

    _render() {
      const y = this._viewDate.getFullYear();
      const m = this._viewDate.getMonth();
      const first = new Date(y, m, 1);
      const last = new Date(y, m + 1, 0);
      const startDow = first.getDay();
      const daysInMonth = last.getDate();
      const today = new Date();
      today.setHours(0, 0, 0, 0);

      const val = this._value && !isNaN(this._value) ? this._value : null;
      const selectedTime = val ? val.getTime() : 0;

      let html = '<style>:host{display:inline-block;font-family:inherit;background:var(--color-base-100,#fff);border:1px solid var(--color-base-300,#e5e7eb);border-radius:var(--radius-box,0.5rem);padding:0.75rem;color:var(--color-base-content,#1f2937);font-size:14px;user-select:none}.header{display:flex;align-items:center;justify-content:space-between;margin-bottom:0.5rem;font-weight:600}.header button{background:none;border:none;cursor:pointer;padding:0.25rem 0.5rem;border-radius:0.25rem;color:inherit}.header button:hover{background:var(--color-base-200,#f3f4f6)}.weekdays,.days{display:grid;grid-template-columns:repeat(7,1fr);gap:1px}.weekdays span{text-align:center;font-size:12px;color:var(--color-base-content, #6b7280);padding:0.25rem 0}.days button{aspect-ratio:1;display:flex;align-items:center;justify-content:center;border:none;background:none;cursor:pointer;border-radius:0.25rem;font-size:13px;color:inherit}.days button:hover:not(:disabled){background:var(--color-base-200,#f3f4f6)}.days button:disabled{opacity:0.35;cursor:default}.days button.selected{background:var(--color-primary,#0d9488);color:var(--color-primary-content,#fff)}.days button.today{border:1px solid var(--color-primary,#0d9488)}</style>';

      html += '<div class="header">';
      html += '<button id="prev">&larr;</button>';
      html += '<span>' + MONTHS[m] + ' ' + y + '</span>';
      html += '<button id="next">&rarr;</button>';
      html += '</div>';

      html += '<div class="weekdays">';
      for (let i = 0; i < 7; i++) {
        html += '<span>' + DAYS[i] + '</span>';
      }
      html += '</div>';

      html += '<div class="days">';
      for (let i = 0; i < startDow; i++) {
        html += '<span></span>';
      }
      for (let d = 1; d <= daysInMonth; d++) {
        const date = new Date(y, m, d);
        const time = date.getTime();
        const isSelected = val && time === selectedTime;
        const isToday = time === today.getTime();
        const isPast = this._min && time < this._min.getTime();
        const isFuture = this._max && time > this._max.getTime();
        const disabled = isPast || isFuture;
        html += '<button data-date="' + d + '"' +
          (isSelected ? ' class="selected"' : '') +
          (isToday && !isSelected ? ' class="today"' : '') +
          (disabled ? ' disabled' : '') +
          '>' + d + '</button>';
      }
      html += '</div>';

      this.shadowRoot.innerHTML = html;

      this.shadowRoot.getElementById('prev').addEventListener('click', () => {
        this._viewDate.setMonth(this._viewDate.getMonth() - 1);
        this._render();
      });
      this.shadowRoot.getElementById('next').addEventListener('click', () => {
        this._viewDate.setMonth(this._viewDate.getMonth() + 1);
        this._render();
      });

      const self = this;
      this.shadowRoot.querySelectorAll('.days button[data-date]').forEach(function (btn) {
        btn.addEventListener('click', function () {
          const d = parseInt(this.dataset.date);
          const selected = new Date(y, m, d);
          self._value = selected;
          self.setAttribute('value', selected.toISOString().split('T')[0]);
          self._render();
          self.dispatchEvent(new CustomEvent('change', {
            bubbles: true,
            detail: { value: selected.toISOString().split('T')[0] }
          }));
        });
      });
    }

    get value() { return this.getAttribute('value') || ''; }
    set value(v) { this.setAttribute('value', v); }
  }

  if (!customElements.get('go-daisy-calendar')) {
    customElements.define('go-daisy-calendar', GoDaisyCalendar);
  }
})();
