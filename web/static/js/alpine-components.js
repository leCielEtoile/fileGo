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

// SVGアイコン（より詳細な表現）
window.getFileIconSVG = function(filename) {
    const ext = filename.split('.').pop().toLowerCase();

    const svgIcons = {
        // 画像
        'jpg': '<svg class="w-8 h-8" fill="currentColor" viewBox="0 0 20 20"><path fill-rule="evenodd" d="M4 3a2 2 0 00-2 2v10a2 2 0 002 2h12a2 2 0 002-2V5a2 2 0 00-2-2H4zm12 12H4l4-8 3 6 2-4 3 6z" clip-rule="evenodd"/></svg>',
        'png': '<svg class="w-8 h-8" fill="currentColor" viewBox="0 0 20 20"><path fill-rule="evenodd" d="M4 3a2 2 0 00-2 2v10a2 2 0 002 2h12a2 2 0 002-2V5a2 2 0 00-2-2H4zm12 12H4l4-8 3 6 2-4 3 6z" clip-rule="evenodd"/></svg>',

        // 動画
        'mp4': '<svg class="w-8 h-8" fill="currentColor" viewBox="0 0 20 20"><path d="M2 6a2 2 0 012-2h6a2 2 0 012 2v8a2 2 0 01-2 2H4a2 2 0 01-2-2V6zM14.553 7.106A1 1 0 0014 8v4a1 1 0 00.553.894l2 1A1 1 0 0018 13V7a1 1 0 00-1.447-.894l-2 1z"/></svg>',

        // PDF
        'pdf': '<svg class="w-8 h-8" fill="currentColor" viewBox="0 0 20 20"><path fill-rule="evenodd" d="M4 4a2 2 0 012-2h4.586A2 2 0 0112 2.586L15.414 6A2 2 0 0116 7.414V16a2 2 0 01-2 2H6a2 2 0 01-2-2V4z" clip-rule="evenodd"/></svg>',

        // 圧縮
        'zip': '<svg class="w-8 h-8" fill="currentColor" viewBox="0 0 20 20"><path d="M4 3a2 2 0 100 4h12a2 2 0 100-4H4z"/><path fill-rule="evenodd" d="M3 8h14v7a2 2 0 01-2 2H5a2 2 0 01-2-2V8zm5 3a1 1 0 011-1h2a1 1 0 110 2H9a1 1 0 01-1-1z" clip-rule="evenodd"/></svg>',

        // デフォルト
        'default': '<svg class="w-8 h-8" fill="currentColor" viewBox="0 0 20 20"><path fill-rule="evenodd" d="M4 4a2 2 0 012-2h4.586A2 2 0 0112 2.586L15.414 6A2 2 0 0116 7.414V16a2 2 0 01-2 2H6a2 2 0 01-2-2V4zm2 6a1 1 0 011-1h6a1 1 0 110 2H7a1 1 0 01-1-1zm1 3a1 1 0 100 2h6a1 1 0 100-2H7z" clip-rule="evenodd"/></svg>'
    };

    return svgIcons[ext] || svgIcons['default'];
};
