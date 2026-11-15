package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2/lang"
)

// 지원 언어. 순서는 환경설정 목록에 보이는 순서다.
var languages = []struct {
	code, name string // name 은 그 언어 자체로 표기한다
}{
	{"ko", "한국어"},
	{"en", "English"},
	{"ja", "日本語"},
	{"zh", "简体中文"},
}

// curLang 은 현재 화면 언어 코드다. setLanguage 로 바꾼다.
var curLang = "en"

// setLanguage 는 설정값("" 이면 시스템 언어)으로 화면 언어를 정한다.
func setLanguage(pref string) {
	if pref == "" {
		pref = systemLanguage()
	}
	if _, ok := messages["app.title"][pref]; ok {
		curLang = pref
	} else {
		curLang = "en"
	}
}

// systemLanguage 는 OS 언어를 지원 언어 코드로 바꾼다. 지원하지 않으면 영어.
func systemLanguage() string {
	tag := strings.ToLower(lang.SystemLocale().LanguageString()) // 예: "ko-kr"
	for _, l := range languages {
		if tag == l.code || strings.HasPrefix(tag, l.code+"-") || strings.HasPrefix(tag, l.code+"_") {
			return l.code
		}
	}
	return "en"
}

// msg 는 나중에(언어가 바뀐 뒤에도) 번역할 수 있게 키와 인자를 묶어 둔 것이다.
// T 의 인자로 넘기면 그 자리에서 번역된다.
type msg struct {
	key  string
	args []any
}

func m(key string, args ...any) msg { return msg{key, args} }

// T 는 현재 언어로 번역한다. 번역이 없으면 영어, 그것도 없으면 키를 돌려준다.
func T(key string, args ...any) string {
	s := messages[key][curLang]
	if s == "" {
		s = messages[key]["en"]
	}
	if s == "" {
		s = key
	}
	if len(args) == 0 {
		return s
	}
	resolved := make([]any, len(args))
	for i, a := range args {
		if mm, ok := a.(msg); ok {
			resolved[i] = T(mm.key, mm.args...)
		} else {
			resolved[i] = a
		}
	}
	return fmt.Sprintf(s, resolved...)
}

// messages[키][언어] = 문구. 번역을 함께 보기 쉽도록 키 단위로 모아 둔다.
var messages = map[string]map[string]string{
	"app.title": {
		"ko": "XMView — CCTV 뷰어",
		"en": "XMView — CCTV Viewer",
		"ja": "XMView — CCTVビューア",
		"zh": "XMView — CCTV 查看器",
	},

	// ---- 접속 폼 ----
	"field.name":     {"ko": "이름", "en": "Name", "ja": "名前", "zh": "名称"},
	"field.host":     {"ko": "주소", "en": "Address", "ja": "アドレス", "zh": "地址"},
	"field.port":     {"ko": "포트", "en": "Port", "ja": "ポート", "zh": "端口"},
	"field.user":     {"ko": "아이디", "en": "User ID", "ja": "ユーザーID", "zh": "用户名"},
	"field.pass":     {"ko": "비밀번호", "en": "Password", "ja": "パスワード", "zh": "密码"},
	"field.channels": {"ko": "채널", "en": "Channels", "ja": "チャンネル", "zh": "通道"},
	"field.stream":   {"ko": "스트림", "en": "Stream", "ja": "ストリーム", "zh": "码流"},
	"field.saved":    {"ko": "저장목록", "en": "Saved", "ja": "保存済み", "zh": "已保存"},

	"ph.name":  {"ko": "이름(저장용)", "en": "Name (for saving)", "ja": "名前（保存用）", "zh": "名称（用于保存）"},
	"ph.host":  {"ko": "IP 또는 도메인", "en": "IP or domain", "ja": "IPまたはドメイン", "zh": "IP 或域名"},
	"ph.pass":  {"ko": "비밀번호", "en": "Password", "ja": "パスワード", "zh": "密码"},
	"ph.saved": {"ko": "저장된 접속 선택", "en": "Select a saved connection", "ja": "保存した接続を選択", "zh": "选择已保存的连接"},

	"stream.main":  {"ko": "Main(고화질)", "en": "Main (high quality)", "ja": "Main（高画質）", "zh": "Main（高清）"},
	"stream.extra": {"ko": "Extra1(저지연)", "en": "Extra1 (low latency)", "ja": "Extra1（低遅延）", "zh": "Extra1（低延迟）"},

	// ---- 버튼 ----
	"btn.connect":   {"ko": "연결", "en": "Connect", "ja": "接続", "zh": "连接"},
	"btn.save":      {"ko": "저장", "en": "Save", "ja": "保存", "zh": "保存"},
	"btn.panelHide": {"ko": "◀ 접속 설정 숨기기", "en": "◀ Hide connection", "ja": "◀ 接続設定を隠す", "zh": "◀ 隐藏连接设置"},
	"btn.panelShow": {"ko": "▶ 접속 설정", "en": "▶ Connection", "ja": "▶ 接続設定", "zh": "▶ 连接设置"},
	"btn.prefs":     {"ko": "환경설정", "en": "Preferences", "ja": "環境設定", "zh": "偏好设置"},
	"btn.install":   {"ko": "설치", "en": "Install", "ja": "インストール", "zh": "安装"},
	"btn.cancel":    {"ko": "취소", "en": "Cancel", "ja": "キャンセル", "zh": "取消"},
	"btn.ok":        {"ko": "확인", "en": "OK", "ja": "OK", "zh": "确定"},
	"btn.close":     {"ko": "닫기", "en": "Close", "ja": "閉じる", "zh": "关闭"},

	// ---- 상태 표시줄 ----
	"status.idle":            {"ko": "대기 중", "en": "Idle", "ja": "待機中", "zh": "待机"},
	"status.idleAutoInstall": {"ko": "대기 중 — 연결 시 ffmpeg 를 자동 설치합니다", "en": "Idle — ffmpeg will be installed when you connect", "ja": "待機中 — 接続時に ffmpeg を自動インストールします", "zh": "待机 — 连接时将自动安装 ffmpeg"},
	"status.idleNeedFF":      {"ko": "대기 중 — ffmpeg 필요: %s", "en": "Idle — ffmpeg required: %s", "ja": "待機中 — ffmpeg が必要: %s", "zh": "待机 — 需要 ffmpeg：%s"},
	"status.needHost":        {"ko": "주소를 입력하세요", "en": "Enter an address", "ja": "アドレスを入力してください", "zh": "请输入地址"},
	"status.badChannel":      {"ko": "채널 목록 오류: %s", "en": "Invalid channel list: %s", "ja": "チャンネル一覧のエラー: %s", "zh": "通道列表错误：%s"},
	"status.needChannel":     {"ko": "채널을 입력하세요", "en": "Enter channels", "ja": "チャンネルを入力してください", "zh": "请输入通道"},
	"status.streaming":       {"ko": "%d개 채널 · %s · GPU 디코드", "en": "%d channel(s) · %s · GPU decode", "ja": "%d チャンネル · %s · GPU デコード", "zh": "%d 个通道 · %s · GPU 解码"},
	"status.needNameHost":    {"ko": "이름과 주소를 입력하세요", "en": "Enter a name and address", "ja": "名前とアドレスを入力してください", "zh": "请输入名称和地址"},
	"status.saved":           {"ko": "저장됨: %s", "en": "Saved: %s", "ja": "保存しました: %s", "zh": "已保存：%s"},
	"status.noFF":            {"ko": "ffmpeg 없음 — %s", "en": "ffmpeg not found — %s", "ja": "ffmpeg がありません — %s", "zh": "未找到 ffmpeg — %s"},
	"status.declined":        {"ko": "ffmpeg 없음 — 설치해야 영상이 나옵니다", "en": "ffmpeg not installed — video needs it", "ja": "ffmpeg 未インストール — 映像の表示に必要です", "zh": "未安装 ffmpeg — 需要安装才能显示视频"},
	"status.installed":       {"ko": "ffmpeg 설치 완료", "en": "ffmpeg installed", "ja": "ffmpeg のインストールが完了しました", "zh": "ffmpeg 安装完成"},

	// ---- ffmpeg 설치 ----
	"ff.needTitle": {"ko": "ffmpeg 필요", "en": "ffmpeg required", "ja": "ffmpeg が必要です", "zh": "需要 ffmpeg"},
	"ff.needBody": {
		"ko": "영상 디코딩에 ffmpeg 가 필요합니다.\n\n%s",
		"en": "ffmpeg is required to decode video.\n\n%s",
		"ja": "映像のデコードに ffmpeg が必要です。\n\n%s",
		"zh": "解码视频需要 ffmpeg。\n\n%s",
	},
	"ff.installTitle": {"ko": "ffmpeg 자동 설치", "en": "Install ffmpeg", "ja": "ffmpeg の自動インストール", "zh": "自动安装 ffmpeg"},
	"ff.installBody": {
		"ko": "영상 디코딩에 ffmpeg 가 필요하지만 설치돼 있지 않습니다.\n\n지금 자동으로 받아올까요?\n\n출처: %s\n크기: %s (처음 한 번만)\n내려받은 파일은 SHA-256 으로 검증합니다.",
		"en": "ffmpeg is required to decode video but is not installed.\n\nDownload it automatically now?\n\nSource: %s\nSize: %s (first time only)\nThe download is verified with SHA-256.",
		"ja": "映像のデコードに ffmpeg が必要ですが、インストールされていません。\n\n今すぐ自動でダウンロードしますか？\n\n入手元: %s\nサイズ: %s（初回のみ）\nダウンロードしたファイルは SHA-256 で検証します。",
		"zh": "解码视频需要 ffmpeg，但尚未安装。\n\n现在自动下载吗？\n\n来源：%s\n大小：%s（仅首次）\n下载的文件将通过 SHA-256 校验。",
	},
	"ff.progressTitle": {"ko": "ffmpeg 설치 중", "en": "Installing ffmpeg", "ja": "ffmpeg をインストール中", "zh": "正在安装 ffmpeg"},
	"ff.starting":      {"ko": "시작 중…", "en": "Starting…", "ja": "開始中…", "zh": "正在开始…"},
	"ff.failTitle":     {"ko": "ffmpeg 설치 실패", "en": "ffmpeg installation failed", "ja": "ffmpeg のインストールに失敗しました", "zh": "ffmpeg 安装失败"},
	"ff.failBody": {
		"ko": "%s\n\n수동 설치: %s",
		"en": "%s\n\nManual install: %s",
		"ja": "%s\n\n手動インストール: %s",
		"zh": "%s\n\n手动安装：%s",
	},

	"phase.download": {"ko": "다운로드 중", "en": "Downloading", "ja": "ダウンロード中", "zh": "正在下载"},
	"phase.verify":   {"ko": "검증 중", "en": "Verifying", "ja": "検証中", "zh": "正在校验"},
	"phase.extract":  {"ko": "압축 해제 중", "en": "Extracting", "ja": "展開中", "zh": "正在解压"},
	"phase.done":     {"ko": "완료", "en": "Done", "ja": "完了", "zh": "完成"},

	"hint.mac":   {"ko": "터미널에서: brew install ffmpeg", "en": "In Terminal: brew install ffmpeg", "ja": "ターミナルで: brew install ffmpeg", "zh": "在终端中：brew install ffmpeg"},
	"hint.linux": {"ko": "배포판 패키지로: sudo apt install ffmpeg", "en": "With your distro's package: sudo apt install ffmpeg", "ja": "ディストリビューションのパッケージで: sudo apt install ffmpeg", "zh": "使用发行版软件包：sudo apt install ffmpeg"},
	"hint.other": {"ko": "https://ffmpeg.org/download.html 참고", "en": "See https://ffmpeg.org/download.html", "ja": "https://ffmpeg.org/download.html を参照", "zh": "请参阅 https://ffmpeg.org/download.html"},

	// ---- 환경설정 ----
	"prefs.title":    {"ko": "환경설정", "en": "Preferences", "ja": "環境設定", "zh": "偏好设置"},
	"prefs.language": {"ko": "언어", "en": "Language", "ja": "言語", "zh": "语言"},
	"prefs.theme":    {"ko": "화면 모드", "en": "Appearance", "ja": "表示モード", "zh": "外观"},
	"prefs.system":   {"ko": "시스템 설정 따름", "en": "System default", "ja": "システム設定に従う", "zh": "跟随系统"},
	"theme.light":    {"ko": "라이트", "en": "Light", "ja": "ライト", "zh": "浅色"},
	"theme.dark":     {"ko": "다크", "en": "Dark", "ja": "ダーク", "zh": "深色"},
}
