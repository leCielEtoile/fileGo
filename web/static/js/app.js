// HTMLエスケープ（格納型XSS対策）。
// ファイル名やアップロード者名はユーザー入力由来のため、innerHTML へ差し込む前に必ず通す。
function escapeHtml(value) {
    return String(value ?? '').replace(/[&<>"']/g, (c) => ({
        '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    })[c]);
}
window.escapeHtml = escapeHtml;

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
    searchQuery: '',
    eventListenersInitialized: false,
    selectedFiles: new Set() // 一括操作用の選択されたファイル
};

// ページ読み込み時
document.addEventListener('DOMContentLoaded', async () => {
    await checkAuth();
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
        <span class="text-gray-700 dark:text-gray-200 font-medium truncate max-w-[8rem]">${escapeHtml(state.user.username)}</span>
        <a href="/auth/logout" class="px-3 py-1.5 border border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-700 font-medium rounded-lg transition-colors whitespace-nowrap flex-shrink-0">
            ログアウト
        </a>
    `;

    // イベントリスナー設定（DOM表示後）
    setupEventListeners();

    // 初期状態のウェルカム画面を表示
    showWelcomeScreen();

    // ログイン成功トースト
    if (window.toast) {
        toast.success('ログインしました');
    }
}

// ウェルカム画面表示
function showWelcomeScreen() {
    const container = document.getElementById('files-list');
    container.innerHTML = `
        <div class="flex flex-col items-center justify-center py-24 px-4 fade-in">
            <div class="w-20 h-20 bg-primary-50 dark:bg-gray-700 rounded-full flex items-center justify-center mb-6">
                <svg class="w-10 h-10 text-primary-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/>
                </svg>
            </div>
            <h2 class="text-xl font-medium text-gray-800 dark:text-white mb-2">フォルダを選択してください</h2>
            <p class="text-gray-500 dark:text-gray-400 text-center max-w-md">
                左のサイドバーからフォルダを開くと、ファイルの一覧が表示されます。ファイルはドラッグ＆ドロップでもアップロードできます。
            </p>
        </div>
    `;
}

// ディレクトリ一覧読み込み（Skeleton loading対応）
async function loadDirectories() {
    const container = document.getElementById('directory-list');

    // Skeleton表示
    showDirectorySkeleton();

    try {
        const response = await fetch('/files/directories', {
            credentials: 'include'
        });

        if (response.ok) {
            const data = await response.json();
            state.directories = data.directories || [];
            renderDirectories();
        } else {
            container.innerHTML = '<div class="p-4 text-center text-sm text-red-500">ディレクトリの読み込みに失敗しました</div>';
            if (window.toast) toast.error('ディレクトリの読み込みに失敗しました');
        }
    } catch (error) {
        console.error('ディレクトリ読み込みエラー:', error);
        container.innerHTML = '<div class="p-4 text-center text-sm text-red-500">ディレクトリの読み込みに失敗しました</div>';
        addActivityLog('error', 'ディレクトリの読み込みに失敗しました');
        if (window.toast) toast.error('ディレクトリの読み込みに失敗しました');
    }
}

// ディレクトリ Skeleton 表示
function showDirectorySkeleton() {
    const container = document.getElementById('directory-list');
    container.innerHTML = Array.from({ length: 3 }, () => `
        <div class="px-3 py-2.5 mb-1">
            <div class="flex items-center gap-2 mb-2">
                <div class="w-8 h-8 bg-gray-200 dark:bg-gray-700 rounded-lg skeleton"></div>
                <div class="flex-1 h-4 bg-gray-200 dark:bg-gray-700 rounded skeleton"></div>
            </div>
            <div class="flex gap-1">
                <div class="w-12 h-5 bg-gray-200 dark:bg-gray-700 rounded skeleton"></div>
                <div class="w-12 h-5 bg-gray-200 dark:bg-gray-700 rounded skeleton"></div>
            </div>
        </div>
    `).join('');
}

// ディレクトリ描画（サイドバー用）
function renderDirectories() {
    const container = document.getElementById('directory-list');

    if (state.directories.length === 0) {
        container.innerHTML = `
            <div class="flex flex-col items-center justify-center py-8 px-4 text-center">
                <div class="w-16 h-16 bg-gray-100 dark:bg-gray-700 rounded-full flex items-center justify-center mb-4">
                    <svg class="w-8 h-8 text-gray-400 dark:text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/>
                    </svg>
                </div>
                <p class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">フォルダがありません</p>
                <p class="text-xs text-gray-500 dark:text-gray-400">管理者に権限を依頼してください</p>
            </div>
        `;
        return;
    }

    container.innerHTML = state.directories.map(dir => {
        const isSelected = state.selectedDirectory === dir.path;
        const permissionBadges = dir.permissions.map(p => {
            const badgeConfig = {
                'read': { icon: `<svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"/></svg>`, label: '読取', color: 'bg-blue-500/10 text-blue-600 dark:text-blue-400' },
                'write': { icon: `<svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"/></svg>`, label: '書込', color: 'bg-green-500/10 text-green-600 dark:text-green-400' },
                'delete': { icon: `<svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/></svg>`, label: '削除', color: 'bg-red-500/10 text-red-600 dark:text-red-400' }
            };
            const config = badgeConfig[p] || { icon: '', label: p, color: 'bg-gray-500/10 text-gray-600' };
            return `<span class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-xs font-medium ${config.color}" title="${config.label}">${config.icon}</span>`;
        }).join('');

        const safePath = escapeHtml(dir.path);
        return `
        <div class="group cursor-pointer px-3 py-2 rounded-lg transition-colors mb-0.5 ${
            isSelected
                ? 'bg-primary-50 dark:bg-primary-500/15 text-primary-700 dark:text-primary-200'
                : 'hover:bg-gray-100 dark:hover:bg-gray-700 text-gray-700 dark:text-gray-300'
        }"
             onclick="selectDirectory('${dir.path}')" data-dir="${safePath}">
            <div class="flex items-center gap-2 mb-1.5">
                <svg class="w-5 h-5 flex-shrink-0 ${isSelected ? 'text-primary-500' : 'text-gray-400 dark:text-gray-500'}" fill="currentColor" viewBox="0 0 20 20">
                    <path d="M2 6a2 2 0 012-2h5l2 2h5a2 2 0 012 2v6a2 2 0 01-2 2H4a2 2 0 01-2-2V6z"/>
                </svg>
                <span class="font-medium text-sm truncate flex-1">${safePath}</span>
            </div>
            <div class="flex gap-1 flex-wrap pl-7">
                ${permissionBadges}
            </div>
        </div>
        `;
    }).join('');
}

// パンくずリスト更新（インタラクティブ対応）
function updateBreadcrumb() {
    const breadcrumb = document.getElementById('breadcrumb');
    if (!state.selectedDirectory) {
        breadcrumb.innerHTML = '<span class="text-gray-500 dark:text-gray-400">フォルダを選択してください</span>';
        return;
    }

    // パスを分割してパンくずリストを構築
    const pathParts = state.selectedDirectory.split('/');
    let currentPath = '';
    const breadcrumbItems = pathParts.map((part, index) => {
        currentPath = index === 0 ? part : `${currentPath}/${part}`;
        const isLast = index === pathParts.length - 1;

        if (isLast) {
            // 最後の要素（現在のディレクトリ）はクリック不可
            return `<span class="font-medium text-gray-800 dark:text-white">${escapeHtml(part)}</span>`;
        } else {
            // 親ディレクトリはクリック可能
            const path = currentPath;
            return `<button onclick="selectDirectory('${path}')" class="font-medium text-primary-600 hover:text-primary-700 dark:text-primary-300 hover:underline transition-colors">${escapeHtml(part)}</button>`;
        }
    });

    // 区切り文字を挿入
    const breadcrumbHTML = breadcrumbItems.map((item, index) => {
        if (index === 0) {
            return item;
        }
        return `<svg class="w-4 h-4 text-gray-400 mx-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
                </svg>${item}`;
    }).join('');

    breadcrumb.innerHTML = `
        <div class="flex items-center min-w-0 w-full">
            <svg class="w-4 h-4 text-primary-500 mr-2 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                <path d="M2 6a2 2 0 012-2h5l2 2h5a2 2 0 012 2v6a2 2 0 01-2 2H4a2 2 0 01-2-2V6z"/>
            </svg>
            <div class="flex items-center min-w-0 truncate">${breadcrumbHTML}</div>
            <span class="ml-3 flex-shrink-0 whitespace-nowrap text-gray-500 dark:text-gray-400">(${state.files.length} ファイル)</span>
        </div>
    `;
}

// ディレクトリ選択
async function selectDirectory(path) {
    state.selectedDirectory = path;
    state.searchQuery = ''; // 検索クリア
    const searchInput = document.getElementById('search-input');
    if (searchInput) {
        searchInput.value = '';
    }
    renderDirectories();
    updateBreadcrumb();
    await loadFiles(path);

    // モバイル版: ディレクトリ選択後にメニューを自動で閉じる。
    // Alpine v3 は要素の .__x を廃止したため、公式の Alpine.$data で状態へアクセスする。
    const appSection = document.getElementById('app-section');
    if (appSection && window.Alpine) {
        const data = Alpine.$data(appSection);
        if (data && 'showDirMenu' in data) {
            data.showDirMenu = false;
        }
    }
}

// ファイル一覧読み込み（Skeleton loading対応）
async function loadFiles(directory) {
    const container = document.getElementById('files-list');

    // Skeleton表示
    showFilesSkeleton();

    try {
        const response = await fetch(`/files?directory=${encodeURIComponent(directory)}`, {
            credentials: 'include'
        });

        if (response.ok) {
            const data = await response.json();
            state.files = data.files || [];
            applyFilters(); // フィルタ・ソート適用
        } else {
            container.innerHTML = '<div class="text-center py-16 px-4 text-red-500">ファイル一覧の取得に失敗しました</div>';
            if (window.toast) toast.error('ファイル一覧の取得に失敗しました');
        }
    } catch (error) {
        console.error('ファイル読み込みエラー:', error);
        container.innerHTML = '<div class="text-center py-16 px-4 text-red-500">ファイル一覧の取得に失敗しました</div>';
        addActivityLog('error', 'ファイル一覧の取得に失敗しました');
        if (window.toast) toast.error('ファイル一覧の取得に失敗しました');
    }
}

// ファイル Skeleton 表示
function showFilesSkeleton() {
    const container = document.getElementById('files-list');

    // デスクトップ用テーブルSkeleton
    container.innerHTML = `
        <div class="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 overflow-hidden">
            <div class="bg-gray-50 dark:bg-gray-700/40 border-b border-gray-200 dark:border-gray-700 px-6 py-4">
                <div class="flex gap-4">
                    <div class="flex-1 h-3 bg-gray-200 dark:bg-gray-600 rounded skeleton"></div>
                    <div class="w-20 h-3 bg-gray-200 dark:bg-gray-600 rounded skeleton"></div>
                    <div class="w-32 h-3 bg-gray-200 dark:bg-gray-600 rounded skeleton"></div>
                    <div class="w-24 h-3 bg-gray-200 dark:bg-gray-600 rounded skeleton"></div>
                </div>
            </div>
            <div class="divide-y divide-gray-200 dark:divide-gray-700">
                ${Array.from({ length: 5 }, () => `
                    <div class="px-6 py-4 flex items-center gap-4">
                        <div class="w-10 h-10 bg-gray-200 dark:bg-gray-700 rounded-lg skeleton"></div>
                        <div class="flex-1 h-4 bg-gray-200 dark:bg-gray-700 rounded skeleton"></div>
                        <div class="w-16 h-4 bg-gray-200 dark:bg-gray-700 rounded skeleton"></div>
                        <div class="w-32 h-4 bg-gray-200 dark:bg-gray-700 rounded skeleton"></div>
                        <div class="w-20 flex gap-2">
                            <div class="w-8 h-8 bg-gray-200 dark:bg-gray-700 rounded-lg skeleton"></div>
                            <div class="w-8 h-8 bg-gray-200 dark:bg-gray-700 rounded-lg skeleton"></div>
                        </div>
                    </div>
                `).join('')}
            </div>
        </div>
    `;
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

// ファイル一覧描画（リスト/グリッド対応 + モバイル最適化）
// ファイル一覧の行/カードは、ファイル名を onclick 文字列へ埋め込まず配列インデックスで参照する。
// これによりファイル名由来のスクリプト注入（XSS）を根本から断つ。
function renderFiles() {
    const container = document.getElementById('files-list');

    if (state.filteredFiles.length === 0) {
        if (state.searchQuery) {
            container.innerHTML = `
                <div class="flex flex-col items-center justify-center py-20 px-4">
                    <div class="w-16 h-16 bg-gray-100 dark:bg-gray-700 rounded-full flex items-center justify-center mb-4">
                        <svg class="w-8 h-8 text-gray-400 dark:text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/>
                        </svg>
                    </div>
                    <h3 class="text-lg font-medium text-gray-800 dark:text-white mb-1">検索結果なし</h3>
                    <p class="text-gray-500 dark:text-gray-400 text-center">「<span class="font-medium">${escapeHtml(state.searchQuery)}</span>」に一致するファイルが見つかりませんでした</p>
                </div>
            `;
        } else {
            container.innerHTML = `
                <div class="flex flex-col items-center justify-center py-20 px-4">
                    <div class="w-16 h-16 bg-primary-50 dark:bg-gray-700 rounded-full flex items-center justify-center mb-4">
                        <svg class="w-8 h-8 text-primary-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12"/>
                        </svg>
                    </div>
                    <h3 class="text-lg font-medium text-gray-800 dark:text-white mb-1">ファイルがありません</h3>
                    <p class="text-gray-500 dark:text-gray-400 mb-6 text-center">ファイルをドラッグ＆ドロップするか、アップロードボタンから追加してください</p>
                    <button onclick="document.getElementById('file-input').click()" class="px-5 py-2.5 bg-primary-500 hover:bg-primary-600 text-white font-medium rounded-lg transition-colors flex items-center gap-2">
                        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12"/>
                        </svg>
                        ファイルをアップロード
                    </button>
                </div>
            `;
        }
        return;
    }

    const selectedDir = state.directories.find(d => d.path === state.selectedDirectory);
    const canDelete = selectedDir && selectedDir.permissions.includes('delete');
    const viewMode = state.viewMode;

    // モバイルは横長テーブルだと右側（サイズ/日時/操作）が画面外に隠れるため、
    // 行タップで詳細モーダル（DL/削除を含む）を開く縦積みリストで描画する。
    if (window.IS_MOBILE) {
        const chevron = `<svg class="w-5 h-5 text-gray-300 dark:text-gray-500 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/></svg>`;
        container.innerHTML = state.filteredFiles.map((file, i) => {
            const filename = escapeHtml(file.original_name || file.filename);
            if (file.is_directory) {
                return `
                <button onclick="window.openDirByIndex(${i})" class="w-full flex items-center gap-3 px-3 py-3 mb-2 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg text-left active:bg-gray-50 dark:active:bg-gray-700">
                    <svg class="w-8 h-8 flex-shrink-0 text-primary-400" fill="currentColor" viewBox="0 0 20 20"><path d="M2 6a2 2 0 012-2h5l2 2h5a2 2 0 012 2v6a2 2 0 01-2 2H4a2 2 0 01-2-2V6z"/></svg>
                    <div class="flex-1 min-w-0"><div class="text-sm text-gray-800 dark:text-white truncate">${filename}</div><div class="text-xs text-gray-400 dark:text-gray-500">フォルダ</div></div>
                    ${chevron}
                </button>`;
            }
            const ic = window.getFileIconSVG ? window.getFileIconSVG(file.original_name || file.filename) : { svg:'', color:'text-gray-500', bg:'bg-gray-50' };
            return `
            <button onclick="window.detailByIndex(${i})" class="w-full flex items-center gap-3 px-3 py-3 mb-2 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg text-left active:bg-gray-50 dark:active:bg-gray-700">
                <div class="w-9 h-9 ${ic.bg} rounded-lg flex items-center justify-center ${ic.color} p-2 flex-shrink-0">${ic.svg}</div>
                <div class="flex-1 min-w-0">
                    <div class="text-sm text-gray-800 dark:text-white truncate">${filename}</div>
                    <div class="text-xs text-gray-400 dark:text-gray-500">${escapeHtml(formatFileSize(file.size))} ・ ${escapeHtml(formatDate(file.modified_at))}</div>
                </div>
                ${chevron}
            </button>`;
        }).join('');
        return;
    }

    if (viewMode === 'list') {
        const allSelected = state.selectedFiles.size === state.filteredFiles.length && state.filteredFiles.length > 0;
        const bulkActionsBar = state.selectedFiles.size > 0 ? `
            <div class="mb-3 px-4 py-2.5 bg-primary-50 dark:bg-primary-500/15 border border-primary-100 dark:border-primary-500/30 rounded-lg flex items-center justify-between fade-in">
                <span class="text-sm font-medium text-primary-700 dark:text-primary-200">${state.selectedFiles.size} 件を選択中</span>
                <div class="flex items-center gap-2">
                    <button onclick="window.bulkDownload()" class="px-3 py-1.5 text-sm text-primary-700 dark:text-primary-200 hover:bg-primary-100 dark:hover:bg-primary-500/20 rounded-lg transition-colors font-medium">ダウンロード</button>
                    ${canDelete ? `<button onclick="window.bulkDelete()" class="px-3 py-1.5 text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg transition-colors font-medium">削除</button>` : ''}
                    <button onclick="window.clearSelection()" class="px-3 py-1.5 text-sm text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors font-medium">選択解除</button>
                </div>
            </div>
        ` : '';

        const rows = state.filteredFiles.map((file, i) => {
            const filename = escapeHtml(file.original_name || file.filename);
            const isSelected = state.selectedFiles.has(file.filename);

            if (file.is_directory) {
                return `
                <tr class="hover:bg-gray-50 dark:hover:bg-gray-700/40 transition-colors cursor-pointer"
                    onclick="window.openDirByIndex(${i})">
                    <td class="px-4 py-2.5"></td>
                    <td class="px-4 py-2.5">
                        <div class="flex items-center gap-3">
                            <svg class="w-6 h-6 flex-shrink-0 text-primary-400" fill="currentColor" viewBox="0 0 20 20"><path d="M2 6a2 2 0 012-2h5l2 2h5a2 2 0 012 2v6a2 2 0 01-2 2H4a2 2 0 01-2-2V6z"/></svg>
                            <span class="text-sm text-gray-800 dark:text-white truncate max-w-md" title="${filename}">${filename}</span>
                        </div>
                    </td>
                    <td class="px-4 py-2.5 text-sm text-gray-500 dark:text-gray-400">—</td>
                    <td class="px-4 py-2.5 text-sm text-gray-500 dark:text-gray-400">${escapeHtml(formatDate(file.modified_at))}</td>
                    <td class="px-4 py-2.5"></td>
                </tr>`;
            }

            const iconConfig = window.getFileIconSVG ? window.getFileIconSVG(file.original_name || file.filename) : { svg: '', color: 'text-gray-500', bg: 'bg-gray-50' };
            return `
            <tr class="hover:bg-gray-50 dark:hover:bg-gray-700/40 transition-colors ${isSelected ? 'bg-primary-50 dark:bg-primary-500/10' : ''}"
                oncontextmenu="window.contextMenuByIndex(event, ${i})">
                <td class="px-4 py-2.5">
                    <input type="checkbox" ${isSelected ? 'checked' : ''}
                           onchange="window.toggleSelectionByIndex(${i}, this.checked)"
                           class="w-4 h-4 text-primary-500 bg-gray-100 border-gray-300 rounded focus:ring-primary-500 focus:ring-2 cursor-pointer">
                </td>
                <td class="px-4 py-2.5">
                    <div class="flex items-center gap-3">
                        <div class="flex-shrink-0 w-8 h-8 ${iconConfig.bg} rounded-lg flex items-center justify-center ${iconConfig.color} p-1.5">${iconConfig.svg}</div>
                        <button onclick="window.detailByIndex(${i})" class="text-sm text-gray-800 dark:text-white hover:text-primary-600 dark:hover:text-primary-300 hover:underline truncate max-w-md text-left" title="${filename}">${filename}</button>
                    </div>
                </td>
                <td class="px-4 py-2.5 text-sm text-gray-500 dark:text-gray-400">${escapeHtml(formatFileSize(file.size))}</td>
                <td class="px-4 py-2.5 text-sm text-gray-500 dark:text-gray-400">${escapeHtml(formatDate(file.modified_at))}</td>
                <td class="px-4 py-2.5 text-right">
                    <div class="flex justify-end gap-1">
                        <button onclick="window.detailByIndex(${i})" title="詳細" aria-label="詳細" class="p-1.5 text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors">
                            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>
                        </button>
                        <button onclick="window.downloadByIndex(${i})" title="ダウンロード" aria-label="ダウンロード" class="p-1.5 text-gray-400 hover:text-primary-600 dark:hover:text-primary-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors">
                            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/></svg>
                        </button>
                        ${canDelete ? `
                        <button onclick="window.deleteByIndex(${i})" title="削除" aria-label="削除" class="p-1.5 text-gray-400 hover:text-red-600 dark:hover:text-red-400 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors">
                            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/></svg>
                        </button>` : ''}
                    </div>
                </td>
            </tr>`;
        }).join('');

        container.innerHTML = bulkActionsBar + `
            <div class="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 overflow-hidden">
                <table class="w-full">
                    <thead class="bg-gray-50 dark:bg-gray-700/40 border-b border-gray-200 dark:border-gray-700">
                        <tr>
                            <th class="px-4 py-2.5 w-12">
                                <input type="checkbox" onchange="window.toggleSelectAll(this.checked)" ${allSelected ? 'checked' : ''}
                                       class="w-4 h-4 text-primary-500 bg-gray-100 border-gray-300 rounded focus:ring-primary-500 focus:ring-2 cursor-pointer">
                            </th>
                            <th class="px-4 py-2.5 text-left text-xs font-medium text-gray-500 dark:text-gray-400">名前</th>
                            <th class="px-4 py-2.5 text-left text-xs font-medium text-gray-500 dark:text-gray-400 w-28">サイズ</th>
                            <th class="px-4 py-2.5 text-left text-xs font-medium text-gray-500 dark:text-gray-400 w-44">更新日時</th>
                            <th class="px-4 py-2.5 w-28"></th>
                        </tr>
                    </thead>
                    <tbody class="divide-y divide-gray-100 dark:divide-gray-700/60">${rows}</tbody>
                </table>
            </div>
        `;
    } else {
        const cards = state.filteredFiles.map((file, i) => {
            const filename = escapeHtml(file.original_name || file.filename);

            if (file.is_directory) {
                return `
                <div class="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4 hover:border-primary-300 dark:hover:border-primary-500/50 hover:shadow-sm transition-colors cursor-pointer"
                     onclick="window.openDirByIndex(${i})">
                    <div class="flex flex-col items-center text-center">
                        <svg class="w-14 h-14 text-primary-400 mb-2" fill="currentColor" viewBox="0 0 20 20"><path d="M2 6a2 2 0 012-2h5l2 2h5a2 2 0 012 2v6a2 2 0 01-2 2H4a2 2 0 01-2-2V6z"/></svg>
                        <div class="text-sm text-gray-800 dark:text-white truncate w-full" title="${filename}">${filename}</div>
                        <div class="text-xs text-gray-400 dark:text-gray-500 mt-0.5">フォルダ</div>
                    </div>
                </div>`;
            }

            const iconConfig = window.getFileIconSVG ? window.getFileIconSVG(file.original_name || file.filename) : { svg: '', color: 'text-gray-500', bg: 'bg-gray-50' };
            return `
            <div class="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4 hover:border-primary-300 dark:hover:border-primary-500/50 hover:shadow-sm transition-colors cursor-pointer"
                 onclick="window.detailByIndex(${i})" oncontextmenu="window.contextMenuByIndex(event, ${i})">
                <div class="flex flex-col items-center text-center">
                    <div class="w-16 h-16 ${iconConfig.bg} rounded-lg flex items-center justify-center ${iconConfig.color} mb-2 p-3.5">${iconConfig.svg}</div>
                    <div class="text-sm text-gray-800 dark:text-white truncate w-full" title="${filename}">${filename}</div>
                    <div class="text-xs text-gray-400 dark:text-gray-500 mt-0.5 mb-3">${escapeHtml(formatFileSize(file.size))}</div>
                    <div class="flex gap-1 w-full" onclick="event.stopPropagation()">
                        <button onclick="window.downloadByIndex(${i})" aria-label="ダウンロード" class="flex-1 flex items-center justify-center px-3 py-1.5 border border-gray-200 dark:border-gray-600 text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700 text-xs rounded-lg transition-colors">
                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/></svg>
                        </button>
                        ${canDelete ? `
                        <button onclick="window.deleteByIndex(${i})" aria-label="削除" class="px-3 py-1.5 border border-gray-200 dark:border-gray-600 text-gray-600 dark:text-gray-300 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 text-xs rounded-lg transition-colors">
                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/></svg>
                        </button>` : ''}
                    </div>
                </div>
            </div>`;
        }).join('');

        container.innerHTML = `<div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 2xl:grid-cols-6 gap-3">${cards}</div>`;
    }
}

// ファイル一覧のインデックス経由アクセサ（ファイル名を onclick へ埋め込まないための橋渡し）。
window.openDirByIndex = function(i) { const f = state.filteredFiles[i]; if (f) selectDirectory(f.path); };
window.detailByIndex = function(i) { const f = state.filteredFiles[i]; if (f) window.showFileDetail(f); };
window.downloadByIndex = function(i) { const f = state.filteredFiles[i]; if (f) downloadFile(f.filename); };
window.deleteByIndex = function(i) { const f = state.filteredFiles[i]; if (f) deleteFile(f.filename); };
window.contextMenuByIndex = function(event, i) { const f = state.filteredFiles[i]; if (f) window.showContextMenu(event, f); };
window.toggleSelectionByIndex = function(i, checked) { const f = state.filteredFiles[i]; if (f) window.toggleFileSelection(f.filename, checked); };

// イベントリスナー設定
function setupEventListeners() {
    if (state.eventListenersInitialized) {
        console.log('イベントリスナーは既に初期化済みです');
        return;
    }

    console.log('イベントリスナーを初期化します...');

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

    state.eventListenersInitialized = true;
    console.log('イベントリスナーの初期化が完了しました');

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

    console.log('setupDragAndDrop: dropOverlay=', dropOverlay, 'fileInput=', fileInput);

    if (!dropOverlay || !fileInput) {
        console.error('Drop overlay or file input not found', { dropOverlay, fileInput });
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
        console.log('ファイルがドロップされました:', files);
        if (files.length > 0) {
            handleDroppedFiles(files);
        }
    });

    // ファイル選択時
    fileInput.addEventListener('change', (e) => {
        console.log('ファイルが選択されました:', fileInput.files);
        if (fileInput.files.length > 0) {
            handleDroppedFiles(Array.from(fileInput.files));
            fileInput.value = ''; // リセット
        }
    });

    console.log('ドラッグ&ドロップのイベントリスナーを設定しました');
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

    // 進行中リストに追加
    const uploadId = addActiveUpload(file, state.selectedDirectory);

    // 100MB以上はチャンクアップロード
    if (file.size > 100 * 1024 * 1024) {
        await uploadFileInChunks(file, uploadId);
    } else {
        await uploadFileNormal(file, uploadId);
    }
}

// 通常アップロード（リファクタリング）
async function uploadFileNormal(file, uploadId) {
    const upload = activeUploads[uploadId];
    if (!upload) return;

    const formData = new FormData();
    formData.append('file', file);
    formData.append('directory', state.selectedDirectory);

    try {
        const xhr = new XMLHttpRequest();

        // プログレス更新（進行中アップロードパネルへ反映）
        xhr.upload.addEventListener('progress', (e) => {
            if (e.lengthComputable) {
                const percent = Math.round((e.loaded / e.total) * 100);
                updateUploadProgress(uploadId, percent);
            }
        });

        // 完了
        xhr.addEventListener('load', async () => {
            if (xhr.status === 200) {
                addActivityLog('upload', `${file.name} をアップロードしました`);
                if (window.toast) toast.success(`${file.name} のアップロードが完了しました`);
                updateUploadProgress(uploadId, 100, 'completed');
                await loadFiles(state.selectedDirectory);
            } else {
                addActivityLog('error', `アップロード失敗: ${xhr.responseText}`);
                if (window.toast) toast.error(`アップロード失敗`);
                updateUploadProgress(uploadId, upload.progress, 'failed');
            }
        });

        // エラー
        xhr.addEventListener('error', () => {
            console.error('アップロードエラー');
            addActivityLog('error', 'アップロードに失敗しました');
            if (window.toast) toast.error('アップロードに失敗しました');
            updateUploadProgress(uploadId, upload.progress, 'failed');
        });

        // キャンセル対応
        upload.abortController.signal.addEventListener('abort', () => {
            xhr.abort();
        });

        xhr.open('POST', '/files/upload');
        xhr.withCredentials = true;
        xhr.send(formData);

    } catch (error) {
        console.error('アップロードエラー:', error);
        addActivityLog('error', 'アップロードに失敗しました');
        if (window.toast) toast.error('アップロードに失敗しました');
        updateUploadProgress(uploadId, upload.progress, 'failed');
    }
}

// チャンクアップロード（レジューム対応）
async function uploadFileInChunks(file, uploadId) {
    const upload = activeUploads[uploadId];
    if (!upload) return;

    const chunkSize = 20 * 1024 * 1024; // 20MB
    const totalChunks = Math.ceil(file.size / chunkSize);
    const storageKey = `upload_${file.name}_${file.size}_${state.selectedDirectory}`;

    let upload_id = null;
    let startChunk = 0;

    try {
        // localStorage から過去のアップロードセッションを確認
        const savedSession = localStorage.getItem(storageKey);
        if (savedSession) {
            const { uploadId, timestamp } = JSON.parse(savedSession);

            // 48時間以内のセッションのみレジューム対象
            if (Date.now() - timestamp < 48 * 60 * 60 * 1000) {
                try {
                    // サーバーに状態確認
                    const statusResponse = await fetch(`/files/chunk/status/${uploadId}`, {
                        credentials: 'include'
                    });

                    if (statusResponse.ok) {
                        const status = await statusResponse.json();
                        upload_id = uploadId;
                        startChunk = status.uploaded_chunks.length;

                        if (startChunk > 0) {
                            addActivityLog('upload', `${file.name} のアップロードを再開します (${startChunk}/${totalChunks} チャンク完了)`);
                            if (window.toast) toast.info(`アップロードを再開します (${Math.round(startChunk / totalChunks * 100)}% 完了)`);
                        }
                    }
                } catch (err) {
                    console.log('過去のセッションは利用できません:', err);
                    localStorage.removeItem(storageKey);
                }
            } else {
                localStorage.removeItem(storageKey);
            }
        }

        // 新規セッション作成
        if (!upload_id) {
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

            const result = await initResponse.json();
            upload_id = result.upload_id;

            // アップロードIDを設定
            setUploadChunkId(uploadId, upload_id);

            // セッション情報を保存
            localStorage.setItem(storageKey, JSON.stringify({
                uploadId: upload_id,
                timestamp: Date.now()
            }));

            addActivityLog('upload', `チャンクアップロード開始: ${file.name}`);
        }

        // 各チャンクをアップロード（途中から再開可能）
        for (let i = startChunk; i < totalChunks; i++) {
            const start = i * chunkSize;
            const end = Math.min(start + chunkSize, file.size);
            const chunk = file.slice(start, end);

            let retries = 3;
            let uploaded = false;

            while (retries > 0 && !uploaded) {
                try {
                    const uploadResponse = await fetch(`/files/chunk/upload/${upload_id}?chunk_index=${i}`, {
                        method: 'POST',
                        body: chunk,
                        credentials: 'include'
                    });

                    if (!uploadResponse.ok) {
                        throw new Error(`チャンク ${i + 1} のアップロードに失敗しました`);
                    }

                    uploaded = true;
                    const progress = Math.round(((i + 1) / totalChunks) * 100);
                    updateUploadProgress(uploadId, progress);
                } catch (err) {
                    retries--;
                    if (retries > 0) {
                        console.log(`チャンク ${i + 1} のリトライ中... (残り${retries}回)`);
                        await new Promise(resolve => setTimeout(resolve, 1000));
                    } else {
                        // リトライ失敗 - セッション情報は保持して中断
                        throw new Error(`チャンク ${i + 1} のアップロードに失敗しました。後で再開できます。`);
                    }
                }
            }
        }

        // 完了
        const completeResponse = await fetch(`/files/chunk/complete/${upload_id}`, {
            method: 'POST',
            credentials: 'include'
        });

        if (!completeResponse.ok) {
            throw new Error('チャンクアップロードの完了に失敗しました');
        }

        // 成功したらlocalStorageをクリア
        localStorage.removeItem(storageKey);

        addActivityLog('upload', `${file.name} のアップロードが完了しました`);
        if (window.toast) toast.success(`${file.name} のアップロードが完了しました`);
        updateUploadProgress(uploadId, 100, 'completed');
        await loadFiles(state.selectedDirectory);

    } catch (error) {
        console.error('チャンクアップロードエラー:', error);
        addActivityLog('error', error.message);
        if (window.toast) toast.error(error.message);
        updateUploadProgress(uploadId, upload.progress, 'failed');
        // エラー時もセッション情報は保持（レジューム可能にする）
    }
}


// ファイルダウンロード。
// window.location.href での遷移は連続実行すると相互に上書きされ、一括DLで1件しか
// 落ちない不具合になるため、隠しアンカーの click で個別にトリガーする。
function downloadFile(filename) {
    const url = `/files/download/${encodeURIComponent(state.selectedDirectory)}/${encodeURIComponent(filename)}`;
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    a.rel = 'noopener';
    document.body.appendChild(a);
    a.click();
    a.remove();
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

// Server-Sent Events接続
function connectSSE() {
    if (state.eventSource) {
        state.eventSource.close();
    }

    const eventSource = new EventSource('/api/events');
    state.eventSource = eventSource;

    const statusDot = document.getElementById('sse-status-dot');
    const statusText = document.getElementById('sse-status-text');

    if (statusDot) {
        statusDot.className = 'absolute top-1 right-1 w-2.5 h-2.5 bg-yellow-500 rounded-full border-2 border-white dark:border-gray-800';
    }
    if (statusText) {
        statusText.textContent = '接続中...';
    }

    eventSource.onopen = () => {
        if (statusDot) {
            statusDot.className = 'absolute top-1 right-1 w-2.5 h-2.5 bg-green-500 rounded-full border-2 border-white dark:border-gray-800';
        }
        if (statusText) {
            statusText.textContent = '接続済み';
        }
        // 接続成功したら再接続カウンターをリセット
        state.sseReconnectCount = 0;
    };

    eventSource.onerror = () => {
        if (statusDot) {
            statusDot.className = 'absolute top-1 right-1 w-2.5 h-2.5 bg-red-500 rounded-full border-2 border-white dark:border-gray-800';
        }
        if (statusText) {
            statusText.textContent = '切断';
        }

        // 指数バックオフで再接続（最大30秒）
        state.sseReconnectCount = (state.sseReconnectCount || 0) + 1;
        const delay = Math.min(1000 * Math.pow(2, state.sseReconnectCount), 30000);

        setTimeout(() => {
            if (state.user) {
                console.log(`SSE再接続試行 (${state.sseReconnectCount}回目, ${delay}ms待機)`);
                connectSSE();
            }
        }, delay);
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

    // アイコン設定
    const iconMap = {
        'upload': '<svg class="w-4 h-4 text-blue-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12"/></svg>',
        'download': '<svg class="w-4 h-4 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/></svg>',
        'delete': '<svg class="w-4 h-4 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/></svg>',
        'error': '<svg class="w-4 h-4 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>'
    };
    const icon = iconMap[type] || iconMap['error'];

    const logItem = document.createElement('div');
    logItem.className = 'flex items-start gap-3 p-3 hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors border-b border-gray-100 dark:border-gray-700';
    logItem.innerHTML = `
        <div class="flex-shrink-0 mt-0.5">
            ${icon}
        </div>
        <div class="flex-1 min-w-0">
            <p class="text-sm text-gray-900 dark:text-gray-100 break-words">
                ${fromSSE ? '<span class="inline-block w-2 h-2 bg-blue-500 rounded-full mr-1"></span>' : ''}${message}
            </p>
            <p class="text-xs text-gray-500 dark:text-gray-400 mt-0.5">${time}</p>
        </div>
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

// グローバルに公開（モーダルで使用）
window.formatFileSize = formatFileSize;
window.formatDate = formatDate;

// ファイル詳細モーダル表示
window.showFileDetail = function(file) {
    // Alpine.jsのイベントをディスパッチしてモーダルを開く
    window.dispatchEvent(new CustomEvent('open-file-detail', { detail: file }));
};

// 一括操作関数
window.toggleFileSelection = function(filename, checked) {
    if (checked) {
        state.selectedFiles.add(filename);
    } else {
        state.selectedFiles.delete(filename);
    }
    renderFiles();
};

window.toggleSelectAll = function(checked) {
    if (checked) {
        state.filteredFiles.forEach(file => state.selectedFiles.add(file.filename));
    } else {
        state.selectedFiles.clear();
    }
    renderFiles();
};

window.clearSelection = function() {
    state.selectedFiles.clear();
    renderFiles();
};

// 一括ダウンロード
window.bulkDownload = async function() {
    if (state.selectedFiles.size === 0) {
        if (window.toast) toast.warning('ファイルが選択されていません');
        return;
    }

    const selectedArray = Array.from(state.selectedFiles);
    if (window.toast) toast.info(`${selectedArray.length}個のファイルをダウンロード中...`);

    for (const filename of selectedArray) {
        await downloadFile(filename);
        // 連続ダウンロードの間隔を少し開ける
        await new Promise(resolve => setTimeout(resolve, 300));
    }

    if (window.toast) toast.success('一括ダウンロード完了');
};

// 一括削除
window.bulkDelete = async function() {
    if (state.selectedFiles.size === 0) {
        if (window.toast) toast.warning('ファイルが選択されていません');
        return;
    }

    const selectedArray = Array.from(state.selectedFiles);
    const confirmMessage = `${selectedArray.length}個のファイルを削除してもよろしいですか？\n\nこの操作は取り消せません。`;

    if (!confirm(confirmMessage)) {
        return;
    }

    let successCount = 0;
    let failCount = 0;

    for (const filename of selectedArray) {
        try {
            const response = await fetch(`/files/${encodeURIComponent(state.selectedDirectory)}/${encodeURIComponent(filename)}`, {
                method: 'DELETE',
                credentials: 'include'
            });

            if (response.ok) {
                successCount++;
                state.selectedFiles.delete(filename);
            } else {
                failCount++;
                console.error(`削除失敗: ${filename}`);
            }
        } catch (error) {
            failCount++;
            console.error(`削除エラー: ${filename}`, error);
        }
    }

    // 再読み込み
    await loadFiles(state.selectedDirectory);

    if (window.toast) {
        if (failCount === 0) {
            toast.success(`${successCount}個のファイルを削除しました`);
        } else {
            toast.warning(`${successCount}個削除、${failCount}個失敗`);
        }
    }
};

// 右クリックコンテキストメニュー
window.showContextMenu = function(event, file) {
    event.preventDefault();

    // コンテキストメニューのAlpine.jsインスタンスにアクセス
    const contextMenu = document.getElementById('context-menu');
    const alpineData = Alpine.$data(contextMenu);

    // ファイル情報と位置を設定
    alpineData.file = file;
    alpineData.x = event.clientX;
    alpineData.y = event.clientY;
    alpineData.show = true;

    // 画面外にはみ出さないように調整
    setTimeout(() => {
        const menuRect = contextMenu.getBoundingClientRect();
        const windowWidth = window.innerWidth;
        const windowHeight = window.innerHeight;

        if (alpineData.x + menuRect.width > windowWidth) {
            alpineData.x = windowWidth - menuRect.width - 10;
        }

        if (alpineData.y + menuRect.height > windowHeight) {
            alpineData.y = windowHeight - menuRect.height - 10;
        }
    }, 10);
};

// グローバルステートをウィンドウに公開（コンテキストメニューで使用）
window.state = state;

// ページ離脱時にSSE切断
window.addEventListener('beforeunload', () => {
    if (state.eventSource) {
        state.eventSource.close();
    }
    // 進行中のアップロードをキャンセル
    Object.values(activeUploads).forEach(upload => {
        if (upload.status === 'uploading') {
            cancelUpload(upload.id);
        }
    });
});

// アップロード管理
const activeUploads = {};
let uploadIdCounter = 0;

// アップロード追加（進行中リストに表示）
function addActiveUpload(file, directory) {
    const id = `upload_${uploadIdCounter++}`;
    activeUploads[id] = {
        id,
        file,
        directory,
        status: 'uploading', // uploading, completed, failed, cancelled
        progress: 0,
        uploadId: null, // チャンクアップロードの場合
        abortController: new AbortController()
    };
    renderActiveUploads();
    updateUploadBadge();
    return id;
}

// アップロード更新
function updateUploadProgress(id, progress, status = 'uploading') {
    if (activeUploads[id]) {
        activeUploads[id].progress = progress;
        activeUploads[id].status = status;
        renderActiveUploads();
        updateUploadBadge();
    }
}

// アップロードをチャンクアップロードIDと紐付け
function setUploadChunkId(id, uploadId) {
    if (activeUploads[id]) {
        activeUploads[id].uploadId = uploadId;
    }
}

// アップロードキャンセル
async function cancelUpload(id) {
    const upload = activeUploads[id];
    if (!upload) return;

    // AbortControllerでリクエストキャンセル
    upload.abortController.abort();

    // チャンクアップロードの場合はサーバー側もキャンセル
    if (upload.uploadId) {
        try {
            await fetch(`/files/chunk/cancel/${upload.uploadId}`, {
                method: 'DELETE',
                credentials: 'include'
            });
        } catch (err) {
            console.error('チャンクアップロードのキャンセルに失敗:', err);
        }
    }

    updateUploadProgress(id, upload.progress, 'cancelled');
    if (window.toast) toast.info(`${upload.file.name} のアップロードをキャンセルしました`);
}

// 完了済みアップロードをクリア
window.clearCompletedUploads = function() {
    Object.keys(activeUploads).forEach(id => {
        if (activeUploads[id].status !== 'uploading') {
            delete activeUploads[id];
        }
    });
    renderActiveUploads();
    updateUploadBadge();
};

// 進行中アップロード一覧を描画
function renderActiveUploads() {
    const container = document.getElementById('active-uploads-list');
    const uploads = Object.values(activeUploads);

    if (uploads.length === 0) {
        container.innerHTML = `
            <div class="text-center py-12 text-gray-500 dark:text-gray-400">
                <svg class="w-16 h-16 mx-auto mb-4 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12"/>
                </svg>
                <p class="text-sm">進行中のアップロードはありません</p>
            </div>
        `;
        return;
    }

    container.innerHTML = uploads.map(upload => {
        const statusIcons = {
            uploading: '⏳',
            completed: '✅',
            failed: '❌',
            cancelled: '⛔'
        };
        const statusColors = {
            uploading: 'text-blue-600',
            completed: 'text-green-600',
            failed: 'text-red-600',
            cancelled: 'text-gray-600'
        };

        return `
            <div class="bg-gray-50 dark:bg-gray-700 rounded-lg p-3 border border-gray-200 dark:border-gray-600">
                <div class="flex items-start justify-between mb-2">
                    <div class="flex-1 min-w-0">
                        <div class="flex items-center gap-2">
                            <span class="text-lg">${statusIcons[upload.status]}</span>
                            <p class="text-sm font-semibold text-gray-800 dark:text-white truncate">${upload.file.name}</p>
                        </div>
                        <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">
                            ${upload.directory} • ${formatFileSize(upload.file.size)}
                        </p>
                    </div>
                    ${upload.status === 'uploading' ? `
                        <button onclick="cancelUpload('${upload.id}')" class="ml-2 p-1.5 hover:bg-red-100 dark:hover:bg-red-900/30 text-red-500 rounded transition-colors" title="キャンセル">
                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                            </svg>
                        </button>
                    ` : ''}
                </div>

                ${upload.status === 'uploading' ? `
                    <div class="relative w-full bg-gray-200 dark:bg-gray-600 rounded-full h-2 overflow-hidden">
                        <div class="bg-primary-500 h-full rounded-full transition-all duration-300" style="width: ${upload.progress}%"></div>
                    </div>
                    <p class="text-xs text-gray-600 dark:text-gray-300 mt-1 text-right">${upload.progress}%</p>
                ` : `
                    <p class="text-xs ${statusColors[upload.status]} mt-1">
                        ${upload.status === 'completed' ? '完了' : upload.status === 'failed' ? '失敗' : 'キャンセル済み'}
                    </p>
                `}
            </div>
        `;
    }).join('');
}

// アップロード数バッジを更新
function updateUploadBadge() {
    const badge = document.getElementById('upload-count-badge');
    const uploadingCount = Object.values(activeUploads).filter(u => u.status === 'uploading').length;

    if (uploadingCount > 0) {
        badge.textContent = uploadingCount;
        badge.classList.remove('hidden');
    } else {
        badge.classList.add('hidden');
    }
}
