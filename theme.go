package main

import (
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// appTheme 은 기본 테마에 두 가지를 덧씌운다.
//   - 글꼴: 맑은 고딕(시스템에 있으면)
//   - 화면 모드: 사용자가 고른 라이트/다크(시스템 설정을 따르게 할 수도 있음)
type appTheme struct {
	mode string // "" | "light" | "dark"
}

func (t *appTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch t.mode {
	case "light":
		v = theme.VariantLight
	case "dark":
		v = theme.VariantDark
	}
	return theme.DefaultTheme().Color(n, v)
}

func (t *appTheme) Font(s fyne.TextStyle) fyne.Resource {
	// 고정폭·기호 글꼴은 기본 것을 쓴다(맑은 고딕은 고정폭이 아니다).
	if s.Monospace || s.Symbol {
		return theme.DefaultTheme().Font(s)
	}
	reg, bold := malgunFonts()
	if s.Bold && bold != nil {
		return bold
	}
	if reg != nil {
		return reg
	}
	return theme.DefaultTheme().Font(s)
}

func (t *appTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }
func (t *appTheme) Size(n fyne.ThemeSizeName) float32      { return theme.DefaultTheme().Size(n) }

// 맑은 고딕은 Microsoft 글꼴이라 앱에 넣어 배포할 수 없다.
// 대신 실행 시 OS 에 설치된 글꼴 파일을 읽는다. 없으면(대부분의 macOS) nil 을
// 돌려주고, 그러면 Fyne 기본 글꼴(+시스템 글꼴 대체)을 쓴다.
var (
	malgunOnce          sync.Once
	malgunReg, malgunBd fyne.Resource
)

func malgunFonts() (regular, bold fyne.Resource) {
	malgunOnce.Do(func() {
		for _, dir := range fontDirs() {
			if malgunReg == nil {
				malgunReg = readFont(dir, "malgun.ttf")
			}
			if malgunBd == nil {
				malgunBd = readFont(dir, "malgunbd.ttf")
			}
		}
	})
	return malgunReg, malgunBd
}

func fontDirs() []string {
	var dirs []string
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		win := os.Getenv("WINDIR")
		if win == "" {
			win = `C:\Windows`
		}
		dirs = append(dirs, filepath.Join(win, "Fonts"))
		// 사용자 단위로 설치한 글꼴
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			dirs = append(dirs, filepath.Join(la, "Microsoft", "Windows", "Fonts"))
		}
	case "darwin":
		dirs = append(dirs, "/Library/Fonts", filepath.Join(home, "Library", "Fonts"))
	default:
		dirs = append(dirs, "/usr/share/fonts/truetype/malgun", filepath.Join(home, ".local", "share", "fonts"))
	}
	return dirs
}

func readFont(dir, name string) fyne.Resource {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil || len(b) == 0 {
		return nil
	}
	return fyne.NewStaticResource(name, b)
}
