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
        <div class="cursor-pointer p-4 rounded-xl border-2 transition-all ${
            state.selectedDirectory === dir.path
                ? 'border-discord-500 bg-discord-50 dark:bg-discord-900/20 shadow-lg'
                : 'border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-700 hover:border-discord-500 hover:shadow-md'
        }"
             onclick="selectDirectory('${dir.path}')">
            <div class="flex items-center gap-2 mb-3">
                <svg class="w-6 h-6 text-discord-500" fill="currentColor" viewBox="0 0 20 20">
                    <path d="M2 6a2 2 0 012-2h5l2 2h5a2 2 0 012 2v6a2 2 0 01-2 2H4a2 2 0 01-2-2V6z"/>
                </svg>
                <span class="font-bold text-lg text-gray-800 dark:text-white">${dir.path}</span>
            </div>
            <div class="flex gap-2 flex-wrap">
                ${dir.permissions.map(p => `
                    <span class="px-2 py-1 text-xs font-semibold rounded-md ${
                        p === 'read' ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300' :
                        p === 'write' ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300' :
                        'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
                    }">${p}</span>
                `).join('')}
            </div>
        </div>
    `).join('');
}

// ディレクトリ選択
async function selectDirectory(path) {
    state.selectedDirectory = path;
    renderDirectories();
    await loadFiles(path);

    // アップロードボタン有効化
    const selectedDir = state.directories.find(d => d.path === path);
    const canWrite = selectedDir && selectedDir.permissions.includes('write');

    document.getElementById('upload-btn').disabled = !canWrite;
    document.getElementById('chunk-upload-btn').disabled = !canWrite;
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

    container.innerHTML = state.files.map(file => {
        const filename = file.original_name || file.filename;
        const fileIcon = window.getFileIcon ? window.getFileIcon(filename) : { icon: '📁', color: 'text-gray-500', bg: 'bg-gray-50' };

        return `
        <div class="flex items-center gap-4 p-4 bg-white dark:bg-gray-700 rounded-xl border border-gray-200 dark:border-gray-600 hover:shadow-lg transition-all group">
            <div class="flex-shrink-0 w-12 h-12 ${fileIcon.bg} rounded-lg flex items-center justify-center text-2xl">
                ${fileIcon.icon}
            </div>
            <div class="flex-1 min-w-0">
                <div class="font-semibold text-gray-800 dark:text-white truncate">${filename}</div>
                <div class="text-sm text-gray-500 dark:text-gray-400">
                    ${formatFileSize(file.size)} • ${formatDate(file.modified_at)}
                </div>
            </div>
            <div class="flex gap-2">
                <button onclick="downloadFile('${file.filename}')"
                        class="px-4 py-2 bg-discord-500 hover:bg-discord-600 text-white font-medium rounded-lg transition-all transform hover:scale-105">
                    <svg class="w-5 h-5 inline-block mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/>
                    </svg>
                    ダウンロード
                </button>
                ${canDelete ? `
                    <button onclick="deleteFile('${file.filename}')"
                            class="px-4 py-2 bg-red-500 hover:bg-red-600 text-white font-medium rounded-lg transition-all transform hover:scale-105">
                        <svg class="w-5 h-5 inline-block mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
                        </svg>
                        削除
                    </button>
                ` : ''}
            </div>
        </div>
        `;
    }).join('');
}

// イベントリスナー設定
function setupEventListeners() {
    // 通常アップロード
    document.getElementById('upload-btn').addEventListener('click', handleUpload);

    // チャンクアップロード
    document.getElementById('chunk-upload-btn').addEventListener('click', handleChunkUpload);

    // ドラッグ&ドロップ
    setupDragAndDrop();
}

// ドラッグ&ドロップ設定
function setupDragAndDrop() {
    const uploadSection = document.querySelector('.upload-section');
    const dropZone = document.createElement('div');
    dropZone.className = 'drop-zone';
    dropZone.innerHTML = `
        <div class="drop-zone-content">
            <svg class="drop-zone-icon" width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
                <polyline points="17 8 12 3 7 8"></polyline>
                <line x1="12" y1="3" x2="12" y2="15"></line>
            </svg>
            <p class="drop-zone-text">ファイルをドラッグ&ドロップ</p>
            <p class="drop-zone-subtext">または</p>
            <label for="file-input" class="drop-zone-button">ファイルを選択</label>
        </div>
    `;

    // 既存のupload-boxを置き換え
    const uploadBox = uploadSection.querySelector('.upload-box');
    uploadBox.replaceWith(dropZone);

    // ファイル入力を隠す
    const fileInput = document.getElementById('file-input');
    fileInput.style.display = 'none';

    // ドラッグイベント
    ['dragenter', 'dragover', 'dragleave', 'drop'].forEach(eventName => {
        dropZone.addEventListener(eventName, preventDefaults, false);
        document.body.addEventListener(eventName, preventDefaults, false);
    });

    function preventDefaults(e) {
        e.preventDefault();
        e.stopPropagation();
    }

    // ハイライト
    ['dragenter', 'dragover'].forEach(eventName => {
        dropZone.addEventListener(eventName, () => {
            dropZone.classList.add('drop-zone-active');
        });
    });

    ['dragleave', 'drop'].forEach(eventName => {
        dropZone.addEventListener(eventName, () => {
            dropZone.classList.remove('drop-zone-active');
        });
    });

    // ドロップ処理
    dropZone.addEventListener('drop', (e) => {
        const files = Array.from(e.dataTransfer.files);
        if (files.length > 0) {
            handleDroppedFiles(files);
        }
    });

    // クリックでファイル選択
    dropZone.addEventListener('click', (e) => {
        if (e.target !== fileInput && !e.target.closest('label')) {
            fileInput.click();
        }
    });

    // ファイル選択時
    fileInput.addEventListener('change', (e) => {
        if (fileInput.files.length > 0) {
            handleDroppedFiles(Array.from(fileInput.files));
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

// 通常アップロード
async function handleUpload() {
    const fileInput = document.getElementById('file-input');
    const file = fileInput.files[0];

    if (!file) {
        if (window.toast) toast.warning('ファイルを選択してください');
        return;
    }

    if (file.size > 100 * 1024 * 1024) {
        if (window.toast) toast.warning('ファイルサイズが100MBを超えています。大容量アップロードを使用してください。');
        return;
    }

    const formData = new FormData();
    formData.append('file', file);
    formData.append('directory', state.selectedDirectory);

    showProgress(true);

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
            await loadFiles(state.selectedDirectory);
            fileInput.value = '';
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
        showProgress(false);
    }
}

// チャンクアップロード
async function handleChunkUpload() {
    const fileInput = document.getElementById('file-input');
    const file = fileInput.files[0];

    if (!file) {
        if (window.toast) toast.warning('ファイルを選択してください');
        return;
    }

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
        fileInput.value = '';

    } catch (error) {
        console.error('チャンクアップロードエラー:', error);
        addActivityLog('error', error.message);
        if (window.toast) toast.error(error.message);
    } finally {
        showProgress(false);
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
