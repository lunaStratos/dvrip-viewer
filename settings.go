package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"xmview/core"
)

// settings 는 사용자가 환경설정에서 고르는 값이다.
// 접속정보와 같은 앱 데이터 폴더의 settings.json 에 저장한다.
type settings struct {
	// Lang: "" = 시스템 언어를 따름, 그 외 "ko" | "en" | "ja" | "zh"
	Lang string `json:"lang"`
	// Theme: "" = 시스템 설정을 따름, 그 외 "light" | "dark"
	Theme string `json:"theme"`
}

func settingsPath() string { return filepath.Join(core.DataDir(), "settings.json") }

func loadSettings() settings {
	var s settings
	if b, err := os.ReadFile(settingsPath()); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

func saveSettings(s settings) {
	b, _ := json.MarshalIndent(s, "", "  ")
	_ = os.WriteFile(settingsPath(), b, 0o600)
}
