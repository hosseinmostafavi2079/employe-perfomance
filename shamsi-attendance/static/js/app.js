document.addEventListener("DOMContentLoaded", function() {
    'use strict';

    // ---- ۱. ساعت زندهٔ پنل ----
    const pad = n => n < 10 ? '0' + n : '' + n;
    const tickClock = () => {
        const els = document.querySelectorAll('[data-clock-time]');
        if (!els.length) return;
        const now = new Date();
        const str = `${pad(now.getHours())}:${pad(now.getMinutes())}:${pad(now.getSeconds())}`;
        els.forEach(el => el.textContent = str);
    };
    tickClock();
    setInterval(tickClock, 1000);

    // ---- ۲. محو شدن نرم پیام‌های خطا و موفقیت (Alerts) ----
    const alertEl = document.querySelector('[data-alert]');
    if (alertEl) {
        setTimeout(() => {
            alertEl.classList.add('fade-out');
            setTimeout(() => alertEl.remove(), 400);
        }, 6000);
    }

    // ---- ۳. تغییر دهنده پوسته (Theme Toggler) ----
    const themeBtn = document.getElementById('theme-toggle-btn');
    if (themeBtn) {
        themeBtn.addEventListener('click', () => {
            const root = document.documentElement;
            const isDark = root.classList.contains('theme-dark');
            if (isDark) {
                root.classList.replace('theme-dark', 'theme-light');
                localStorage.setItem('app-theme', 'light');
            } else {
                root.classList.replace('theme-light', 'theme-dark');
                localStorage.setItem('app-theme', 'dark');
            }
        });
    }

    // ---- ۴. دراپ‌دان‌های جستجودار اختصاصی (Custom Searchable Selects) ----
    function initCustomSearchableSelects() {
        const selects = document.querySelectorAll('select:not([multiple])');
        selects.forEach(originalSelect => {
            if (originalSelect.dataset.customSelect) return;
            originalSelect.dataset.customSelect = "true";
            originalSelect.style.display = 'none';

            const wrapper = document.createElement('div');
            wrapper.className = 'select-search-wrapper';
            originalSelect.parentNode.insertBefore(wrapper, originalSelect);
            wrapper.appendChild(originalSelect);

            const trigger = document.createElement('div');
            trigger.className = 'select-search-trigger';
            
            const updateTriggerText = () => {
                const selectedOpt = originalSelect.options[originalSelect.selectedIndex];
                trigger.textContent = selectedOpt ? selectedOpt.text : 'انتخاب کنید...';
            };
            updateTriggerText();
            wrapper.appendChild(trigger);

            const dropdown = document.createElement('div');
            dropdown.className = 'select-search-dropdown';
            wrapper.appendChild(dropdown);

            const searchInput = document.createElement('input');
            searchInput.type = 'text';
            searchInput.className = 'select-search-input';
            searchInput.placeholder = 'جستجو...';
            dropdown.appendChild(searchInput);

            const optionsContainer = document.createElement('div');
            optionsContainer.className = 'select-search-options';
            dropdown.appendChild(optionsContainer);

            const optionElements = [];
            Array.from(originalSelect.options).forEach(opt => {
                const optEl = document.createElement('div');
                optEl.className = 'select-search-option';
                if (opt.selected) optEl.classList.add('selected');
                optEl.textContent = opt.text;
                
                optionsContainer.appendChild(optEl);
                optionElements.push({ el: optEl, text: opt.text.toLowerCase() });

                optEl.addEventListener('click', (e) => {
                    e.stopPropagation();
                    originalSelect.value = opt.value;
                    originalSelect.dispatchEvent(new Event('change'));
                    
                    optionElements.forEach(o => o.el.classList.remove('selected'));
                    optEl.classList.add('selected');
                    updateTriggerText();
                    
                    wrapper.classList.remove('open');
                });
            });

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

            trigger.addEventListener('click', (e) => {
                e.preventDefault(); e.stopPropagation();
                const isOpen = wrapper.classList.contains('open');
                
                document.querySelectorAll('.select-search-wrapper').forEach(w => w.classList.remove('open'));
                
                if (!isOpen) {
                    wrapper.classList.add('open');
                    searchInput.value = '';
                    optionElements.forEach(o => o.el.classList.remove('hidden'));
                    setTimeout(() => searchInput.focus(), 50);
                }
            });

            searchInput.addEventListener('click', e => e.stopPropagation());
            searchInput.addEventListener('input', (e) => {
                const query = e.target.value.toLowerCase();
                optionElements.forEach(o => {
                    if (o.text.includes(query)) { o.el.classList.remove('hidden'); } 
                    else { o.el.classList.add('hidden'); }
                });
            });
        });

        document.addEventListener('click', () => {
            document.querySelectorAll('.select-search-wrapper').forEach(w => w.classList.remove('open'));
        });
    }

    initCustomSearchableSelects();
});