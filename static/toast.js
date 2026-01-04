/* Toast Notification Logic */
const Toast = {
    init() {
        // Ensure body exists
        if (!document.body) {
            document.addEventListener('DOMContentLoaded', () => this.init());
            return;
        }

        if (!document.getElementById('toast-container')) {
            const container = document.createElement('div');
            container.id = 'toast-container';
            container.className = 'toast-container';
            document.body.appendChild(container);
        }
        this.checkUrlParams();
    },

    show(message, type = 'success') {
        // Ensure container exists (lazy init)
        if (!document.getElementById('toast-container')) {
            this.init();
        }

        const container = document.getElementById('toast-container');
        if (!container) return; // Should not happen

        const toast = document.createElement('div');
        toast.className = `toast ${type}`;

        // Simple Icon Mapping
        const icon = type === 'success' ? '✅' : (type === 'error' ? '❌' : 'ℹ️');

        toast.innerHTML = `
            <div class="toast-icon">${icon}</div>
            <div class="toast-message">${message}</div>
        `;

        container.appendChild(toast);

        // Trigger animation (allow reflow)
        requestAnimationFrame(() => {
            toast.classList.add('toast-visible');
        });

        // Auto remove after 4 seconds
        setTimeout(() => {
            toast.classList.remove('toast-visible');
            toast.addEventListener('transitionend', () => toast.remove());
        }, 4000);
    },

    checkUrlParams() {
        const urlParams = new URLSearchParams(window.location.search);
        const success = urlParams.get('success');
        const error = urlParams.get('error');

        if (success) {
            this.show(success, 'success');
            this.cleanUrl('success');
        }
        if (error) {
            this.show(error, 'error');
            this.cleanUrl('error');
        }
    },

    cleanUrl(param) {
        const url = new URL(window.location);
        url.searchParams.delete(param);
        window.history.replaceState({}, '', url);
    }
};

// Initialize immediately if possible, or wait
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => Toast.init());
} else {
    Toast.init();
}

// Expose to window for inline scripts
window.showToast = (msg, type) => Toast.show(msg, type);

// HTMX Integration (Manual Header Parsing for Robustness)
// Use document instead of document.body to avoid null reference if script loaded in head
document.addEventListener('htmx:afterOnLoad', function (evt) {
    const xhr = evt.detail.xhr;
    if (!xhr) return;

    // Check standard HTMX trigger headers
    const headerNames = ['HX-Trigger', 'HX-Trigger-After-Swap', 'HX-Trigger-After-Settle'];

    headerNames.forEach(name => {
        const headerVal = xhr.getResponseHeader(name);
        if (headerVal) {
            try {
                const triggers = JSON.parse(headerVal);
                // Look for 'showMessage' key specifically
                if (triggers.showMessage) {
                    const val = triggers.showMessage;
                    let msg = val;
                    let type = 'success';

                    if (typeof val === 'object') {
                        msg = val.message || 'Action executed';
                        type = val.type || 'success';
                    }

                    console.log(`[Toast] HTMX Trigger: ${msg} (${type})`);
                    Toast.show(msg, type);
                }
            } catch (e) {
                console.error("[Toast] Error parsing HTMX header:", e);
            }
        }
    });
});

console.log("Toast System Initialized (v3-Safe)");
