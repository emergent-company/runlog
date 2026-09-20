// go-daisy-gantt — Gantt chart custom element wrapping frappe-gantt
// Usage: <go-daisy-gantt data-tasks='[{"id":"1","name":"Design","start":"2025-01-01","end":"2025-01-05"}]'></go-daisy-gantt>
//
// Attributes:
//   data-tasks — JSON array of {id, name, start, end, progress?, dependencies?}
//   data-view-mode — "Day", "Week", "Month", "Quarter Day", "Half Day" (default "Day")
//
// Requires: frappe-gantt CSS and JS loaded on the page.

(function () {
  class GoDaisyGantt extends HTMLElement {
    constructor() {
      super();
      this._gantt = null;
    }

    connectedCallback() {
      if (typeof Gantt === 'undefined') {
        console.warn('go-daisy-gantt: frappe-gantt not loaded. Include frappe-gantt.js and frappe-gantt.css on the page.');
        return;
      }
      this._init();
    }

    disconnectedCallback() {
      if (this._gantt && typeof this._gantt.destroy === 'function') {
        try { this._gantt.destroy(); } catch(e) {}
      }
    }

    _init() {
      let tasks = [];
      try {
        tasks = JSON.parse(this.dataset.tasks || '[]');
      } catch (e) {
        tasks = [];
      }

      const viewMode = this.dataset.viewMode || 'Day';

      this.innerHTML = '<svg id="go-daisy-gantt-svg"></svg>';

      this._gantt = new Gantt('#go-daisy-gantt-svg', tasks, {
        view_mode: viewMode,
        bar_height: 20,
        bar_corner_radius: 3,
        arrow_curve: 5,
        padding: 18,
        date_format: 'YYYY-MM-DD',
        language: 'en',
        custom_popup_html: function (task) {
          return '<div class="text-sm p-2"><b>' + task.name + '</b><br>' +
            task.start + ' — ' + task.end + '</div>';
        }
      });
    }

    get tasks() { return JSON.parse(this.dataset.tasks || '[]'); }
    set tasks(v) { this.dataset.tasks = JSON.stringify(v); if (this._gantt) { this._gantt.refresh(v); } }
  }

  if (!customElements.get('go-daisy-gantt')) {
    customElements.define('go-daisy-gantt', GoDaisyGantt);
  }
})();
