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
        <span class="text-white font-medium">${state.user.username}</span>
        <a href="/auth/logout" class="px-4 py-2 bg-red-500 hover:bg-red-600 text-white font-medium rounded-lg transition-colors">
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
        <div class="flex flex-col items-center justify-center py-20 px-4 animate-fade-in">
            <div class="w-32 h-32 bg-gradient-to-br from-discord-500 to-purple-600 rounded-full flex items-center justify-center mb-8 shadow-2xl animate-bounce">
                <svg class="w-16 h-16 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/>
                </svg>
            </div>
            <h2 class="text-3xl font-bold text-gray-800 dark:text-white mb-4">ようこそ！</h2>
            <p class="text-gray-600 dark:text-gray-400 text-center mb-8 max-w-md">
                左のサイドバーからフォルダを選択して、ファイルの管理を始めましょう
            </p>
            <div class="flex flex-col sm:flex-row gap-4">
                <div class="flex items-center gap-3 px-6 py-3 bg-gray-50 dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700">
                    <svg class="w-6 h-6 text-discord-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/>
                    </svg>
                    <div>
                        <div class="text-sm font-semibold text-gray-800 dark:text-white">フォルダを選択</div>
                        <div class="text-xs text-gray-500 dark:text-gray-400">サイドバーから選ぶ</div>
                    </div>
                </div>
                <div class="flex items-center gap-3 px-6 py-3 bg-gray-50 dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700">
                    <svg class="w-6 h-6 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12"/>
                    </svg>
                    <div>
                        <div class="text-sm font-semibold text-gray-800 dark:text-white">ファイルアップロード</div>
                        <div class="text-xs text-gray-500 dark:text-gray-400">ドラッグ&ドロップ対応</div>
                    </div>
                </div>
            </div>
            <!-- モバイル用サイドバー開くボタン -->
            <button onclick="document.querySelector('[\\@click*=sidebarOpen]').click()" class="mt-8 lg:hidden px-6 py-3 bg-discord-500 hover:bg-discord-600 text-white font-semibold rounded-xl transition-all transform hover:scale-105 flex items-center gap-2">
                <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16"/>
                </svg>
                フォルダを開く
            </button>
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

        return `
        <div class="group cursor-pointer px-3 py-2.5 rounded-lg transition-all mb-1 ${
            isSelected
                ? 'bg-gradient-to-r from-discord-500 to-discord-600 text-white shadow-lg shadow-discord-500/30'
                : 'hover:bg-gray-100 dark:hover:bg-gray-700 text-gray-700 dark:text-gray-300'
        }"
             onclick="selectDirectory('${dir.path}')">
            <div class="flex items-center gap-2 mb-2">
                <div class="flex-shrink-0 w-8 h-8 ${isSelected ? 'bg-white/20' : 'bg-discord-500/10'} rounded-lg flex items-center justify-center">
                    <svg class="w-4 h-4 ${isSelected ? 'text-white' : 'text-discord-500'}" fill="currentColor" viewBox="0 0 20 20">
                        <path d="M2 6a2 2 0 012-2h5l2 2h5a2 2 0 012 2v6a2 2 0 01-2 2H4a2 2 0 01-2-2V6z"/>
                    </svg>
                </div>
                <span class="font-semibold text-sm truncate flex-1">${dir.path}</span>
            </div>
            <div class="flex gap-1 flex-wrap">
                ${permissionBadges}
            </div>
        </div>
        `;
    }).join('');
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
    const desktopSkeleton = `
        <div class="hidden md:block bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden">
            <div class="bg-gradient-to-r from-gray-50 to-gray-100 dark:from-gray-700 dark:to-gray-800 border-b border-gray-200 dark:border-gray-600 px-6 py-4">
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

    // モバイル用カードSkeleton
    const mobileSkeleton = `
        <div class="md:hidden space-y-3">
            ${Array.from({ length: 5 }, () => `
                <div class="bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700 p-4">
                    <div class="flex items-start gap-3">
                        <div class="w-12 h-12 bg-gray-200 dark:bg-gray-700 rounded-xl skeleton"></div>
                        <div class="flex-1 space-y-2">
                            <div class="h-4 bg-gray-200 dark:bg-gray-700 rounded skeleton"></div>
                            <div class="flex gap-3">
                                <div class="w-16 h-3 bg-gray-200 dark:bg-gray-700 rounded skeleton"></div>
                                <div class="w-24 h-3 bg-gray-200 dark:bg-gray-700 rounded skeleton"></div>
                            </div>
                        </div>
                    </div>
                    <div class="flex gap-2 mt-3 pt-3 border-t border-gray-100 dark:border-gray-700">
                        <div class="flex-1 h-10 bg-gray-200 dark:bg-gray-700 rounded-lg skeleton"></div>
                        <div class="w-10 h-10 bg-gray-200 dark:bg-gray-700 rounded-lg skeleton"></div>
                    </div>
                </div>
            `).join('')}
        </div>
    `;

    container.innerHTML = desktopSkeleton + mobileSkeleton;
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
function renderFiles() {
    const container = document.getElementById('files-list');

    if (state.filteredFiles.length === 0) {
        // 改善されたEmpty State
        if (state.searchQuery) {
            container.innerHTML = `
                <div class="flex flex-col items-center justify-center py-16 px-4">
                    <div class="w-24 h-24 bg-gray-100 dark:bg-gray-700 rounded-full flex items-center justify-center mb-6">
                        <svg class="w-12 h-12 text-gray-400 dark:text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/>
                        </svg>
                    </div>
                    <h3 class="text-xl font-semibold text-gray-800 dark:text-white mb-2">検索結果なし</h3>
                    <p class="text-gray-500 dark:text-gray-400 text-center">「<span class="font-semibold">${state.searchQuery}</span>」に一致するファイルが見つかりませんでした</p>
                </div>
            `;
        } else {
            container.innerHTML = `
                <div class="flex flex-col items-center justify-center py-16 px-4">
                    <div class="w-24 h-24 bg-gradient-to-br from-discord-500/10 to-purple-500/10 rounded-full flex items-center justify-center mb-6">
                        <svg class="w-12 h-12 text-discord-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12"/>
                        </svg>
                    </div>
                    <h3 class="text-xl font-semibold text-gray-800 dark:text-white mb-2">ファイルがありません</h3>
                    <p class="text-gray-500 dark:text-gray-400 mb-6 text-center">ファイルをドラッグ&ドロップするか、アップロードボタンをクリックしてください</p>
                    <button onclick="document.getElementById('file-input').click()" class="px-6 py-3 bg-discord-500 hover:bg-discord-600 text-white font-semibold rounded-lg transition-all transform hover:scale-105 flex items-center gap-2">
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

    // viewModeを状態から取得
    const viewMode = state.viewMode;

    if (viewMode === 'list') {
        // 一括操作ツールバー（選択されたファイルがある場合のみ表示）
        const bulkActionsBar = state.selectedFiles.size > 0 ? `
            <div class="mb-4 p-4 bg-discord-500 dark:bg-discord-600 rounded-xl flex items-center justify-between text-white animate-fade-in">
                <div class="flex items-center gap-3">
                    <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/>
                    </svg>
                    <span class="font-semibold">${state.selectedFiles.size}個のファイルを選択中</span>
                </div>
                <div class="flex items-center gap-2">
                    <button onclick="window.bulkDownload()" class="px-4 py-2 bg-white/20 hover:bg-white/30 rounded-lg transition-colors font-medium flex items-center gap-2">
                        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/>
                        </svg>
                        一括ダウンロード
                    </button>
                    ${canDelete ? `
                        <button onclick="window.bulkDelete()" class="px-4 py-2 bg-red-500 hover:bg-red-600 rounded-lg transition-colors font-medium flex items-center gap-2">
                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
                            </svg>
                            一括削除
                        </button>
                    ` : ''}
                    <button onclick="window.clearSelection()" class="px-4 py-2 bg-white/20 hover:bg-white/30 rounded-lg transition-colors font-medium">
                        選択解除
                    </button>
                </div>
            </div>
        ` : '';

        // リスト表示 - デスクトップ: テーブル、モバイル: カード
        container.innerHTML = bulkActionsBar + `
            <!-- デスクトップ用テーブル (md以上) -->
            <div class="hidden md:block bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden">
                <table class="w-full">
                    <thead class="bg-gradient-to-r from-gray-50 to-gray-100 dark:from-gray-700 dark:to-gray-800 border-b border-gray-200 dark:border-gray-600">
                        <tr>
                            <th class="px-6 py-4 w-12">
                                <input type="checkbox"
                                       onchange="window.toggleSelectAll(this.checked)"
                                       ${state.selectedFiles.size === state.filteredFiles.length && state.filteredFiles.length > 0 ? 'checked' : ''}
                                       class="w-4 h-4 text-discord-500 bg-gray-100 border-gray-300 rounded focus:ring-discord-500 focus:ring-2 cursor-pointer">
                            </th>
                            <th class="px-6 py-4 text-left text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">ファイル名</th>
                            <th class="px-6 py-4 text-left text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">サイズ</th>
                            <th class="px-6 py-4 text-left text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">更新日時</th>
                            <th class="px-6 py-4 text-right text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">アクション</th>
                        </tr>
                    </thead>
                    <tbody class="divide-y divide-gray-200 dark:divide-gray-700">
                        ${state.filteredFiles.map(file => {
                            const filename = file.original_name || file.filename;
                            const iconConfig = window.getFileIconSVG ? window.getFileIconSVG(filename) : { svg: '', color: 'text-gray-500', bg: 'bg-gray-50' };
                            const isSelected = state.selectedFiles.has(file.filename);

                            return `
                            <tr class="hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-all group ${isSelected ? 'bg-discord-50 dark:bg-discord-900/20' : ''}"
                                oncontextmenu="window.showContextMenu(event, ${JSON.stringify(file).replace(/"/g, '&quot;')})">
                                <td class="px-6 py-4">
                                    <input type="checkbox"
                                           ${isSelected ? 'checked' : ''}
                                           onchange="window.toggleFileSelection('${file.filename}', this.checked)"
                                           class="w-4 h-4 text-discord-500 bg-gray-100 border-gray-300 rounded focus:ring-discord-500 focus:ring-2 cursor-pointer">
                                </td>
                                <td class="px-6 py-4">
                                    <div class="flex items-center gap-3">
                                        <div class="flex-shrink-0 w-10 h-10 ${iconConfig.bg} rounded-lg flex items-center justify-center ${iconConfig.color} transition-transform group-hover:scale-110">
                                            ${iconConfig.svg}
                                        </div>
                                        <span class="font-medium text-gray-800 dark:text-white truncate max-w-md" title="${filename}">${filename}</span>
                                    </div>
                                </td>
                                <td class="px-6 py-4 text-sm text-gray-600 dark:text-gray-400">${formatFileSize(file.size)}</td>
                                <td class="px-6 py-4 text-sm text-gray-600 dark:text-gray-400">${formatDate(file.modified_at)}</td>
                                <td class="px-6 py-4 text-right">
                                    <div class="flex justify-end gap-2">
                                        <button onclick="window.showFileDetail(${JSON.stringify(file).replace(/"/g, '&quot;')})" title="詳細"
                                                class="p-2 text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-all transform hover:scale-110">
                                            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>
                                            </svg>
                                        </button>
                                        <button onclick="downloadFile('${file.filename}')" title="ダウンロード"
                                                class="p-2 text-discord-500 hover:bg-discord-50 dark:hover:bg-discord-900/20 rounded-lg transition-all transform hover:scale-110">
                                            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/>
                                            </svg>
                                        </button>
                                        ${canDelete ? `
                                            <button onclick="deleteFile('${file.filename}')" title="削除"
                                                    class="p-2 text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg transition-all transform hover:scale-110">
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

            <!-- モバイル用カード (md未満) -->
            <div class="md:hidden space-y-3">
                ${state.filteredFiles.map(file => {
                    const filename = file.original_name || file.filename;
                    const iconConfig = window.getFileIconSVG ? window.getFileIconSVG(filename) : { svg: '', color: 'text-gray-500', bg: 'bg-gray-50' };
                    const isSelected = state.selectedFiles.has(file.filename);

                    return `
                    <div class="bg-white dark:bg-gray-800 rounded-xl border ${isSelected ? 'border-discord-500 bg-discord-50 dark:bg-discord-900/20' : 'border-gray-200 dark:border-gray-700'} p-4 hover:shadow-lg hover:border-discord-500 dark:hover:border-discord-500 transition-all transform hover:scale-[1.02] active:scale-[0.98]"
                         oncontextmenu="window.showContextMenu(event, ${JSON.stringify(file).replace(/"/g, '&quot;')})">
                        <div class="flex items-start gap-3">
                            <!-- チェックボックス -->
                            <input type="checkbox"
                                   ${isSelected ? 'checked' : ''}
                                   onchange="window.toggleFileSelection('${file.filename}', this.checked)"
                                   class="mt-1 w-5 h-5 text-discord-500 bg-gray-100 border-gray-300 rounded focus:ring-discord-500 focus:ring-2 cursor-pointer">
                            <!-- アイコン -->
                            <div class="flex-shrink-0 w-12 h-12 ${iconConfig.bg} rounded-xl flex items-center justify-center ${iconConfig.color}">
                                ${iconConfig.svg}
                            </div>

                            <!-- 詳細 -->
                            <div class="flex-1 min-w-0">
                                <h4 class="font-semibold text-gray-800 dark:text-white truncate mb-1" title="${filename}">${filename}</h4>
                                <div class="flex items-center gap-3 text-xs text-gray-500 dark:text-gray-400">
                                    <span class="flex items-center gap-1">
                                        <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z"/>
                                        </svg>
                                        ${formatFileSize(file.size)}
                                    </span>
                                    <span class="flex items-center gap-1">
                                        <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"/>
                                        </svg>
                                        ${formatDate(file.modified_at)}
                                    </span>
                                </div>
                            </div>
                        </div>

                        <!-- アクション -->
                        <div class="flex gap-2 mt-3 pt-3 border-t border-gray-100 dark:border-gray-700">
                            <button onclick="window.showFileDetail(${JSON.stringify(file).replace(/"/g, '&quot;')})"
                                    class="px-4 py-2.5 bg-gray-100 hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600 text-gray-800 dark:text-white font-semibold rounded-lg transition-all transform active:scale-95">
                                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>
                                </svg>
                            </button>
                            <button onclick="downloadFile('${file.filename}')"
                                    class="flex-1 flex items-center justify-center gap-2 px-4 py-2.5 bg-discord-500 hover:bg-discord-600 active:bg-discord-700 text-white font-semibold rounded-lg transition-all transform active:scale-95">
                                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/>
                                </svg>
                                ダウンロード
                            </button>
                            ${canDelete ? `
                                <button onclick="deleteFile('${file.filename}')"
                                        class="px-4 py-2.5 bg-red-500 hover:bg-red-600 active:bg-red-700 text-white font-semibold rounded-lg transition-all transform active:scale-95">
                                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
                                    </svg>
                                </button>
                            ` : ''}
                        </div>
                    </div>
                    `;
                }).join('')}
            </div>
        `;
    } else {
        // グリッド表示（SVGアイコン + マイクロインタラクション）
        container.innerHTML = `
            <div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
                ${state.filteredFiles.map(file => {
                    const filename = file.original_name || file.filename;
                    const iconConfig = window.getFileIconSVG ? window.getFileIconSVG(filename) : { svg: '', color: 'text-gray-500', bg: 'bg-gray-50' };

                    return `
                    <div class="bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700 p-4 hover:shadow-xl hover:border-discord-500 dark:hover:border-discord-500 transition-all transform hover:scale-105 hover:-translate-y-1 active:scale-100 group cursor-pointer"
                         onclick="window.showFileDetail(${JSON.stringify(file).replace(/"/g, '&quot;')})"
                         oncontextmenu="window.showContextMenu(event, ${JSON.stringify(file).replace(/"/g, '&quot;')})">
                        <div class="flex flex-col items-center text-center">
                            <div class="w-20 h-20 ${iconConfig.bg} rounded-xl flex items-center justify-center ${iconConfig.color} mb-3 p-4 transition-transform group-hover:scale-110 group-hover:rotate-3">
                                ${iconConfig.svg}
                            </div>
                            <div class="font-semibold text-sm text-gray-800 dark:text-white truncate w-full mb-1" title="${filename}">${filename}</div>
                            <div class="text-xs text-gray-500 dark:text-gray-400 mb-3">${formatFileSize(file.size)}</div>
                            <div class="flex gap-2 w-full" onclick="event.stopPropagation()">
                                <button onclick="downloadFile('${file.filename}')"
                                        class="flex-1 flex items-center justify-center gap-1 px-3 py-2 bg-discord-500 hover:bg-discord-600 active:bg-discord-700 text-white text-xs font-semibold rounded-lg transition-all transform hover:scale-105 active:scale-95">
                                    <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/>
                                    </svg>
                                    DL
                                </button>
                                ${canDelete ? `
                                    <button onclick="deleteFile('${file.filename}')"
                                            class="px-3 py-2 bg-red-500 hover:bg-red-600 active:bg-red-700 text-white text-xs font-semibold rounded-lg transition-all transform hover:scale-105 active:scale-95">
                                        <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
                                        </svg>
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

    showProgress(true);
    setProgress(0);

    try {
        const xhr = new XMLHttpRequest();

        // プログレス更新
        xhr.upload.addEventListener('progress', (e) => {
            if (e.lengthComputable) {
                const percent = Math.round((e.loaded / e.total) * 100);
                setProgress(percent);
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
    } finally {
        setTimeout(() => showProgress(false), 500);
    }
}

// チャンクアップロード（レジューム対応）
async function uploadFileInChunks(file, uploadId) {
    const upload = activeUploads[uploadId];
    if (!upload) return;

    const chunkSize = 20 * 1024 * 1024; // 20MB
    const totalChunks = Math.ceil(file.size / chunkSize);
    const storageKey = `upload_${file.name}_${file.size}_${state.selectedDirectory}`;

    showProgress(true);
    setProgress(0);

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
                            setProgress(Math.round(startChunk / totalChunks * 100));
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
                    setProgress(progress);
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
    if (!progressDiv) return; // 要素が存在しない場合は何もしない

    if (show) {
        progressDiv.classList.remove('hidden');
    } else {
        progressDiv.classList.add('hidden');
        setProgress(0);
    }
}

function setProgress(percent) {
    const progressFill = document.getElementById('progress-fill');
    const progressText = document.getElementById('progress-text');

    if (!progressFill || !progressText) return; // 要素が存在しない場合は何もしない

    progressFill.style.width = percent + '%';
    progressText.textContent = percent + '%';
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
                        <div class="bg-discord-500 h-full rounded-full transition-all duration-300" style="width: ${upload.progress}%"></div>
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
