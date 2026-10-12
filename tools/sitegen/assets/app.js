/* Progressive enhancement for the LocalRPG showcase site. Everything here is
   optional: without JavaScript the pages are fully readable, only the
   screenshot lightbox and the copy buttons stop working. */

(function () {
  'use strict';

  /** Build the lightbox once, on first use, and keep it out of the DOM until then. */
  function createLightbox() {
    var box = document.createElement('div');
    box.className = 'lightbox';
    box.hidden = true;
    box.setAttribute('role', 'dialog');
    box.setAttribute('aria-modal', 'true');
    box.innerHTML = '<button type="button" class="lightbox__close" aria-label="Close">&times;</button><img alt="">';

    var image = box.querySelector('img');
    var close = function () {
      box.hidden = true;
      image.removeAttribute('src');
      document.body.style.overflow = '';
    };

    box.addEventListener('click', function (event) {
      if (event.target === box || event.target.classList.contains('lightbox__close')) {
        close();
      }
    });
    document.addEventListener('keydown', function (event) {
      if (event.key === 'Escape' && !box.hidden) {
        close();
      }
    });

    document.body.appendChild(box);

    return {
      open: function (src, alt) {
        image.src = src;
        image.alt = alt || '';
        box.hidden = false;
        document.body.style.overflow = 'hidden';
        box.querySelector('.lightbox__close').focus();
      }
    };
  }

  function initGallery() {
    var frames = document.querySelectorAll('.shot:not(.is-pending) .shot__frame img');
    if (!frames.length) {
      return;
    }

    var lightbox = null;
    frames.forEach(function (img) {
      img.parentElement.addEventListener('click', function () {
        if (!lightbox) {
          lightbox = createLightbox();
        }
        lightbox.open(img.currentSrc || img.src, img.alt);
      });
      img.parentElement.setAttribute('tabindex', '0');
      img.parentElement.setAttribute('role', 'button');
      img.parentElement.addEventListener('keydown', function (event) {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault();
          img.parentElement.click();
        }
      });
    });
  }

  function initCopyButtons() {
    document.querySelectorAll('.code[data-copy]').forEach(function (block) {
      block.addEventListener('click', function () {
        // A click that is really the end of a text selection should not copy.
        if (String(window.getSelection ? window.getSelection() : '') !== '') {
          return;
        }
        var text = block.textContent.replace(/\s*copy\s*$/, '').trim();
        if (!navigator.clipboard) {
          return;
        }
        navigator.clipboard.writeText(text).then(function () {
          block.classList.add('is-copied');
          window.setTimeout(function () {
            block.classList.remove('is-copied');
          }, 1600);
        });
      });
    });
  }

  /* Highlight the dock icon whose section is in view. Each participating dock
     item carries data-spy naming a section on the page, and each such section
     carries the same data-spy. A page without any landmark (the documentation
     articles) keeps the active icon the server rendered. */
  function initDockSpy() {
    var dock = document.querySelector('.dock');
    if (!dock) {
      return;
    }

    var items = {};
    dock.querySelectorAll('.dock__item[data-spy]').forEach(function (item) {
      items[item.getAttribute('data-spy')] = item;
    });

    // Only the page's own sections count: the dock anchors carry data-spy too,
    // and they sit in a fixed rail at the top of the window.
    var marks = Array.prototype.filter.call(document.querySelectorAll('#main [data-spy]'), function (el) {
      return items[el.getAttribute('data-spy')];
    });
    if (!marks.length) {
      return;
    }

    var frame = 0;
    var current = null;

    function apply() {
      frame = 0;

      // A section counts as current once its top crosses a reading line rather
      // than the very top of the window, so an icon lights up while its heading
      // is still coming into view.
      var line = window.innerHeight * 0.34;
      var found = null;
      marks.forEach(function (el) {
        if (el.getBoundingClientRect().top <= line) {
          found = el;
        }
      });
      if (!found) {
        found = marks[0];
      }

      // A last section shorter than the viewport never reaches the line, so the
      // end of the page settles on the final landmark instead.
      var atEnd = window.innerHeight + window.scrollY >= document.documentElement.scrollHeight - 4;
      if (atEnd) {
        found = marks[marks.length - 1];
      }

      if (found === current) {
        return;
      }
      current = found;

      var key = found.getAttribute('data-spy');
      Object.keys(items).forEach(function (name) {
        items[name].classList.toggle('is-active', name === key);
      });
    }

    function schedule() {
      if (!frame) {
        frame = window.requestAnimationFrame(apply);
      }
    }

    window.addEventListener('scroll', schedule, { passive: true });
    window.addEventListener('resize', schedule);
    schedule();
  }

  document.addEventListener('DOMContentLoaded', function () {
    initDockSpy();
    initGallery();
    initCopyButtons();
  });
})();
