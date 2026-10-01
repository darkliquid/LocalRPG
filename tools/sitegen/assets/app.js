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

  document.addEventListener('DOMContentLoaded', function () {
    initGallery();
    initCopyButtons();
  });
})();
