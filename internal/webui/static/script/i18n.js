const AVAILABLE_LANGS = {
    zh: "中文",
    en: "English",
    ja:'日本語',
    vi: "Tiếng Việt"
};

const DEFAULT_LANG = "zh";
const LANG_STORAGE_KEY = "kano_lang";
let currentLang = DEFAULT_LANG;
let translations = {};

const PROJECT_TERMS = {
    zh: `项目：UFI-TOOLS U60 Pro Remake<br>许可证：GNU AGPL-3.0-or-later；上游 UFI-TOOLS 的 MIT 许可与归属见 NOTICE。<br><br>本项目默认不向作者或第三方发送设备标识、状态、日志或统计信息，也不包含远程黑名单或停用开关。只有在管理员明确配置插件商店、更新源、短信 Webhook 或其他外部地址并主动使用相应功能时，数据才会发送到该地址。<br><br>设备管理、插件、更新、安装和固件相关操作可能造成配置丢失、网络中断或设备无法启动。请先备份并确认恢复路径。本软件按“现状”提供，不附带任何明示或暗示担保。`,
    en: `Project: UFI-TOOLS U60 Pro Remake<br>License: GNU AGPL-3.0-or-later. The upstream UFI-TOOLS MIT license and attribution are preserved in NOTICE.<br><br>This project does not send device identifiers, status, logs, or analytics to its authors or third parties by default, and it contains no remote blacklist or kill switch. Data leaves the device only when an administrator explicitly configures and uses an external plugin store, update source, SMS webhook, or another external endpoint.<br><br>Device management, plugins, updates, installation, and firmware operations can cause configuration loss, network interruption, or an unbootable device. Back up first and verify a recovery path. This software is provided as-is, without warranty.`,
    ja: `プロジェクト：UFI-TOOLS U60 Pro Remake<br>ライセンス：GNU AGPL-3.0-or-later。上流 UFI-TOOLS の MIT ライセンスと帰属表示は NOTICE に保存されています。<br><br>本プロジェクトは、既定ではデバイス識別子、状態、ログ、統計情報を作者または第三者へ送信せず、遠隔ブラックリストや停止スイッチも含みません。管理者が外部プラグインストア、更新元、SMS Webhook、その他の外部送信先を明示的に設定して使用した場合にのみ、その送信先へデータが送られます。<br><br>デバイス管理、プラグイン、更新、インストール、ファームウェア操作は、設定消失、通信断、起動不能を招く可能性があります。事前にバックアップと復旧経路を確認してください。本ソフトウェアは無保証で現状のまま提供されます。`,
    vi: `Dự án: UFI-TOOLS U60 Pro Remake<br>Giấy phép: GNU AGPL-3.0-or-later. Giấy phép MIT và ghi công của UFI-TOOLS nguồn được lưu trong NOTICE.<br><br>Theo mặc định, dự án không gửi mã định danh thiết bị, trạng thái, nhật ký hoặc dữ liệu thống kê cho tác giả hay bên thứ ba, đồng thời không có danh sách chặn hoặc công tắc vô hiệu hóa từ xa. Dữ liệu chỉ rời thiết bị khi quản trị viên chủ động cấu hình và sử dụng kho plugin, nguồn cập nhật, SMS webhook hoặc điểm cuối bên ngoài khác.<br><br>Quản lý thiết bị, plugin, cập nhật, cài đặt và thao tác firmware có thể làm mất cấu hình, gián đoạn mạng hoặc khiến thiết bị không khởi động. Hãy sao lưu và xác minh đường khôi phục trước. Phần mềm được cung cấp nguyên trạng, không có bảo hành.`
};

function detectBrowserLang() {
    const lang = navigator.language || navigator.userLanguage;
    const shortLang = lang.slice(0, 2);
    return AVAILABLE_LANGS[shortLang] ? shortLang : DEFAULT_LANG;
}

async function loadLanguage(lang) {
    try {
        const res = await fetch(`/lang/${lang}.json`);
        translations = await res.json();
        currentLang = lang;
        localStorage.setItem(LANG_STORAGE_KEY, lang);
        applyTranslations();
        updateSelectValue(lang);
    } catch (e) {
        console.error(`Failed to load language: ${lang}`, e);
    }
}

function applyTranslations() {
    document.querySelectorAll("[data-i18n]").forEach(el => {
        const key = el.getAttribute("data-i18n");
        if (translations[key]) {
            el.textContent = translations[key];
        }
    });
    document.querySelectorAll("[data-i18n-placeholder]").forEach(el => {
        const key = el.getAttribute("data-i18n-placeholder");
        if (translations[key]) {
            el.placeholder = translations[key];
        }
    });
}

function createLanguageSelector() {
    const selector = document.querySelector("#LANG");

    Object.entries(AVAILABLE_LANGS).forEach(([key, label]) => {
        const option = document.createElement("option");
        option.value = key;
        option.textContent = label;
        selector.appendChild(option);
    });

    selector.addEventListener("change", (e) => {
        const selectedLang = e.target.value;
        loadLanguage(selectedLang);
    });
}

function updateSelectValue(lang) {
    const selector = document.querySelector("#LANG");
    selector.value = lang;
}


function t(key, fallback = "") {
    if (key === "useTerms") {
        return PROJECT_TERMS[currentLang] || PROJECT_TERMS[DEFAULT_LANG];
    }
    return translations[key] || fallback || key;
}

// 初始化
(function initI18n() {
    createLanguageSelector();
    const savedLang = localStorage.getItem(LANG_STORAGE_KEY);
    const langToLoad = savedLang || detectBrowserLang();
    loadLanguage(langToLoad);
})();
