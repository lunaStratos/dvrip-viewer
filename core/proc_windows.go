//go:build windows

package core

import (
	"os/exec"
	"syscall"
)

// createNoWindow 는 CREATE_NO_WINDOW 프로세스 생성 플래그다.
const createNoWindow = 0x08000000

// hideConsole 은 자식 프로세스(ffmpeg 등 콘솔 프로그램)가 실행될 때
// 새 콘솔(cmd) 창이 뜨지 않게 한다.
// 앱은 -H=windowsgui 로 빌드돼 콘솔이 없으므로, 이 플래그가 없으면
// Windows 가 ffmpeg.exe 용 콘솔 창을 새로 만든다.
func hideConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
