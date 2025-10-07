// アプリケーション状態
const state = {
    user: null,
    directories: [],
    selectedDirectory: null,
    files: [],
    filteredFiles: [],
    eventSource: null,
    viewMode: 'list', // 'list' or 'grid'
    sortBy: 'name-asc',
    searchQuery: ''
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

// ディレクトリ描画（サイドバー用）
function renderDirectories() {
    const container = document.getElementById('directory-list');

    if (state.directories.length === 0) {
        container.innerHTML = '<div class="p-4 text-center text-sm text-gray-500 dark:text-gray-400">アクセス可能なフォルダがありません</div>';
        return;
    }

    container.innerHTML = state.directories.map(dir => `
        <div class="cursor-pointer px-3 py-2 rounded-lg transition-all mb-1 ${
            state.selectedDirectory === dir.path
                ? 'bg-discord-500 text-white shadow-md'
                : 'hover:bg-gray-100 dark:hover:bg-gray-700 text-gray-700 dark:text-gray-300'
        }"
             onclick="selectDirectory('${dir.path}')">
            <div class="flex items-center gap-2">
                <svg class="w-5 h-5 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                    <path d="M2 6a2 2 0 012-2h5l2 2h5a2 2 0 012 2v6a2 2 0 01-2 2H4a2 2 0 01-2-2V6z"/>
                </svg>
                <span class="font-semibold text-sm truncate">${dir.path}</span>
            </div>
            <div class="flex gap-1 mt-2 flex-wrap">
                ${dir.permissions.map(p => {
                    const iconMap = {
                        'read': '👁',
                        'write': '✏️',
                        'delete': '🗑️'
                    };
                    return `<span class="text-xs opacity-75">${iconMap[p] || p}</span>`;
                }).join(' ')}
            </div>
        </div>
    `).join('');
}

// パンくずリスト更新
function updateBreadcrumb() {
    const breadcrumb = document.getElementById('breadcrumb');
    if (!state.selectedDirectory) {
        breadcrumb.innerHTML = '<span class="text-gray-500 dark:text-gray-400">フォルダを選択してください</span>';
        return;
    }

    breadcrumb.innerHTML = `
        <svg class="w-4 h-4 text-discord-500" fill="currentColor" viewBox="0 0 20 20">
            <path d="M2 6a2 2 0 012-2h5l2 2h5a2 2 0 012 2v6a2 2 0 01-2 2H4a2 2 0 01-2-2V6z"/>
        </svg>
        <span class="ml-2 font-semibold text-gray-800 dark:text-white">${state.selectedDirectory}</span>
        <span class="ml-2 text-gray-500 dark:text-gray-400">(${state.files.length} ファイル)</span>
    `;
}

// ディレクトリ選択
async function selectDirectory(path) {
    state.selectedDirectory = path;
    state.searchQuery = ''; // 検索クリア
    document.getElementById('search-input').value = '';
    renderDirectories();
    updateBreadcrumb();
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
            applyFilters(); // フィルタ・ソート適用
        } else {
            if (window.toast) toast.error('ファイル一覧の取得に失敗しました');
        }
    } catch (error) {
        console.error('ファイル読み込みエラー:', error);
        addActivityLog('error', 'ファイル一覧の取得に失敗しました');
        if (window.toast) toast.error('ファイル一覧の取得に失敗しました');
    }
}

// フィルタ・ソート適用
function applyFilters() {
    let filtered = [...state.files];

    // 検索フィルタ
    if (state.searchQuery) {
        filtered = filtered.filter(file => {
            const filename = file.original_name || file.filename;
            return filename.toLowerCase().includes(state.searchQuery);
        });
    }

    // ソート
    filtered.sort((a, b) => {
        const aName = (a.original_name || a.filename).toLowerCase();
        const bName = (b.original_name || b.filename).toLowerCase();

        switch (state.sortBy) {
            case 'name-asc':
                return aName.localeCompare(bName);
            case 'name-desc':
                return bName.localeCompare(aName);
            case 'size-asc':
                return a.size - b.size;
            case 'size-desc':
                return b.size - a.size;
            case 'date-asc':
                return new Date(a.modified_at) - new Date(b.modified_at);
            case 'date-desc':
                return new Date(b.modified_at) - new Date(a.modified_at);
            default:
                return 0;
        }
    });

    state.filteredFiles = filtered;
    renderFiles();
    updateBreadcrumb();
}

// ファイル一覧描画（リスト/グリッド対応）
function renderFiles() {
    const container = document.getElementById('files-list');

    if (state.filteredFiles.length === 0) {
        if (state.searchQuery) {
            container.innerHTML = '<div class="text-center py-16"><p class="text-gray-500 dark:text-gray-400 text-lg">「<span class="font-semibold">' + state.searchQuery + '</span>」に一致するファイルが見つかりませんでした</p></div>';
        } else {
            container.innerHTML = '<div class="text-center py-16"><p class="text-gray-500 dark:text-gray-400 text-lg">ファイルがありません</p></div>';
        }
        return;
    }

    const selectedDir = state.directories.find(d => d.path === state.selectedDirectory);
    const canDelete = selectedDir && selectedDir.permissions.includes('delete');

    // viewModeを状態から取得
    const viewMode = state.viewMode;

    if (viewMode === 'list') {
        // リスト表示（Boxスタイルのテーブル）
        container.innerHTML = `
            <div class="bg-white dark:bg-gray-800 rounded-lg shadow overflow-hidden">
                <table class="w-full">
                    <thead class="bg-gray-50 dark:bg-gray-700 border-b border-gray-200 dark:border-gray-600">
                        <tr>
                            <th class="px-6 py-3 text-left text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">ファイル名</th>
                            <th class="px-6 py-3 text-left text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">サイズ</th>
                            <th class="px-6 py-3 text-left text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">更新日時</th>
                            <th class="px-6 py-3 text-right text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">アクション</th>
                        </tr>
                    </thead>
                    <tbody class="divide-y divide-gray-200 dark:divide-gray-700">
                        ${state.filteredFiles.map(file => {
                            const filename = file.original_name || file.filename;
                            const fileIcon = window.getFileIcon ? window.getFileIcon(filename) : { icon: '📁', color: 'text-gray-500', bg: 'bg-gray-50' };

                            return `
                            <tr class="hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors">
                                <td class="px-6 py-4">
                                    <div class="flex items-center gap-3">
                                        <div class="flex-shrink-0 w-10 h-10 ${fileIcon.bg} rounded-lg flex items-center justify-center text-xl">
                                            ${fileIcon.icon}
                                        </div>
                                        <span class="font-medium text-gray-800 dark:text-white truncate max-w-md" title="${filename}">${filename}</span>
                                    </div>
                                </td>
                                <td class="px-6 py-4 text-sm text-gray-600 dark:text-gray-400">${formatFileSize(file.size)}</td>
                                <td class="px-6 py-4 text-sm text-gray-600 dark:text-gray-400">${formatDate(file.modified_at)}</td>
                                <td class="px-6 py-4 text-right">
                                    <div class="flex justify-end gap-2">
                                        <button onclick="downloadFile('${file.filename}')" title="ダウンロード"
                                                class="p-2 text-discord-500 hover:bg-discord-50 dark:hover:bg-discord-900/20 rounded-lg transition-all">
                                            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/>
                                            </svg>
                                        </button>
                                        ${canDelete ? `
                                            <button onclick="deleteFile('${file.filename}')" title="削除"
                                                    class="p-2 text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg transition-all">
                                                <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
                                                </svg>
                                            </button>
                                        ` : ''}
                                    </div>
                                </td>
                            </tr>
                            `;
                        }).join('')}
                    </tbody>
                </table>
            </div>
        `;
    } else {
        // グリッド表示
        container.innerHTML = `
            <div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
                ${state.filteredFiles.map(file => {
                    const filename = file.original_name || file.filename;
                    const fileIcon = window.getFileIcon ? window.getFileIcon(filename) : { icon: '📁', color: 'text-gray-500', bg: 'bg-gray-50' };

                    return `
                    <div class="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4 hover:shadow-lg hover:border-discord-500 transition-all group cursor-pointer">
                        <div class="flex flex-col items-center text-center">
                            <div class="w-20 h-20 ${fileIcon.bg} rounded-xl flex items-center justify-center text-4xl mb-3">
                                ${fileIcon.icon}
                            </div>
                            <div class="font-semibold text-sm text-gray-800 dark:text-white truncate w-full mb-1" title="${filename}">${filename}</div>
                            <div class="text-xs text-gray-500 dark:text-gray-400 mb-3">${formatFileSize(file.size)}</div>
                            <div class="flex gap-2 w-full">
                                <button onclick="downloadFile('${file.filename}')"
                                        class="flex-1 px-3 py-1.5 bg-discord-500 hover:bg-discord-600 text-white text-xs font-semibold rounded transition-all">
                                    DL
                                </button>
                                ${canDelete ? `
                                    <button onclick="deleteFile('${file.filename}')"
                                            class="px-3 py-1.5 bg-red-500 hover:bg-red-600 text-white text-xs font-semibold rounded transition-all">
                                        削除
                                    </button>
                                ` : ''}
                            </div>
                        </div>
                    </div>
                    `;
                }).join('')}
            </div>
        `;
    }
}

// イベントリスナー設定
function setupEventListeners() {
    // ドラッグ&ドロップ
    setupDragAndDrop();

    // 検索
    const searchInput = document.getElementById('search-input');
    if (searchInput) {
        searchInput.addEventListener('input', (e) => {
            state.searchQuery = e.target.value.toLowerCase();
            applyFilters();
        });
    }

    // 並べ替え
    const sortSelect = document.getElementById('sort-select');
    if (sortSelect) {
        sortSelect.addEventListener('change', (e) => {
            state.sortBy = e.target.value;
            applyFilters();
        });
    }

    // ビュー切り替え (Alpine.jsが管理)
    // Alpine.jsのx-dataでviewModeを管理しているため、ここでは不要
}

// ビュー切り替え関数（HTMLから呼ばれる）
window.switchViewMode = function(mode) {
    state.viewMode = mode;
    renderFiles();
};

// ドラッグ&ドロップ設定（全画面対応）
function setupDragAndDrop() {
    const dropOverlay = document.getElementById('drop-overlay');
    const fileInput = document.getElementById('file-input');

    if (!dropOverlay || !fileInput) {
        console.error('Drop overlay or file input not found');
        return;
    }

    let dragCounter = 0;

    // ドラッグイベント
    ['dragenter', 'dragover', 'dragleave', 'drop'].forEach(eventName => {
        document.body.addEventListener(eventName, preventDefaults, false);
    });

    function preventDefaults(e) {
        e.preventDefault();
        e.stopPropagation();
    }

    // ドラッグ開始
    document.body.addEventListener('dragenter', (e) => {
        dragCounter++;
        if (e.dataTransfer.types.includes('Files')) {
            dropOverlay.classList.remove('hidden');
        }
    });

    // ドラッグ終了
    document.body.addEventListener('dragleave', (e) => {
        dragCounter--;
        if (dragCounter === 0) {
            dropOverlay.classList.add('hidden');
        }
    });

    // ドロップ処理
    document.body.addEventListener('drop', (e) => {
        dragCounter = 0;
        dropOverlay.classList.add('hidden');
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
