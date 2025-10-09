// Alpine.js コンポーネント

// アプリケーション状態
function appState() {
    return {
        // 既存のapp.jsのstateと統合
    }
}

// トースト通知マネージャー
function toastManager() {
    return {
        toasts: [],
        nextId: 1,

        addToast(message, type = 'info', duration = 4000) {
            const id = this.nextId++;
            const toast = {
                id,
                message,
                type,
                show: true
            };

            this.toasts.push(toast);

            // 自動削除
            setTimeout(() => {
                this.removeToast(id);
            }, duration);
        },

        removeToast(id) {
            const index = this.toasts.findIndex(t => t.id === id);
            if (index !== -1) {
                this.toasts[index].show = false;
                // アニメーション後に配列から削除
                setTimeout(() => {
                    this.toasts = this.toasts.filter(t => t.id !== id);
                }, 300);
            }
        },

        success(message, duration) {
            this.addToast(message, 'success', duration);
        },

        error(message, duration) {
            this.addToast(message, 'error', duration);
        },

        info(message, duration) {
            this.addToast(message, 'info', duration);
        },

        warning(message, duration) {
            this.addToast(message, 'warning', duration);
        }
    }
}

// グローバルトースト関数
window.toast = {
    success: (message, duration = 4000) => {
        Alpine.store('toastManager')?.addToast(message, 'success', duration);
        // フォールバック
        const toastContainer = document.querySelector('[x-data*="toastManager"]');
        if (toastContainer && toastContainer.__x) {
            toastContainer.__x.$data.addToast(message, 'success', duration);
        }
    },
    error: (message, duration = 5000) => {
        const toastContainer = document.querySelector('[x-data*="toastManager"]');
        if (toastContainer && toastContainer.__x) {
            toastContainer.__x.$data.addToast(message, 'error', duration);
        }
    },
    info: (message, duration = 4000) => {
        const toastContainer = document.querySelector('[x-data*="toastManager"]');
        if (toastContainer && toastContainer.__x) {
            toastContainer.__x.$data.addToast(message, 'info', duration);
        }
    },
    warning: (message, duration = 4000) => {
        const toastContainer = document.querySelector('[x-data*="toastManager"]');
        if (toastContainer && toastContainer.__x) {
            toastContainer.__x.$data.addToast(message, 'warning', duration);
        }
    }
};

// ファイルタイプ別アイコン
window.getFileIcon = function(filename) {
    const ext = filename.split('.').pop().toLowerCase();

    const iconMap = {
        // 画像
        'jpg': { icon: '🖼️', color: 'text-green-500', bg: 'bg-green-50 dark:bg-green-900/20' },
        'jpeg': { icon: '🖼️', color: 'text-green-500', bg: 'bg-green-50 dark:bg-green-900/20' },
        'png': { icon: '🖼️', color: 'text-green-500', bg: 'bg-green-50 dark:bg-green-900/20' },
        'gif': { icon: '🖼️', color: 'text-green-500', bg: 'bg-green-50 dark:bg-green-900/20' },
        'webp': { icon: '🖼️', color: 'text-green-500', bg: 'bg-green-50 dark:bg-green-900/20' },
        'svg': { icon: '🎨', color: 'text-purple-500', bg: 'bg-purple-50 dark:bg-purple-900/20' },

        // 動画
        'mp4': { icon: '🎬', color: 'text-red-500', bg: 'bg-red-50 dark:bg-red-900/20' },
        'mov': { icon: '🎬', color: 'text-red-500', bg: 'bg-red-50 dark:bg-red-900/20' },
        'avi': { icon: '🎬', color: 'text-red-500', bg: 'bg-red-50 dark:bg-red-900/20' },
        'mkv': { icon: '🎬', color: 'text-red-500', bg: 'bg-red-50 dark:bg-red-900/20' },
        'webm': { icon: '🎬', color: 'text-red-500', bg: 'bg-red-50 dark:bg-red-900/20' },

        // 音声
        'mp3': { icon: '🎵', color: 'text-pink-500', bg: 'bg-pink-50 dark:bg-pink-900/20' },
        'wav': { icon: '🎵', color: 'text-pink-500', bg: 'bg-pink-50 dark:bg-pink-900/20' },
        'flac': { icon: '🎵', color: 'text-pink-500', bg: 'bg-pink-50 dark:bg-pink-900/20' },
        'm4a': { icon: '🎵', color: 'text-pink-500', bg: 'bg-pink-50 dark:bg-pink-900/20' },

        // ドキュメント
        'pdf': { icon: '📄', color: 'text-red-600', bg: 'bg-red-50 dark:bg-red-900/20' },
        'doc': { icon: '📝', color: 'text-blue-600', bg: 'bg-blue-50 dark:bg-blue-900/20' },
        'docx': { icon: '📝', color: 'text-blue-600', bg: 'bg-blue-50 dark:bg-blue-900/20' },
        'txt': { icon: '📝', color: 'text-gray-600', bg: 'bg-gray-50 dark:bg-gray-900/20' },
        'md': { icon: '📝', color: 'text-gray-600', bg: 'bg-gray-50 dark:bg-gray-900/20' },

        // 表計算
        'xls': { icon: '📊', color: 'text-green-600', bg: 'bg-green-50 dark:bg-green-900/20' },
        'xlsx': { icon: '📊', color: 'text-green-600', bg: 'bg-green-50 dark:bg-green-900/20' },
        'csv': { icon: '📊', color: 'text-green-600', bg: 'bg-green-50 dark:bg-green-900/20' },

        // プレゼンテーション
        'ppt': { icon: '📊', color: 'text-orange-600', bg: 'bg-orange-50 dark:bg-orange-900/20' },
        'pptx': { icon: '📊', color: 'text-orange-600', bg: 'bg-orange-50 dark:bg-orange-900/20' },

        // 圧縮
        'zip': { icon: '🗜️', color: 'text-yellow-600', bg: 'bg-yellow-50 dark:bg-yellow-900/20' },
        'rar': { icon: '🗜️', color: 'text-yellow-600', bg: 'bg-yellow-50 dark:bg-yellow-900/20' },
        '7z': { icon: '🗜️', color: 'text-yellow-600', bg: 'bg-yellow-50 dark:bg-yellow-900/20' },
        'tar': { icon: '🗜️', color: 'text-yellow-600', bg: 'bg-yellow-50 dark:bg-yellow-900/20' },
        'gz': { icon: '🗜️', color: 'text-yellow-600', bg: 'bg-yellow-50 dark:bg-yellow-900/20' },

        // コード
        'js': { icon: '⚡', color: 'text-yellow-500', bg: 'bg-yellow-50 dark:bg-yellow-900/20' },
        'ts': { icon: '⚡', color: 'text-blue-500', bg: 'bg-blue-50 dark:bg-blue-900/20' },
        'py': { icon: '🐍', color: 'text-blue-400', bg: 'bg-blue-50 dark:bg-blue-900/20' },
        'go': { icon: '🔷', color: 'text-cyan-500', bg: 'bg-cyan-50 dark:bg-cyan-900/20' },
        'java': { icon: '☕', color: 'text-red-600', bg: 'bg-red-50 dark:bg-red-900/20' },
        'html': { icon: '🌐', color: 'text-orange-500', bg: 'bg-orange-50 dark:bg-orange-900/20' },
        'css': { icon: '🎨', color: 'text-blue-500', bg: 'bg-blue-50 dark:bg-blue-900/20' },
        'json': { icon: '{}', color: 'text-gray-600', bg: 'bg-gray-50 dark:bg-gray-900/20' },
        'xml': { icon: '<>', color: 'text-orange-600', bg: 'bg-orange-50 dark:bg-orange-900/20' },

        // デフォルト
        'default': { icon: '📁', color: 'text-gray-500', bg: 'bg-gray-50 dark:bg-gray-900/20' }
    };

    return iconMap[ext] || iconMap['default'];
};

// SVGアイコン（拡張版 - 絵文字をSVGに置き換え）
window.getFileIconSVG = function(filename) {
    const ext = filename.split('.').pop().toLowerCase();

    const iconConfig = {
        // 画像
        'jpg': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>', color: 'text-green-500', bg: 'bg-green-50 dark:bg-green-900/20' },
        'jpeg': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>', color: 'text-green-500', bg: 'bg-green-50 dark:bg-green-900/20' },
        'png': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>', color: 'text-green-500', bg: 'bg-green-50 dark:bg-green-900/20' },
        'gif': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 4v16M17 4v16M3 8h4m10 0h4M3 12h18M3 16h4m10 0h4M4 20h16a1 1 0 001-1V5a1 1 0 00-1-1H4a1 1 0 00-1 1v14a1 1 0 001 1z"/></svg>', color: 'text-purple-500', bg: 'bg-purple-50 dark:bg-purple-900/20' },
        'webp': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>', color: 'text-green-500', bg: 'bg-green-50 dark:bg-green-900/20' },
        'svg': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 21a4 4 0 01-4-4V5a2 2 0 012-2h4a2 2 0 012 2v12a4 4 0 01-4 4zm0 0h12a2 2 0 002-2v-4a2 2 0 00-2-2h-2.343M11 7.343l1.657-1.657a2 2 0 012.828 0l2.829 2.829a2 2 0 010 2.828l-8.486 8.485M7 17h.01"/></svg>', color: 'text-purple-500', bg: 'bg-purple-50 dark:bg-purple-900/20' },

        // 動画
        'mp4': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z"/></svg>', color: 'text-red-500', bg: 'bg-red-50 dark:bg-red-900/20' },
        'mov': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z"/></svg>', color: 'text-red-500', bg: 'bg-red-50 dark:bg-red-900/20' },
        'avi': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z"/></svg>', color: 'text-red-500', bg: 'bg-red-50 dark:bg-red-900/20' },
        'mkv': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z"/></svg>', color: 'text-red-500', bg: 'bg-red-50 dark:bg-red-900/20' },
        'webm': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z"/></svg>', color: 'text-red-500', bg: 'bg-red-50 dark:bg-red-900/20' },

        // 音声
        'mp3': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3"/></svg>', color: 'text-pink-500', bg: 'bg-pink-50 dark:bg-pink-900/20' },
        'wav': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3"/></svg>', color: 'text-pink-500', bg: 'bg-pink-50 dark:bg-pink-900/20' },
        'flac': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3"/></svg>', color: 'text-pink-500', bg: 'bg-pink-50 dark:bg-pink-900/20' },
        'm4a': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3"/></svg>', color: 'text-pink-500', bg: 'bg-pink-50 dark:bg-pink-900/20' },

        // ドキュメント
        'pdf': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z"/></svg>', color: 'text-red-600', bg: 'bg-red-50 dark:bg-red-900/20' },
        'doc': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/></svg>', color: 'text-blue-600', bg: 'bg-blue-50 dark:bg-blue-900/20' },
        'docx': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/></svg>', color: 'text-blue-600', bg: 'bg-blue-50 dark:bg-blue-900/20' },
        'txt': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/></svg>', color: 'text-gray-600', bg: 'bg-gray-50 dark:bg-gray-700/20' },
        'md': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/></svg>', color: 'text-gray-600', bg: 'bg-gray-50 dark:bg-gray-700/20' },

        // 表計算
        'xls': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 10h18M3 14h18m-9-4v8m-7 0h14a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z"/></svg>', color: 'text-green-600', bg: 'bg-green-50 dark:bg-green-900/20' },
        'xlsx': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 10h18M3 14h18m-9-4v8m-7 0h14a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z"/></svg>', color: 'text-green-600', bg: 'bg-green-50 dark:bg-green-900/20' },
        'csv': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 10h18M3 14h18m-9-4v8m-7 0h14a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z"/></svg>', color: 'text-green-600', bg: 'bg-green-50 dark:bg-green-900/20' },

        // 圧縮
        'zip': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4"/></svg>', color: 'text-yellow-600', bg: 'bg-yellow-50 dark:bg-yellow-900/20' },
        'rar': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4"/></svg>', color: 'text-yellow-600', bg: 'bg-yellow-50 dark:bg-yellow-900/20' },
        '7z': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4"/></svg>', color: 'text-yellow-600', bg: 'bg-yellow-50 dark:bg-yellow-900/20' },
        'tar': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4"/></svg>', color: 'text-yellow-600', bg: 'bg-yellow-50 dark:bg-yellow-900/20' },
        'gz': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4"/></svg>', color: 'text-yellow-600', bg: 'bg-yellow-50 dark:bg-yellow-900/20' },

        // コード
        'js': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"/></svg>', color: 'text-yellow-500', bg: 'bg-yellow-50 dark:bg-yellow-900/20' },
        'ts': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"/></svg>', color: 'text-blue-500', bg: 'bg-blue-50 dark:bg-blue-900/20' },
        'py': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"/></svg>', color: 'text-blue-400', bg: 'bg-blue-50 dark:bg-blue-900/20' },
        'go': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"/></svg>', color: 'text-cyan-500', bg: 'bg-cyan-50 dark:bg-cyan-900/20' },
        'java': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"/></svg>', color: 'text-red-600', bg: 'bg-red-50 dark:bg-red-900/20' },
        'html': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"/></svg>', color: 'text-orange-500', bg: 'bg-orange-50 dark:bg-orange-900/20' },
        'css': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 21a4 4 0 01-4-4V5a2 2 0 012-2h4a2 2 0 012 2v12a4 4 0 01-4 4zm0 0h12a2 2 0 002-2v-4a2 2 0 00-2-2h-2.343M11 7.343l1.657-1.657a2 2 0 012.828 0l2.829 2.829a2 2 0 010 2.828l-8.486 8.485M7 17h.01"/></svg>', color: 'text-blue-500', bg: 'bg-blue-50 dark:bg-blue-900/20' },
        'json': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"/></svg>', color: 'text-gray-600', bg: 'bg-gray-50 dark:bg-gray-700/20' },
        'xml': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"/></svg>', color: 'text-orange-600', bg: 'bg-orange-50 dark:bg-orange-900/20' },

        // デフォルト
        'default': { svg: '<svg class="w-full h-full" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z"/></svg>', color: 'text-gray-500', bg: 'bg-gray-50 dark:bg-gray-700/20' }
    };

    return iconConfig[ext] || iconConfig['default'];
};
