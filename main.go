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
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
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

func dataDir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		return "."
	}
	d := filepath.Join(base, "xmview")
	_ = os.MkdirAll(d, 0755)
	return d
}
func profilePath() string { return filepath.Join(dataDir(), "connections.json") }

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

func main() {
	core.ResolveTools()

	a := app.New()
	icon := fyne.NewStaticResource("icon.png", iconPNG)
	a.SetIcon(icon)
	w := a.NewWindow("XMView — CCTV 뷰어")
	w.SetIcon(icon)
	w.Resize(fyne.NewSize(1180, 780))

	// ---- 입력 폼 ----
	name := widget.NewEntry()
	name.SetPlaceHolder("이름(저장용)")
	host := widget.NewEntry()
	host.SetPlaceHolder("IP 또는 도메인")
	port := widget.NewEntry()
	port.SetText("34567")
	user := widget.NewEntry()
	user.SetText("admin")
	pass := widget.NewPasswordEntry()
	pass.SetPlaceHolder("비밀번호")
	channels := widget.NewEntry()
	channels.SetText("0,1,2,3")
	streamSel := widget.NewSelect([]string{"Main(고화질)", "Extra1(저지연)"}, nil)
	streamSel.SetSelected("Main(고화질)")
	status := widget.NewLabel("대기 중")

	fillForm := func(p profile) {
		name.SetText(p.Name)
		host.SetText(p.Host)
		port.SetText(strconv.Itoa(p.Port))
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
				fillForm(p)
				return
			}
		}
	})
	savedSel.PlaceHolder = "저장된 접속 선택"

	// ---- 영상 그리드 ----
	grid := container.NewGridWithColumns(2)
	var stopMu sync.Mutex
	var stopCh chan struct{}

	streamType := func() string {
		if strings.HasPrefix(streamSel.Selected, "Extra1") {
			return "Extra1"
		}
		return "Main"
	}

	// 좌측 설정 패널(접기 가능)
	form := widget.NewForm(
		widget.NewFormItem("이름", name),
		widget.NewFormItem("주소", host),
		widget.NewFormItem("포트", port),
		widget.NewFormItem("아이디", user),
		widget.NewFormItem("비밀번호", pass),
		widget.NewFormItem("채널", channels),
		widget.NewFormItem("스트림", streamSel),
		widget.NewFormItem("저장목록", savedSel),
	)

	var leftPanel *fyne.Container
	var toggleBtn *widget.Button
	setPanel := func(show bool) {
		if show {
			leftPanel.Show()
			toggleBtn.SetText("◀ 설정 숨기기")
		} else {
			leftPanel.Hide()
			toggleBtn.SetText("▶ 설정")
		}
	}
	toggleBtn = widget.NewButtonWithIcon("◀ 설정 숨기기", theme.MenuIcon(), func() {
		setPanel(!leftPanel.Visible())
	})

	connect := func() {
		p := curProfile()
		if p.Host == "" {
			status.SetText("주소를 입력하세요")
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
				status.SetText("채널 목록 오류: " + s)
				return
			}
			chans = append(chans, n)
		}
		if len(chans) == 0 {
			status.SetText("채널을 입력하세요")
			return
		}

		// 이전 세션 중단
		stopMu.Lock()
		if stopCh != nil {
			close(stopCh)
		}
		stopCh = make(chan struct{})
		myStop := stopCh
		stopMu.Unlock()

		cols := 2
		if len(chans) <= 1 {
			cols = 1
		}
		grid.Layout = container.NewGridWithColumns(cols).Layout
		grid.Objects = nil
		st := streamType()
		for _, ch := range chans {
			img := canvas.NewImageFromImage(nil)
			img.FillMode = canvas.ImageFillContain
			img.SetMinSize(fyne.NewSize(420, 240))
			lbl := canvas.NewText(fmt.Sprintf(" CH %d ", ch), theme.Color(theme.ColorNameForeground))
			lbl.TextSize = 12
			tile := container.NewStack(img, container.NewVBox(lbl, layoutSpacer()))
			grid.Add(tile)
			go runTile(p, ch, st, img, myStop)
		}
		grid.Refresh()

		// 연결하면 좌측 바 자동 축소
		setPanel(false)
		status.SetText(fmt.Sprintf("%d개 채널 · %s · GPU 디코드", len(chans), st))
	}

	saveBtn := widget.NewButton("저장", func() {
		p := curProfile()
		if p.Name == "" || p.Host == "" {
			status.SetText("이름과 주소를 입력하세요")
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
		status.SetText("저장됨: " + p.Name)
	})
	connectBtn := widget.NewButton("연결", connect)
	connectBtn.Importance = widget.HighImportance

	leftPanel = container.NewVBox(form, container.NewHBox(connectBtn, saveBtn))
	topBar := container.NewHBox(toggleBtn, status)

	refreshSaved()
	if ps := loadProfiles(); len(ps) > 0 {
		fillForm(ps[0])
		savedSel.SetSelected(ps[0].Name)
	}

	content := container.NewBorder(topBar, nil, leftPanel, nil, grid)
	w.SetContent(content)
	w.ShowAndRun()
}

func layoutSpacer() fyne.CanvasObject { return canvas.NewRectangle(nil) }

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
		m, _, derr := image.Decode(bytes.NewReader(jpg))
		if derr != nil {
			return
		}
		busy.Store(true)
		fyne.Do(func() {
			img.Image = m
			img.Refresh()
			busy.Store(false)
		})
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ch%d: %v\n", ch, err)
	}
}
