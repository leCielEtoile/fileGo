// アプリケーション状態
const state = {
    user: null,
    directories: [],
    selectedDirectory: null,
    files: [],
    eventSource: null
};

// ページ読み込み時
document.addEventListener('DOMContentLoaded', async () => {
    await checkAuth();
    setupEventListeners();
});

// 認証チェック
async function checkAuth() {
    try {
        console.log('認証チェック開始...');
        const response = await fetch('/api/user', {
            credentials: 'include'
        });

        console.log('認証レスポンス:', response.status);

        if (response.ok) {
            state.user = await response.json();
            console.log('認証成功:', state.user);
            showAppSection();
            await loadDirectories();
            connectSSE();
        } else {
            console.log('認証失敗: ログインが必要です');
            showLoginSection();
        }
    } catch (error) {
        console.error('認証チェックエラー:', error);
        showLoginSection();
    }
}

// ログインセクション表示
function showLoginSection() {
    document.getElementById('login-section').classList.remove('hidden');
    document.getElementById('app-section').classList.add('hidden');
}

// アプリケーションセクション表示
function showAppSection() {
    document.getElementById('login-section').classList.add('hidden');
    document.getElementById('app-section').classList.remove('hidden');

    // ユーザー情報表示（Tailwind スタイル）
    const userInfo = document.getElementById('user-info');
    userInfo.innerHTML = `
        <span class="text-white font-medium">${state.user.username}</span>
        <a href="/auth/logout" class="px-4 py-2 bg-red-500 hover:bg-red-600 text-white font-medium rounded-lg transition-colors">
            ログアウト
        </a>
    `;

    // ログイン成功トースト
    if (window.toast) {
        toast.success('ログインしました');
    }
}

// ディレクトリ一覧読み込み
async function loadDirectories() {
    try {
        const response = await fetch('/files/directories', {
            credentials: 'include'
        });

        if (response.ok) {
            const data = await response.json();
            state.directories = data.directories || [];
            renderDirectories();
        } else {
            if (window.toast) toast.error('ディレクトリの読み込みに失敗しました');
        }
    } catch (error) {
        console.error('ディレクトリ読み込みエラー:', error);
        addActivityLog('error', 'ディレクトリの読み込みに失敗しました');
        if (window.toast) toast.error('ディレクトリの読み込みに失敗しました');
    }
}

// ディレクトリ描画
function renderDirectories() {
    const container = document.getElementById('directory-list');

    if (state.directories.length === 0) {
        container.innerHTML = '<div class="col-span-full text-center py-8 text-gray-500 dark:text-gray-400"><p>アクセス可能なディレクトリがありません</p></div>';
        return;
    }

    container.innerHTML = state.directories.map(dir => `
        <div class="cursor-pointer p-5 rounded-xl border-2 transition-all transform hover:scale-105 ${
            state.selectedDirectory === dir.path
                ? 'border-discord-500 bg-gradient-to-br from-discord-50 to-discord-100 dark:from-discord-900/30 dark:to-discord-900/20 shadow-xl ring-4 ring-discord-500/20'
                : 'border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-700 hover:border-discord-400 hover:shadow-lg'
        }"
             onclick="selectDirectory('${dir.path}')">
            <div class="flex items-center gap-3 mb-4">
                <div class="flex-shrink-0 w-12 h-12 rounded-xl bg-discord-500/10 flex items-center justify-center">
                    <svg class="w-7 h-7 text-discord-500" fill="currentColor" viewBox="0 0 20 20">
                        <path d="M2 6a2 2 0 012-2h5l2 2h5a2 2 0 012 2v6a2 2 0 01-2 2H4a2 2 0 01-2-2V6z"/>
                    </svg>
                </div>
                <span class="font-bold text-xl text-gray-800 dark:text-white flex-1">${dir.path}</span>
                ${state.selectedDirectory === dir.path ? '<svg class="w-6 h-6 text-discord-500" fill="currentColor" viewBox="0 0 20 20"><path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd"/></svg>' : ''}
            </div>
            <div class="flex gap-2 flex-wrap">
                ${dir.permissions.map(p => {
                    const iconMap = {
                        'read': '<svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20"><path d="M10 12a2 2 0 100-4 2 2 0 000 4z"/><path fill-rule="evenodd" d="M.458 10C1.732 5.943 5.522 3 10 3s8.268 2.943 9.542 7c-1.274 4.057-5.064 7-9.542 7S1.732 14.057.458 10zM14 10a4 4 0 11-8 0 4 4 0 018 0z" clip-rule="evenodd"/></svg>',
                        'write': '<svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20"><path d="M13.586 3.586a2 2 0 112.828 2.828l-.793.793-2.828-2.828.793-.793zM11.379 5.793L3 14.172V17h2.828l8.38-8.379-2.83-2.828z"/></svg>',
                        'delete': '<svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20"><path fill-rule="evenodd" d="M9 2a1 1 0 00-.894.553L7.382 4H4a1 1 0 000 2v10a2 2 0 002 2h8a2 2 0 002-2V6a1 1 0 100-2h-3.382l-.724-1.447A1 1 0 0011 2H9zM7 8a1 1 0 012 0v6a1 1 0 11-2 0V8zm5-1a1 1 0 00-1 1v6a1 1 0 102 0V8a1 1 0 00-1-1z" clip-rule="evenodd"/></svg>'
                    };
                    const colorMap = {
                        'read': 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
                        'write': 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300',
                        'delete': 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
                    };
                    return `
                    <span class="px-3 py-1.5 text-xs font-bold rounded-lg flex items-center gap-1.5 ${colorMap[p] || 'bg-gray-100 text-gray-700'}">
                        ${iconMap[p] || ''}
                        ${p.toUpperCase()}
                    </span>
                    `;
                }).join('')}
            </div>
        </div>
    `).join('');
}

// ディレクトリ選択
async function selectDirectory(path) {
    state.selectedDirectory = path;
    renderDirectories();
    await loadFiles(path);
}

// ファイル一覧読み込み
async function loadFiles(directory) {
    try {
        const response = await fetch(`/files?directory=${encodeURIComponent(directory)}`, {
            credentials: 'include'
        });

        if (response.ok) {
            const data = await response.json();
            state.files = data.files || [];
            renderFiles();
        } else {
            if (window.toast) toast.error('ファイル一覧の取得に失敗しました');
        }
    } catch (error) {
        console.error('ファイル読み込みエラー:', error);
        addActivityLog('error', 'ファイル一覧の取得に失敗しました');
        if (window.toast) toast.error('ファイル一覧の取得に失敗しました');
    }
}

// ファイル一覧描画
function renderFiles() {
    const container = document.getElementById('files-list');

    if (state.files.length === 0) {
        container.innerHTML = '<div class="text-center py-12 text-gray-500 dark:text-gray-400"><p>ファイルがありません</p></div>';
        return;
    }

    const selectedDir = state.directories.find(d => d.path === state.selectedDirectory);
    const canDelete = selectedDir && selectedDir.permissions.includes('delete');

    // グリッド表示
    container.innerHTML = `
        <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
            ${state.files.map(file => {
                const filename = file.original_name || file.filename;
                const fileIcon = window.getFileIcon ? window.getFileIcon(filename) : { icon: '📁', color: 'text-gray-500', bg: 'bg-gray-50' };

                return `
                <div class="flex items-center gap-4 p-4 bg-white dark:bg-gray-700 rounded-xl border border-gray-200 dark:border-gray-600 hover:shadow-lg hover:border-discord-500 transition-all group">
                    <div class="flex-shrink-0 w-16 h-16 ${fileIcon.bg} rounded-xl flex items-center justify-center text-3xl shadow-sm">
                        ${fileIcon.icon}
                    </div>
                    <div class="flex-1 min-w-0">
                        <div class="font-bold text-gray-800 dark:text-white truncate text-lg mb-1" title="${filename}">${filename}</div>
                        <div class="flex items-center gap-3 text-sm text-gray-500 dark:text-gray-400">
                            <span class="flex items-center gap-1">
                                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z"/>
                                </svg>
                                ${formatFileSize(file.size)}
                            </span>
                            <span class="flex items-center gap-1">
                                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"/>
                                </svg>
                                ${formatDate(file.modified_at)}
                            </span>
                        </div>
                    </div>
                    <div class="flex flex-col gap-2">
                        <button onclick="downloadFile('${file.filename}')"
                                class="px-4 py-2 bg-discord-500 hover:bg-discord-600 text-white font-semibold rounded-lg transition-all transform hover:scale-105 flex items-center justify-center gap-2 whitespace-nowrap">
                            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/>
                            </svg>
                            <span class="hidden xl:inline">ダウンロード</span>
                        </button>
                        ${canDelete ? `
                            <button onclick="deleteFile('${file.filename}')"
                                    class="px-4 py-2 bg-red-500 hover:bg-red-600 text-white font-semibold rounded-lg transition-all transform hover:scale-105 flex items-center justify-center gap-2 whitespace-nowrap">
                                <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 1 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
                                </svg>
                                <span class="hidden xl:inline">削除</span>
                            </button>
                        ` : ''}
                    </div>
                </div>
                `;
            }).join('')}
        </div>
    `;
}

// イベントリスナー設定
function setupEventListeners() {
    // ドラッグ&ドロップ
    setupDragAndDrop();
}

// ドラッグ&ドロップ設定
function setupDragAndDrop() {
    const dropZone = document.getElementById('upload-drop-zone');
    const fileInput = document.getElementById('file-input');

    if (!dropZone || !fileInput) {
        console.error('Drop zone or file input not found');
        return;
    }

    // ドラッグイベント
    ['dragenter', 'dragover', 'dragleave', 'drop'].forEach(eventName => {
        dropZone.addEventListener(eventName, preventDefaults, false);
        document.body.addEventListener(eventName, preventDefaults, false);
    });

    function preventDefaults(e) {
        e.preventDefault();
        e.stopPropagation();
    }

    // ハイライト（Tailwindクラスを使用）
    ['dragenter', 'dragover'].forEach(eventName => {
        dropZone.addEventListener(eventName, () => {
            dropZone.classList.add('border-discord-500', 'bg-discord-50', 'dark:bg-discord-900/20', 'scale-105');
            dropZone.classList.remove('border-gray-300', 'dark:border-gray-600');
        });
    });

    ['dragleave', 'drop'].forEach(eventName => {
        dropZone.addEventListener(eventName, () => {
            dropZone.classList.remove('border-discord-500', 'bg-discord-50', 'dark:bg-discord-900/20', 'scale-105');
            dropZone.classList.add('border-gray-300', 'dark:border-gray-600');
        });
    });

    // ドロップ処理
    dropZone.addEventListener('drop', (e) => {
        const files = Array.from(e.dataTransfer.files);
        if (files.length > 0) {
            handleDroppedFiles(files);
        }
    });

    // ファイル選択時
    fileInput.addEventListener('change', (e) => {
        if (fileInput.files.length > 0) {
            handleDroppedFiles(Array.from(fileInput.files));
            fileInput.value = ''; // リセット
        }
    });
}

// ドロップされたファイルの処理
async function handleDroppedFiles(files) {
    if (!state.selectedDirectory) {
        if (window.toast) toast.warning('ディレクトリを選択してください');
        return;
    }

    const selectedDir = state.directories.find(d => d.path === state.selectedDirectory);
    const canWrite = selectedDir && selectedDir.permissions.includes('write');

    if (!canWrite) {
        if (window.toast) toast.error('このディレクトリへの書き込み権限がありません');
        return;
    }

    // 複数ファイルを順次アップロード
    for (const file of files) {
        await uploadSingleFile(file);
    }
}

// 単一ファイルアップロード（共通化）
async function uploadSingleFile(file) {
    console.log('アップロード開始:', file.name, formatFileSize(file.size));

    // 100MB以上はチャンクアップロード
    if (file.size > 100 * 1024 * 1024) {
        await uploadFileInChunks(file);
    } else {
        await uploadFileNormal(file);
    }
}

// 通常アップロード（リファクタリング）
async function uploadFileNormal(file) {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('directory', state.selectedDirectory);

    showProgress(true);
    setProgress(0);

    try {
        const response = await fetch('/files/upload', {
            method: 'POST',
            body: formData,
            credentials: 'include'
        });

        if (response.ok) {
            const data = await response.json();
            addActivityLog('upload', `${file.name} をアップロードしました`);
            if (window.toast) toast.success(`${file.name} のアップロードが完了しました`);
            setProgress(100);
            await loadFiles(state.selectedDirectory);
        } else {
            const error = await response.text();
            addActivityLog('error', `アップロード失敗: ${error}`);
            if (window.toast) toast.error(`アップロード失敗: ${error}`);
        }
    } catch (error) {
        console.error('アップロードエラー:', error);
        addActivityLog('error', 'アップロードに失敗しました');
        if (window.toast) toast.error('アップロードに失敗しました');
    } finally {
        setTimeout(() => showProgress(false), 500);
    }
}

// チャンクアップロード（リファクタリング）
async function uploadFileInChunks(file) {
    const chunkSize = 20 * 1024 * 1024; // 20MB
    const totalChunks = Math.ceil(file.size / chunkSize);

    showProgress(true);
    setProgress(0);

    try {
        // 初期化
        const initResponse = await fetch('/files/chunk/init', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                filename: file.name,
                directory: state.selectedDirectory,
                file_size: file.size,
                chunk_size: chunkSize
            }),
            credentials: 'include'
        });

        if (!initResponse.ok) {
            throw new Error('チャンクアップロードの初期化に失敗しました');
        }

        const { upload_id } = await initResponse.json();
        addActivityLog('upload', `チャンクアップロード開始: ${file.name}`);

        // 各チャンクをアップロード
        for (let i = 0; i < totalChunks; i++) {
            const start = i * chunkSize;
            const end = Math.min(start + chunkSize, file.size);
            const chunk = file.slice(start, end);

            const uploadResponse = await fetch(`/files/chunk/upload/${upload_id}?chunk_index=${i}`, {
                method: 'POST',
                body: chunk,
                credentials: 'include'
            });

            if (!uploadResponse.ok) {
                throw new Error(`チャンク ${i + 1} のアップロードに失敗しました`);
            }

            const progress = Math.round(((i + 1) / totalChunks) * 100);
            setProgress(progress);
        }

        // 完了
        const completeResponse = await fetch(`/files/chunk/complete/${upload_id}`, {
            method: 'POST',
            credentials: 'include'
        });

        if (!completeResponse.ok) {
            throw new Error('チャンクアップロードの完了に失敗しました');
        }

        addActivityLog('upload', `${file.name} のアップロードが完了しました`);
        if (window.toast) toast.success(`${file.name} のアップロードが完了しました`);
        await loadFiles(state.selectedDirectory);

    } catch (error) {
        console.error('チャンクアップロードエラー:', error);
        addActivityLog('error', error.message);
        if (window.toast) toast.error(error.message);
    } finally {
        setTimeout(() => showProgress(false), 500);
    }
}


// ファイルダウンロード
function downloadFile(filename) {
    const url = `/files/download/${encodeURIComponent(state.selectedDirectory)}/${encodeURIComponent(filename)}`;
    window.location.href = url;
    addActivityLog('download', `${filename} をダウンロードしました`);
    if (window.toast) toast.info(`${filename} のダウンロードを開始しました`);
}

// ファイル削除
async function deleteFile(filename) {
    if (!confirm(`${filename} を削除しますか?`)) {
        return;
    }

    try {
        const response = await fetch(`/files/${encodeURIComponent(state.selectedDirectory)}/${encodeURIComponent(filename)}`, {
            method: 'DELETE',
            credentials: 'include'
        });

        if (response.ok) {
            addActivityLog('delete', `${filename} を削除しました`);
            if (window.toast) toast.success(`${filename} を削除しました`);
            await loadFiles(state.selectedDirectory);
        } else {
            const error = await response.text();
            addActivityLog('error', `削除失敗: ${error}`);
            if (window.toast) toast.error(`削除失敗: ${error}`);
        }
    } catch (error) {
        console.error('削除エラー:', error);
        addActivityLog('error', 'ファイルの削除に失敗しました');
        if (window.toast) toast.error('ファイルの削除に失敗しました');
    }
}

// プログレス表示
function showProgress(show) {
    const progressDiv = document.getElementById('upload-progress');
    if (show) {
        progressDiv.classList.remove('hidden');
    } else {
        progressDiv.classList.add('hidden');
        setProgress(0);
    }
}

function setProgress(percent) {
    document.getElementById('progress-fill').style.width = percent + '%';
    document.getElementById('progress-text').textContent = percent + '%';
}

// Server-Sent Events接続
function connectSSE() {
    if (state.eventSource) {
        state.eventSource.close();
    }

    const eventSource = new EventSource('/api/events');
    state.eventSource = eventSource;

    const statusEl = document.getElementById('sse-status');
    statusEl.textContent = '接続中...';
    statusEl.className = 'sse-status';

    eventSource.onopen = () => {
        statusEl.textContent = '接続済み';
        statusEl.className = 'sse-status connected';
        addActivityLog('system', 'リアルタイム更新に接続しました');
        if (window.toast) toast.info('リアルタイム更新に接続しました', 3000);
    };

    eventSource.onerror = () => {
        statusEl.textContent = '切断';
        statusEl.className = 'sse-status disconnected';
        addActivityLog('error', 'リアルタイム更新が切断されました');
        if (window.toast) toast.warning('リアルタイム更新が切断されました', 3000);

        // 再接続
        setTimeout(() => {
            if (state.user) {
                connectSSE();
            }
        }, 5000);
    };

    // ファイルアップロードイベント
    eventSource.addEventListener('file_upload', (e) => {
        const data = JSON.parse(e.data);
        addActivityLog('upload', `${data.username} が ${data.filename} をアップロードしました`, true);

        // 同じディレクトリなら再読み込み
        if (data.directory === state.selectedDirectory) {
            loadFiles(state.selectedDirectory);
        }
    });

    // ファイルダウンロードイベント
    eventSource.addEventListener('file_download', (e) => {
        const data = JSON.parse(e.data);
        addActivityLog('download', `${data.username} が ${data.filename} をダウンロードしました`, true);
    });

    // ファイル削除イベント
    eventSource.addEventListener('file_delete', (e) => {
        const data = JSON.parse(e.data);
        addActivityLog('delete', `${data.username} が ${data.filename} を削除しました`, true);

        // 同じディレクトリなら再読み込み
        if (data.directory === state.selectedDirectory) {
            loadFiles(state.selectedDirectory);
        }
    });

    // ログインイベント
    eventSource.addEventListener('user_login', (e) => {
        const data = JSON.parse(e.data);
        addActivityLog('login', `${data.username} がログインしました`, true);
    });
}

// アクティビティログ追加
function addActivityLog(type, message, fromSSE = false) {
    const logContainer = document.getElementById('activity-log');
    const time = new Date().toLocaleTimeString('ja-JP');

    const logItem = document.createElement('div');
    logItem.className = `activity-item activity-type-${type}`;
    logItem.innerHTML = `
        <span class="activity-time">[${time}]</span>
        ${fromSSE ? '🔔 ' : ''}${message}
    `;

    logContainer.insertBefore(logItem, logContainer.firstChild);

    // 最大100件まで
    while (logContainer.children.length > 100) {
        logContainer.removeChild(logContainer.lastChild);
    }
}

// ユーティリティ関数
function formatFileSize(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
}

function formatDate(dateString) {
    const date = new Date(dateString);
    return date.toLocaleString('ja-JP');
}

// ページ離脱時にSSE切断
window.addEventListener('beforeunload', () => {
    if (state.eventSource) {
        state.eventSource.close();
    }
});
