(function(){
if (window._gdPopoverInit) return;
window._gdPopoverInit = true;

var ATTR = {
  root: 'data-gd-popover-root',
  trigger: 'data-gd-popover-trigger',
  content: 'data-gd-popover-content',
  open: 'data-gd-popover-open',
  placement: 'data-gd-popover-placement',
  offset: 'data-gd-popover-offset',
  type: 'data-gd-popover-type',
  arrow: 'data-gd-popover-arrow',
  exclusive: 'data-gd-popover-exclusive',
  disableClickaway: 'data-gd-popover-disable-clickaway',
};

var hoverTimeouts = new WeakMap();
var pointerInside = new WeakMap();

function getContent(trigger) {
  var root = trigger.closest('[' + ATTR.root + ']');
  if (!root) return null;
  return root.querySelector('[' + ATTR.content + ']');
}

function getArrow(content) {
  return content.querySelector('[' + ATTR.arrow + ']');
}

function closeContent(content) {
  if (!content) return;
  content.hidePopover();
  content.setAttribute(ATTR.open, 'false');
  hoverClear(content);
}

function hoverClear(content) {
  var to = hoverTimeouts.get(content);
  if (to) { clearTimeout(to); hoverTimeouts.delete(content); }
}

function positionContent(content, trigger) {
  var placement = content.getAttribute(ATTR.placement) || 'bottom';
  var offset = parseInt(content.getAttribute(ATTR.offset)) || 8;
  var triggerRect = trigger.getBoundingClientRect();
  var arrow = getArrow(content);

  requestAnimationFrame(function(){
    var contentRect = content.getBoundingClientRect();
    var vw = window.innerWidth;
    var vh = window.innerHeight;
    var x, y;
    var arrowX = '', arrowY = '';

    var fits = {};
    fits.bottom = triggerRect.bottom + offset + contentRect.height <= vh;
    fits.top = triggerRect.top - offset - contentRect.height >= 0;
    fits.right = triggerRect.right + offset + contentRect.width <= vw;
    fits.left = triggerRect.left - offset - contentRect.width >= 0;

    if (placement === 'bottom' || placement === 'top') {
      if (placement === 'bottom' && !fits.bottom && fits.top) placement = 'top';
      else if (placement === 'top' && !fits.top && fits.bottom) placement = 'bottom';
    } else if (placement === 'left' || placement === 'right') {
      if (placement === 'right' && !fits.right && fits.left) placement = 'left';
      else if (placement === 'left' && !fits.left && fits.right) placement = 'right';
    }

    switch (placement) {
    case 'bottom':
      x = triggerRect.left + triggerRect.width / 2 - contentRect.width / 2;
      y = triggerRect.bottom + offset;
      arrowX = triggerRect.left + triggerRect.width / 2 - contentRect.left - 4 + 'px';
      arrowY = (y - contentRect.top - 4) + 'px';
      break;
    case 'top':
      x = triggerRect.left + triggerRect.width / 2 - contentRect.width / 2;
      y = triggerRect.top - contentRect.height - offset;
      arrowX = triggerRect.left + triggerRect.width / 2 - contentRect.left - 4 + 'px';
      arrowY = (triggerRect.top - contentRect.top - 4) + 'px';
      break;
    case 'right':
      x = triggerRect.right + offset;
      y = triggerRect.top + triggerRect.height / 2 - contentRect.height / 2;
      arrowX = (x - contentRect.left - 4) + 'px';
      arrowY = triggerRect.top + triggerRect.height / 2 - contentRect.top - 4 + 'px';
      break;
    case 'left':
      x = triggerRect.left - contentRect.width - offset;
      y = triggerRect.top + triggerRect.height / 2 - contentRect.height / 2;
      arrowX = (triggerRect.left - contentRect.left - 4) + 'px';
      arrowY = triggerRect.top + triggerRect.height / 2 - contentRect.top - 4 + 'px';
      break;
    }

    x = Math.max(4, Math.min(x, vw - contentRect.width - 4));
    y = Math.max(4, Math.min(y, vh - contentRect.height - 4));

    if (arrow) {
      if (arrowX) arrow.style.left = arrowX;
      if (arrowY) arrow.style.top = arrowY;
      arrow.style.visibility = '';
      if (placement === 'top') {
        arrow.style.setProperty('--gd-arrow-rotate', '225deg');
        arrow.style.bottom = '-4px';
        arrow.style.top = '';
      } else if (placement === 'bottom') {
        arrow.style.setProperty('--gd-arrow-rotate', '45deg');
        arrow.style.top = '-4px';
        arrow.style.bottom = '';
      } else if (placement === 'left') {
        arrow.style.setProperty('--gd-arrow-rotate', '135deg');
        arrow.style.right = '-4px';
        arrow.style.left = '';
        arrow.style.top = '';
      } else if (placement === 'right') {
        arrow.style.setProperty('--gd-arrow-rotate', '315deg');
        arrow.style.left = '-4px';
        arrow.style.right = '';
        arrow.style.top = '';
      }
    }

    content.style.transform = 'translate(' + Math.round(x) + 'px, ' + Math.round(y) + 'px)';
    content.setAttribute(ATTR.placement, placement);
    content.style.position = 'fixed';
  });
}

function open(trigger) {
  var content = getContent(trigger);
  if (!content) return;
  if (content.getAttribute(ATTR.open) === 'true') return;

  if (content.getAttribute(ATTR.exclusive) !== null) {
    var root = trigger.closest('[' + ATTR.root + ']');
    var scope = root ? root.parentElement : document;
    scope.querySelectorAll('[' + ATTR.content + '][' + ATTR.open + '="true"]').forEach(function(c) {
      if (c !== content) closeContent(c);
    });
  }

  content.showPopover();
  content.setAttribute(ATTR.open, 'true');
  positionContent(content, trigger);

  var refresh = function(){ positionContent(content, trigger); };
  window.addEventListener('scroll', refresh, {passive: true, once: true});
  window.addEventListener('resize', refresh, {passive: true, once: true});
}

function close(trigger) {
  var content = getContent(trigger);
  closeContent(content);
}

function toggle(trigger) {
  var content = getContent(trigger);
  if (content && content.getAttribute(ATTR.open) === 'true') {
    closeContent(content);
  } else {
    open(trigger);
  }
}

function handleClick(e) {
  var trigger = e.target.closest('[' + ATTR.trigger + ']');
  if (!trigger) {
    closeAll(e.target);
    return;
  }
  var content = getContent(trigger);
  if (!content) return;

  if (content.getAttribute(ATTR.disableClickaway) === null) {
    var clickedInside = e.target.closest('[' + ATTR.content + ']');
    if (!clickedInside || e.target.closest('a,button,[role="menuitem"],[data-gd-close]')) {
      if (content.getAttribute(ATTR.open) === 'true') {
        closeContent(content);
      } else {
        open(trigger);
      }
    }
  } else {
    toggle(trigger);
  }
}

function closeAll(exceptElement) {
  document.querySelectorAll('[' + ATTR.content + '][' + ATTR.open + '="true"]').forEach(function(c) {
    if (exceptElement && c.contains(exceptElement)) return;
    closeContent(c);
  });
}

document.addEventListener('click', handleClick, {passive: false});
document.addEventListener('keydown', function(e) {
  if (e.key === 'Escape') closeAll();
}, {passive: false});

document.addEventListener('pointerover', function(e) {
  var trigger = e.target.closest('[' + ATTR.trigger + ']');
  if (!trigger || trigger.getAttribute(ATTR.type) !== 'hover') return;
  var content = getContent(trigger);
  if (!content) return;

  pointerInside.set(content, true);
  hoverClear(content);
  var to = setTimeout(function(){ open(trigger); }, 150);
  hoverTimeouts.set(content, to);
}, {passive: true});

document.addEventListener('pointerout', function(e) {
  var trigger = e.target.closest('[' + ATTR.trigger + ']');
  if (!trigger || trigger.getAttribute(ATTR.type) !== 'hover') return;
  var content = getContent(trigger);
  if (!content) return;
  pointerInside.set(content, false);

  hoverClear(content);
  var to = setTimeout(function(){
    if (!pointerInside.get(content)) closeContent(content);
  }, 200);
  hoverTimeouts.set(content, to);
}, {passive: true});

window.goDaisy = window.goDaisy || {};
window.goDaisy.popover = {
  open: function(trigger) { open(trigger); },
  close: function(trigger) { close(trigger); },
  closeAll: function() { closeAll(); },
  toggle: function(trigger) { toggle(trigger); },
};

})();
