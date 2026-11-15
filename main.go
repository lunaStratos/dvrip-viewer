// xmview-native — XM(DVRIP) CCTV 뷰어 (완전 네이티브 GUI)
//
// 웹/HTML/webview/내장서버 없이 Fyne 네이티브 위젯으로 구성.
// 로그인 폼 → 저장 → 2×2 분할 영상(네이티브 OpenGL 렌더).
// HEVC 디코딩은 ffmpeg 하드웨어 가속(VideoToolbox/D3D11VA)으로 GPU 에서 수행.
package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"xmview/core"
)

//go:embed icon.png
var iconPNG []byte

type profile struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Pass     string `json:"pass"`
	Channels string `json:"channels"`
}

// 접속정보는 앱 데이터 폴더에 둔다(core.DataDir 과 같은 위치를 쓴다).
func profilePath() string { return filepath.Join(core.DataDir(), "connections.json") }

func loadProfiles() []profile {
	b, err := os.ReadFile(profilePath())
	if err != nil {
		return nil
	}
	var ps []profile
	json.Unmarshal(b, &ps)
	return ps
}
func saveProfiles(ps []profile) {
	b, _ := json.MarshalIndent(ps, "", "  ")
	os.WriteFile(profilePath(), b, 0600)
}

// viewer 는 창 하나의 상태다.
//
// 언어를 바꾸면 컨트롤(폼·버튼·상태줄)만 새로 만든다(build). 영상 그리드와
// 실행 중인 스트림은 그대로 두어 언어를 바꿔도 영상이 끊기지 않는다.
// 그래서 입력값·패널 상태·상태 메시지는 위젯이 아니라 여기에 보관한다.
type viewer struct {
	a   fyne.App
	w   fyne.Window
	cfg settings

	grid   *fyne.Container
	stopMu sync.Mutex
	stopCh chan struct{}

	// build 사이에 유지하는 값
	form        profile
	streamExtra bool
	panelHidden bool
	savedName   string
	statusMsg   msg

	// 현재 build 가 만든 것
	status       *widget.Label
	captureForm  func()
	startStreams func()
}

func main() {
	core.ResolveTools()

	cfg := loadSettings()
	setLanguage(cfg.Lang)

	a := app.New()
	a.Settings().SetTheme(&appTheme{mode: cfg.Theme})
	icon := fyne.NewStaticResource("icon.png", iconPNG)
	a.SetIcon(icon)
	w := a.NewWindow(T("app.title"))
	w.SetIcon(icon)
	w.Resize(fyne.NewSize(1180, 780))

	v := &viewer{
		a: a, w: w, cfg: cfg,
		grid: container.NewGridWithColumns(2),
		form: profile{Port: 34567, User: "admin", Channels: "0,1,2,3"},
	}
	if ps := loadProfiles(); len(ps) > 0 {
		v.form = ps[0]
		v.savedName = ps[0].Name
	}

	// ffmpeg 가 없으면 연결을 눌렀을 때 자동 설치를 제안하므로, 미리 알려만 둔다.
	switch {
	case core.HaveFFmpeg():
		v.statusMsg = m("status.idle")
	case core.FFmpegInstallable():
		v.statusMsg = m("status.idleAutoInstall")
	default:
		v.statusMsg = m("status.idleNeedFF", manualHint())
	}

	v.build()
	w.ShowAndRun()
}

// setStatus 는 상태줄을 바꾼다. 언어를 바꿔도 다시 번역되도록 키로 보관한다.
func (v *viewer) setStatus(key string, args ...any) {
	v.statusMsg = m(key, args...)
	if v.status != nil {
		v.status.SetText(T(key, args...))
	}
}

// build 는 현재 언어로 컨트롤을 만들어 창에 올린다.
func (v *viewer) build() {
	if v.captureForm != nil {
		v.captureForm() // 이전 화면에서 입력 중이던 값 보존
	}
	v.w.SetTitle(T("app.title"))

	// ---- 입력 폼 ----
	name := widget.NewEntry()
	name.SetPlaceHolder(T("ph.name"))
	host := widget.NewEntry()
	host.SetPlaceHolder(T("ph.host"))
	port := widget.NewEntry()
	user := widget.NewEntry()
	pass := widget.NewPasswordEntry()
	pass.SetPlaceHolder(T("ph.pass"))
	channels := widget.NewEntry()
	streamOpts := []string{T("stream.main"), T("stream.extra")}
	streamSel := widget.NewSelect(streamOpts, nil)
	if v.streamExtra {
		streamSel.SetSelectedIndex(1)
	} else {
		streamSel.SetSelectedIndex(0)
	}
	v.status = widget.NewLabel(T(v.statusMsg.key, v.statusMsg.args...))

	fillForm := func(p profile) {
		name.SetText(p.Name)
		host.SetText(p.Host)
		port.SetText("")
		if p.Port != 0 {
			port.SetText(strconv.Itoa(p.Port))
		}
		user.SetText(p.User)
		pass.SetText(p.Pass)
		channels.SetText(p.Channels)
	}
	curProfile := func() profile {
		pt, _ := strconv.Atoi(strings.TrimSpace(port.Text))
		if pt == 0 {
			pt = 34567
		}
		return profile{
			Name: strings.TrimSpace(name.Text), Host: strings.TrimSpace(host.Text),
			Port: pt, User: strings.TrimSpace(user.Text), Pass: pass.Text,
			Channels: strings.TrimSpace(channels.Text),
		}
	}
	fillForm(v.form)

	var savedSel *widget.Select
	refreshSaved := func() {
		ps := loadProfiles()
		names := make([]string, len(ps))
		for i, p := range ps {
			names[i] = p.Name
		}
		savedSel.Options = names
		savedSel.Refresh()
	}
	savedSel = widget.NewSelect(nil, func(sel string) {
		for _, p := range loadProfiles() {
			if p.Name == sel {
				v.savedName = sel
				fillForm(p)
				return
			}
		}
	})
	savedSel.PlaceHolder = T("ph.saved")
	refreshSaved()
	// SetSelected 는 콜백을 불러 입력 중이던 값을 덮어쓰므로 직접 지정한다.
	savedSel.Selected = v.savedName
	savedSel.Refresh()

	v.captureForm = func() {
		v.form = curProfile()
		v.form.Port, _ = strconv.Atoi(strings.TrimSpace(port.Text)) // 빈 값은 빈 값으로 보존
		v.streamExtra = streamSel.SelectedIndex() == 1
	}

	// 좌측 설정 패널(접기 가능)
	form := widget.NewForm(
		widget.NewFormItem(T("field.name"), name),
		widget.NewFormItem(T("field.host"), host),
		widget.NewFormItem(T("field.port"), port),
		widget.NewFormItem(T("field.user"), user),
		widget.NewFormItem(T("field.pass"), pass),
		widget.NewFormItem(T("field.channels"), channels),
		widget.NewFormItem(T("field.stream"), streamSel),
		widget.NewFormItem(T("field.saved"), savedSel),
	)

	var leftPanel *fyne.Container
	var toggleBtn *widget.Button
	setPanel := func(show bool) {
		v.panelHidden = !show
		if show {
			leftPanel.Show()
			toggleBtn.SetText(T("btn.panelHide"))
		} else {
			leftPanel.Hide()
			toggleBtn.SetText(T("btn.panelShow"))
		}
	}
	toggleBtn = widget.NewButtonWithIcon("", theme.MenuIcon(), func() {
		setPanel(!leftPanel.Visible())
	})
	prefsBtn := widget.NewButtonWithIcon(T("btn.prefs"), theme.SettingsIcon(), v.showPrefs)

	v.startStreams = func() {
		p := curProfile()
		if p.Host == "" {
			v.setStatus("status.needHost")
			return
		}
		var chans []int
		for _, s := range strings.Split(p.Channels, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			n, err := strconv.Atoi(s)
			if err != nil {
				v.setStatus("status.badChannel", s)
				return
			}
			chans = append(chans, n)
		}
		if len(chans) == 0 {
			v.setStatus("status.needChannel")
			return
		}

		// 이전 세션 중단
		v.stopMu.Lock()
		if v.stopCh != nil {
			close(v.stopCh)
		}
		v.stopCh = make(chan struct{})
		myStop := v.stopCh
		v.stopMu.Unlock()

		cols := 2
		if len(chans) <= 1 {
			cols = 1
		}
		v.grid.Layout = container.NewGridWithColumns(cols).Layout
		v.grid.Objects = nil
		st := "Main"
		if streamSel.SelectedIndex() == 1 {
			st = "Extra1"
		}
		for _, ch := range chans {
			img := canvas.NewImageFromImage(nil)
			img.FillMode = canvas.ImageFillContain
			img.SetMinSize(fyne.NewSize(420, 240))
			lbl := canvas.NewText(fmt.Sprintf(" CH %d ", ch), theme.Color(theme.ColorNameForeground))
			lbl.TextSize = 12
			tile := container.NewStack(img, container.NewVBox(lbl, layoutSpacer()))
			v.grid.Add(tile)
			go runTile(p, ch, st, img, myStop)
		}
		v.grid.Refresh()

		// 연결하면 좌측 바 자동 축소
		setPanel(false)
		v.setStatus("status.streaming", len(chans), st)
	}

	connect := func() {
		if strings.TrimSpace(host.Text) == "" {
			v.setStatus("status.needHost")
			return
		}
		// ffmpeg 없이 연결하면 화면이 영원히 비어 있으므로 먼저 확보한다.
		// 설치 중에 언어를 바꿔 화면이 다시 만들어져도 최신 startStreams 를 부른다.
		v.ensureFFmpeg(func() { v.startStreams() })
	}

	saveBtn := widget.NewButton(T("btn.save"), func() {
		p := curProfile()
		if p.Name == "" || p.Host == "" {
			v.setStatus("status.needNameHost")
			return
		}
		ps := loadProfiles()
		found := false
		for i := range ps {
			if ps[i].Name == p.Name {
				ps[i] = p
				found = true
			}
		}
		if !found {
			ps = append(ps, p)
		}
		sort.Slice(ps, func(i, j int) bool { return ps[i].Name < ps[j].Name })
		saveProfiles(ps)
		refreshSaved()
		v.savedName = p.Name
		savedSel.Selected = p.Name
		savedSel.Refresh()
		v.setStatus("status.saved", p.Name)
	})
	connectBtn := widget.NewButton(T("btn.connect"), connect)
	connectBtn.Importance = widget.HighImportance

	leftPanel = container.NewVBox(form, container.NewHBox(connectBtn, saveBtn))
	setPanel(!v.panelHidden)
	topBar := container.NewBorder(nil, nil, toggleBtn, prefsBtn, v.status)

	v.w.SetContent(container.NewBorder(topBar, nil, leftPanel, nil, v.grid))
}

// showPrefs 는 언어·화면 모드 설정 창을 띄운다. 바꾸면 즉시 적용·저장된다.
func (v *viewer) showPrefs() {
	var dlg dialog.Dialog

	// 언어: 0 = 시스템, 1.. = languages
	langOpts := []string{T("prefs.system")}
	langIdx := 0
	for i, l := range languages {
		langOpts = append(langOpts, l.name)
		if l.code == v.cfg.Lang {
			langIdx = i + 1
		}
	}
	langSel := widget.NewSelect(langOpts, nil)
	langSel.SetSelectedIndex(langIdx)
	langSel.OnChanged = func(string) {
		code := ""
		if i := langSel.SelectedIndex(); i > 0 {
			code = languages[i-1].code
		}
		if code == v.cfg.Lang {
			return
		}
		v.cfg.Lang = code
		saveSettings(v.cfg)
		setLanguage(code)
		dlg.Hide()
		v.build()
		v.showPrefs() // 새 언어로 다시 띄운다
	}

	// 화면 모드
	themeCodes := []string{"", "light", "dark"}
	themeOpts := []string{T("prefs.system"), T("theme.light"), T("theme.dark")}
	radio := widget.NewRadioGroup(themeOpts, nil)
	radio.Required = true
	for i, c := range themeCodes {
		if c == v.cfg.Theme {
			radio.SetSelected(themeOpts[i])
		}
	}
	radio.OnChanged = func(sel string) {
		for i, o := range themeOpts {
			if o == sel && themeCodes[i] != v.cfg.Theme {
				v.cfg.Theme = themeCodes[i]
				saveSettings(v.cfg)
				v.a.Settings().SetTheme(&appTheme{mode: v.cfg.Theme})
			}
		}
	}

	content := widget.NewForm(
		widget.NewFormItem(T("prefs.language"), langSel),
		widget.NewFormItem(T("prefs.theme"), radio),
	)
	dlg = dialog.NewCustom(T("prefs.title"), T("btn.close"), content, v.w)
	dlg.Show()
}

func layoutSpacer() fyne.CanvasObject { return canvas.NewRectangle(nil) }

// manualHint 는 자동 설치가 불가능할 때 보여줄 수동 설치 안내다(번역 대기 상태).
func manualHint() msg {
	switch runtime.GOOS {
	case "darwin":
		return m("hint.mac")
	case "linux":
		return m("hint.linux")
	default:
		return m("hint.other")
	}
}

// showMessage 는 확인 버튼 하나짜리 안내 창이다.
// Fyne 기본 대화상자는 한국어·일본어 버튼 문구가 없어서 직접 만든다.
func (v *viewer) showMessage(title, body string) {
	dialog.NewCustom(title, T("btn.ok"), widget.NewLabel(body), v.w).Show()
}

// ensureFFmpeg 는 ffmpeg 가 준비됐는지 확인하고, 없으면 자동 설치를 제안한다.
// 준비가 끝나면 onReady 를 호출한다. 준비되지 않으면 호출하지 않는다.
//
// ffmpeg 는 라이선스 때문에 동봉할 수 없어 처음 한 번만 내려받는다.
// 인터넷에서 실행파일을 받아오는 일이므로 사용자에게 출처와 크기를 보여주고
// 동의를 받은 뒤에만 진행한다(자동으로 몰래 받지 않는다).
func (v *viewer) ensureFFmpeg(onReady func()) {
	if core.HaveFFmpeg() {
		onReady()
		return
	}

	if !core.FFmpegInstallable() {
		v.showMessage(T("ff.needTitle"), T("ff.needBody", manualHint()))
		v.setStatus("status.noFF", manualHint())
		return
	}

	origin, size, _ := core.FFmpegInstallInfo()
	body := widget.NewLabel(T("ff.installBody", origin, core.HumanSize(size)))
	dialog.NewCustomConfirm(T("ff.installTitle"), T("btn.install"), T("btn.cancel"), body,
		func(ok bool) {
			if !ok {
				v.setStatus("status.declined")
				return
			}
			v.runFFmpegInstall(onReady)
		}, v.w).Show()
}

// runFFmpegInstall 은 진행률 창을 띄우고 백그라운드에서 설치한다.
func (v *viewer) runFFmpegInstall(onReady func()) {
	bar := widget.NewProgressBar()
	phase := widget.NewLabel(T("ff.starting"))
	prog := dialog.NewCustomWithoutButtons(T("ff.progressTitle"),
		container.NewVBox(phase, bar), v.w)
	prog.Show()

	go func() {
		_, err := core.InstallFFmpeg(func(ph string, done, total int64) {
			// UI 갱신은 메인 스레드에서만 해야 한다.
			fyne.Do(func() {
				phase.SetText(fmt.Sprintf("%s  %s / %s",
					T("phase."+ph), core.HumanSize(done), core.HumanSize(total)))
				if total > 0 {
					bar.SetValue(float64(done) / float64(total))
				}
			})
		})

		fyne.Do(func() {
			prog.Hide()
			if err != nil {
				// 세부 오류(err)는 core 가 만든 기술 메시지라 번역하지 않는다.
				v.showMessage(T("ff.failTitle"), T("ff.failBody", err.Error(), manualHint()))
				v.setStatus("ff.failTitle")
				return
			}
			// 설치된 경로를 쓰도록 도구 경로를 다시 해석한다.
			core.ResolveTools()
			v.setStatus("status.installed")
			onReady()
		})
	}()
}

// runTile: 한 채널의 프레임을 받아 네이티브 이미지로 그린다.
// Fyne 는 UI 갱신을 메인 스레드에서 해야 하므로 fyne.Do 로 전달한다.
// UI 가 밀리면 프레임을 건너뛴다(라이브 영상이므로 최신 프레임 우선).
func runTile(p profile, ch int, st string, img *canvas.Image, stop <-chan struct{}) {
	var busy atomic.Bool
	err := core.StreamJPEG(stop, p.Host, p.Port, p.User, p.Pass, ch, st, func(jpg []byte) {
		select {
		case <-stop:
			return
		default:
		}
		if busy.Load() {
			return // 이전 프레임 아직 그리는 중 → 건너뜀
		}
		frame, _, derr := image.Decode(bytes.NewReader(jpg))
		if derr != nil {
			return
		}
		busy.Store(true)
		fyne.Do(func() {
			img.Image = frame
			img.Refresh()
			busy.Store(false)
		})
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ch%d: %v\n", ch, err)
	}
}
