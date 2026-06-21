(function () {
  'use strict';

  // ---- ساعت زندهٔ ایستگاه (فقط نمایشی، تاریخ شمسی همچنان از سرور می‌آید) ----
  function pad(n) { return n < 10 ? '0' + n : '' + n; }

  function tickClock() {
    var els = document.querySelectorAll('[data-clock-time]');
    if (!els.length) return;
    var now = new Date();
    var str = pad(now.getHours()) + ':' + pad(now.getMinutes()) + ':' + pad(now.getSeconds());
    els.forEach(function (el) { el.textContent = str; });
  }

  function initClock() {
    tickClock();
    setInterval(tickClock, 1000);
  }

  // ---- محو خودکار پیام سیستم پس از چند ثانیه ----
  function initAlertAutoFade() {
    var alertEl = document.querySelector('[data-alert]');
    if (!alertEl) return;
    setTimeout(function () {
      alertEl.style.transition = 'opacity .5s ease, transform .5s ease';
      alertEl.style.opacity = '0';
      alertEl.style.transform = 'translateY(-6px)';
    }, 6500);
  }

  // ---- راه‌اندازی تقویم جلالی (در صورت بارگذاری موفق کتابخانه) ----
  function initDatepicker() {
    if (window.jalaliDatepicker && typeof window.jalaliDatepicker.startWatch === 'function') {
      window.jalaliDatepicker.startWatch({ hideAfterSelect: true });
    }
  }

  window.addEventListener('DOMContentLoaded', function () {
    initClock();
    initAlertAutoFade();
  });
  window.addEventListener('load', initDatepicker);
})();
