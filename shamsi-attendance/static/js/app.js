document.addEventListener("DOMContentLoaded", function() {
    'use strict';

    // ---- ۱. ساعت زندهٔ پنل ----
    function pad(n) { return n < 10 ? '0' + n : '' + n; }
    function tickClock() {
        var els = document.querySelectorAll('[data-clock-time]');
        if (!els.length) return;
        var now = new Date();
        var str = pad(now.getHours()) + ':' + pad(now.getMinutes()) + ':' + pad(now.getSeconds());
        els.forEach(function (el) { el.textContent = str; });
    }
    tickClock();
    setInterval(tickClock, 1000);

    // ---- ۲. محو شدن نرم پیام‌های خطا و موفقیت (Alerts) ----
    var alertEl = document.querySelector('[data-alert]');
    if (alertEl) {
        setTimeout(function () {
            alertEl.style.transition = 'opacity .5s ease, transform .5s ease';
            alertEl.style.opacity = '0';
            alertEl.style.transform = 'translateY(-6px)';
            setTimeout(function() { alertEl.remove(); }, 500);
        }, 6500);
    }

    // ---- ۳. دراپ‌دان‌های جستجودار اختصاصی (کاملاً آفلاین و لوکال) ----
    function initCustomSearchableSelects() {
        // این سیستم به صورت هوشمند روی تمام دراپ‌دان‌های سیستم (به جز مولتی‌سلکت‌ها) می‌نشیند
        const selects = document.querySelectorAll('select:not([multiple])');
        
        selects.forEach(originalSelect => {
            // جلوگیری از دوباره ساخته شدن در صورت رفرش جزئی
            if (originalSelect.dataset.customSelect) return;
            originalSelect.dataset.customSelect = "true";

            // مخفی کردن تگ سلکت اصلی و حفظ آن برای ارسال فرم
            originalSelect.style.display = 'none';

            // ساخت کادر اصلی
            const wrapper = document.createElement('div');
            wrapper.className = 'select-search-wrapper';
            originalSelect.parentNode.insertBefore(wrapper, originalSelect);
            wrapper.appendChild(originalSelect);

            // ساخت دکمه کلیک‌پذیر
            const trigger = document.createElement('div');
            trigger.className = 'select-search-trigger';
            
            const updateTriggerText = () => {
                const selectedOpt = originalSelect.options[originalSelect.selectedIndex];
                trigger.textContent = selectedOpt ? selectedOpt.text : 'انتخاب کنید...';
            };
            updateTriggerText();
            wrapper.appendChild(trigger);

            // ساخت منوی کشویی بازشونده
            const dropdown = document.createElement('div');
            dropdown.className = 'select-search-dropdown';
            wrapper.appendChild(dropdown);

            // ساخت فیلد جستجو
            const searchInput = document.createElement('input');
            searchInput.type = 'text';
            searchInput.className = 'select-search-input';
            searchInput.placeholder = 'جستجو کنید...';
            dropdown.appendChild(searchInput);

            // ساخت محفظه گزینه‌ها
            const optionsContainer = document.createElement('div');
            optionsContainer.className = 'select-search-options';
            dropdown.appendChild(optionsContainer);

            // انتقال گزینه‌ها از سلکت اصلی به منوی کاستوم
            const optionElements = [];
            Array.from(originalSelect.options).forEach(opt => {
                const optEl = document.createElement('div');
                optEl.className = 'select-search-option';
                if (opt.selected) optEl.classList.add('selected');
                optEl.textContent = opt.text;
                
                optionsContainer.appendChild(optEl);
                optionElements.push({ el: optEl, text: opt.text.toLowerCase() });

                // رویداد کلیک روی هر گزینه
                optEl.addEventListener('click', (e) => {
                    e.stopPropagation();
                    // آپدیت کردن سلکت مخفی فرم
                    originalSelect.value = opt.value;
                    originalSelect.dispatchEvent(new Event('change'));
                    
                    // آپدیت کردن ظاهر (UI)
                    optionElements.forEach(o => o.el.classList.remove('selected'));
                    optEl.classList.add('selected');
                    updateTriggerText();
                    
                    // بستن دراپ‌دان
                    wrapper.classList.remove('open');
                });
            });

            // هماهنگی دوطرفه (اگر کدهای دیگر سلکت را تغییر دادند)
            originalSelect.addEventListener('change', () => {
                updateTriggerText();
                optionElements.forEach(o => {
                    if(o.el.textContent === originalSelect.options[originalSelect.selectedIndex]?.text) {
                        o.el.classList.add('selected');
                    } else {
                        o.el.classList.remove('selected');
                    }
                });
            });

            // باز و بسته کردن دراپ‌دان با انیمیشن
            trigger.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();
                const isOpen = wrapper.classList.contains('open');
                
                // بستن تمام دراپ‌دان‌های دیگر در صفحه
                document.querySelectorAll('.select-search-wrapper').forEach(w => w.classList.remove('open'));
                
                if (!isOpen) {
                    wrapper.classList.add('open');
                    searchInput.value = '';
                    optionElements.forEach(o => o.el.classList.remove('hidden'));
                    // هدایت خودکار موس به فیلد سرچ
                    setTimeout(() => searchInput.focus(), 50);
                }
            });

            // منطق جستجو هنگام تایپ کردن
            searchInput.addEventListener('click', e => e.stopPropagation());
            searchInput.addEventListener('input', (e) => {
                const query = e.target.value.toLowerCase();
                optionElements.forEach(o => {
                    if (o.text.includes(query)) {
                        o.el.classList.remove('hidden');
                    } else {
                        o.el.classList.add('hidden');
                    }
                });
            });
        });

        // بستن منو با کلیک روی هر جای خالیِ صفحه
        document.addEventListener('click', () => {
            document.querySelectorAll('.select-search-wrapper').forEach(w => w.classList.remove('open'));
        });
    }

    // فراخوانی سیستم دراپ‌دان
    initCustomSearchableSelects();
});