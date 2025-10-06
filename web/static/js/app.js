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

    // ユーザー情報表示
    const userInfo = document.getElementById('user-info');
    userInfo.innerHTML = `
        <span>${state.user.username}</span>
        <a href="/auth/logout" class="btn btn-danger">ログアウト</a>
    `;
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
        }
    } catch (error) {
        console.error('ディレクトリ読み込みエラー:', error);
        addActivityLog('error', 'ディレクトリの読み込みに失敗しました');
    }
}

// ディレクトリ描画
function renderDirectories() {
    const container = document.getElementById('directory-list');

    if (state.directories.length === 0) {
        container.innerHTML = '<div class="empty-state"><p>アクセス可能なディレクトリがありません</p></div>';
        return;
    }

    container.innerHTML = state.directories.map(dir => `
        <div class="directory-card ${state.selectedDirectory === dir.path ? 'active' : ''}"
             onclick="selectDirectory('${dir.path}')">
            <div class="directory-name">${dir.path}</div>
            <div class="directory-permissions">
                ${dir.permissions.map(p => `<span class="permission-badge">${p}</span>`).join('')}
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
        }
    } catch (error) {
        console.error('ファイル読み込みエラー:', error);
        addActivityLog('error', 'ファイル一覧の取得に失敗しました');
    }
}

// ファイル一覧描画
function renderFiles() {
    const container = document.getElementById('files-list');

    if (state.files.length === 0) {
        container.innerHTML = '<div class="empty-state"><p>ファイルがありません</p></div>';
        return;
    }

    const selectedDir = state.directories.find(d => d.path === state.selectedDirectory);
    const canDelete = selectedDir && selectedDir.permissions.includes('delete');

    container.innerHTML = state.files.map(file => `
        <div class="file-item">
            <div class="file-info">
                <div class="file-name">${file.original_name || file.filename}</div>
                <div class="file-meta">
                    ${formatFileSize(file.size)} • ${formatDate(file.modified_at)}
                </div>
            </div>
            <div class="file-actions">
                <button class="btn btn-primary" onclick="downloadFile('${file.filename}')">ダウンロード</button>
                ${canDelete ? `<button class="btn btn-danger" onclick="deleteFile('${file.filename}')">削除</button>` : ''}
            </div>
        </div>
    `).join('');
}

// イベントリスナー設定
function setupEventListeners() {
    // 通常アップロード
    document.getElementById('upload-btn').addEventListener('click', handleUpload);

    // チャンクアップロード
    document.getElementById('chunk-upload-btn').addEventListener('click', handleChunkUpload);
}

// 通常アップロード
async function handleUpload() {
    const fileInput = document.getElementById('file-input');
    const file = fileInput.files[0];

    if (!file) {
        alert('ファイルを選択してください');
        return;
    }

    if (file.size > 100 * 1024 * 1024) {
        alert('ファイルサイズが100MBを超えています。大容量アップロードを使用してください。');
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
            await loadFiles(state.selectedDirectory);
            fileInput.value = '';
        } else {
            const error = await response.text();
            addActivityLog('error', `アップロード失敗: ${error}`);
        }
    } catch (error) {
        console.error('アップロードエラー:', error);
        addActivityLog('error', 'アップロードに失敗しました');
    } finally {
        showProgress(false);
    }
}

// チャンクアップロード
async function handleChunkUpload() {
    const fileInput = document.getElementById('file-input');
    const file = fileInput.files[0];

    if (!file) {
        alert('ファイルを選択してください');
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
        await loadFiles(state.selectedDirectory);
        fileInput.value = '';

    } catch (error) {
        console.error('チャンクアップロードエラー:', error);
        addActivityLog('error', error.message);
    } finally {
        showProgress(false);
    }
}

// ファイルダウンロード
function downloadFile(filename) {
    const url = `/files/download/${encodeURIComponent(state.selectedDirectory)}/${encodeURIComponent(filename)}`;
    window.location.href = url;
    addActivityLog('download', `${filename} をダウンロードしました`);
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
            await loadFiles(state.selectedDirectory);
        } else {
            const error = await response.text();
            addActivityLog('error', `削除失敗: ${error}`);
        }
    } catch (error) {
        console.error('削除エラー:', error);
        addActivityLog('error', 'ファイルの削除に失敗しました');
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
    };

    eventSource.onerror = () => {
        statusEl.textContent = '切断';
        statusEl.className = 'sse-status disconnected';
        addActivityLog('error', 'リアルタイム更新が切断されました');

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
