// XM(DVRIP) 프로토콜 코어: 로그인·실시간 스트림·프레임 디먹스.
// CLI(xmview) 와 데스크톱 앱(xmview-desktop) 이 공통으로 사용한다.
package core

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// ---- DVRIP 메시지 ID ----
const (
	msgLoginReq    = 1000 // 로그인 요청
	msgLoginResp   = 1001
	msgKeepAlive   = 1006
	msgMonitorClaim = 1413 // OPMonitor Claim
	msgMonitorStart = 1410 // OPMonitor Start
	msgMonitorData  = 1412 // 실시간 영상 데이터
)

// DVRIP 20바이트 헤더 + (JSON 또는 바이너리) 본문
type conn struct {
	c       net.Conn
	session uint32
	seq     uint32
}

// XM "Sofia" 비밀번호 해시: MD5(pw) 16바이트를 2바이트씩 더해 62진 문자로 8자리.
func sofiaHash(pw string) string {
	sum := md5.Sum([]byte(pw))
	const tbl = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	out := make([]byte, 8)
	for i := 0; i < 8; i++ {
		out[i] = tbl[(int(sum[2*i])+int(sum[2*i+1]))%62]
	}
	return string(out)
}

// 패킷 전송: 본문은 JSON 문자열 뒤에 0x0a 0x00 을 붙인다(길이에 포함).
func (d *conn) send(msgID uint16, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	body = append(body, 0x0a, 0x00)
	hdr := make([]byte, 20)
	hdr[0] = 0xFF
	binary.LittleEndian.PutUint32(hdr[4:8], d.session)
	binary.LittleEndian.PutUint32(hdr[8:12], d.seq)
	binary.LittleEndian.PutUint16(hdr[14:16], msgID)
	binary.LittleEndian.PutUint32(hdr[16:20], uint32(len(body)))
	d.seq++
	if _, err := d.c.Write(hdr); err != nil {
		return err
	}
	_, err = d.c.Write(body)
	return err
}

// 패킷 1개 수신: (msgID, 본문바이트).
func (d *conn) recv() (uint16, []byte, error) {
	hdr := make([]byte, 20)
	if _, err := io.ReadFull(d.c, hdr); err != nil {
		return 0, nil, err
	}
	if hdr[0] != 0xFF {
		return 0, nil, fmt.Errorf("잘못된 헤더 선두 바이트 0x%02x (XM DVRIP 응답이 아님)", hdr[0])
	}
	msgID := binary.LittleEndian.Uint16(hdr[14:16])
	n := binary.LittleEndian.Uint32(hdr[16:20])
	body := make([]byte, n)
	if _, err := io.ReadFull(d.c, body); err != nil {
		return 0, nil, err
	}
	return msgID, body, nil
}

// JSON 본문을 map 으로 파싱(뒤의 0x00/개행 제거).
func parseJSON(body []byte) (map[string]any, error) {
	body = trimNull(body)
	m := map[string]any{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("JSON 파싱 실패: %w (원문: %q)", err, preview(body))
	}
	return m, nil
}

func trimNull(b []byte) []byte {
	return []byte(strings.TrimRight(string(b), "\x00\n\r "))
}
func preview(b []byte) string {
	s := string(b)
	if len(s) > 120 {
		s = s[:120] + "..."
	}
	return s
}

func (d *conn) login(user, pass string) error {
	req := map[string]any{
		"EncryptType": "MD5",
		"LoginType":   "DVRIP-Web",
		"UserName":    user,
		"PassWord":    sofiaHash(pass),
	}
	if err := d.send(msgLoginReq, req); err != nil {
		return err
	}
	// 장비가 응답하지 않으면 무한 대기하지 않도록 로그인 응답에 타임아웃을 건다.
	d.c.SetReadDeadline(time.Now().Add(8 * time.Second))
	_, body, err := d.recv()
	d.c.SetReadDeadline(time.Time{}) // 해제
	if err != nil {
		return fmt.Errorf("로그인 응답 없음(장비 무응답/과도접속 차단 가능): %w", err)
	}
	m, err := parseJSON(body)
	if err != nil {
		return err
	}
	ret, _ := toInt(m["Ret"])
	if ret != 100 {
		return fmt.Errorf("로그인 거부 (Ret=%v) — 아이디/비밀번호 또는 암호화 방식 확인 필요", m["Ret"])
	}
	// SessionID 는 "0x0000000c" 형태의 16진 문자열
	sidStr, _ := m["SessionID"].(string)
	sid, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(sidStr), "0x"), 16, 32)
	if err != nil {
		return fmt.Errorf("SessionID 파싱 실패: %v", sidStr)
	}
	d.session = uint32(sid)
	alive, _ := toInt(m["AliveInterval"])
	if alive <= 0 {
		alive = 20
	}
	fmt.Fprintf(os.Stderr, "[xmview] 로그인 성공 (SessionID=0x%08x, KeepAlive=%ds)\n", d.session, alive)
	go d.keepAliveLoop(time.Duration(alive) * time.Second)
	return nil
}

func (d *conn) keepAliveLoop(every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for range t.C {
		_ = d.send(msgKeepAlive, map[string]any{
			"Name":      "KeepAlive",
			"SessionID": fmt.Sprintf("0x%08X", d.session),
		})
	}
}

// 실시간 모니터 시작. stream: "Main"(주) 또는 "Extra1"(부).
func (d *conn) monitorStart(channel int, stream string) error {
	params := map[string]any{
		"Channel":    channel,
		"CombinMode": "NONE",
		"StreamType": stream,
		"TransMode":  "TCP",
		"Value":      0,
	}
	claim := map[string]any{
		"Name":      "OPMonitor",
		"SessionID": fmt.Sprintf("0x%08X", d.session),
		"OPMonitor": map[string]any{"Action": "Claim", "Parameter": params},
	}
	if err := d.send(msgMonitorClaim, claim); err != nil {
		return err
	}
	if _, body, err := d.recv(); err == nil {
		if m, e := parseJSON(body); e == nil {
			if ret, _ := toInt(m["Ret"]); ret != 100 && ret != 0 {
				fmt.Fprintf(os.Stderr, "[xmview] Claim 응답 Ret=%v (계속 진행)\n", m["Ret"])
			}
		}
	}
	start := map[string]any{
		"Name":      "OPMonitor",
		"SessionID": fmt.Sprintf("0x%08X", d.session),
		"OPMonitor": map[string]any{"Action": "Start", "Parameter": params},
	}
	return d.send(msgMonitorStart, start)
}

// XM 프레임 파서: 1412 패킷 본문들을 이어붙여 00 00 01 Fx 프레임을 떼고
// 비디오 payload(H.264/H.265 Annex-B)만 out 으로 흘려보낸다.
type demuxer struct {
	buf []byte
	out io.Writer
}

func (x *demuxer) feed(p []byte) error {
	x.buf = append(x.buf, p...)
	for {
		// 다음 마커(00 00 01)를 찾는다
		if len(x.buf) < 8 {
			return nil
		}
		if !(x.buf[0] == 0 && x.buf[1] == 0 && x.buf[2] == 1) {
			// 동기 어긋남: 다음 마커까지 버린다
			idx := findMarker(x.buf, 1)
			if idx < 0 {
				x.buf = x.buf[len(x.buf)-2:] // 꼬리 보존
				return nil
			}
			x.buf = x.buf[idx:]
			continue
		}
		kind := x.buf[3]
		var hdrLen int
		var dataLen int
		switch kind {
		case 0xFC, 0xFE: // 비디오 I-프레임(확장 헤더 16B)
			if len(x.buf) < 16 {
				return nil
			}
			hdrLen = 16
			dataLen = int(binary.LittleEndian.Uint32(x.buf[12:16]))
		case 0xFD: // 비디오 P/B-프레임(헤더 8B)
			if len(x.buf) < 8 {
				return nil
			}
			hdrLen = 8
			dataLen = int(binary.LittleEndian.Uint32(x.buf[4:8]))
		case 0xFA, 0xF9: // 오디오/기타(헤더 8B, 길이 uint16)
			if len(x.buf) < 8 {
				return nil
			}
			hdrLen = 8
			dataLen = int(binary.LittleEndian.Uint16(x.buf[6:8]))
		default:
			// 알 수 없는 마커: 1바이트 밀고 재동기
			idx := findMarker(x.buf, 1)
			if idx < 0 {
				x.buf = x.buf[len(x.buf)-2:]
				return nil
			}
			x.buf = x.buf[idx:]
			continue
		}
		if len(x.buf) < hdrLen+dataLen {
			return nil // 더 받아야 함
		}
		payload := x.buf[hdrLen : hdrLen+dataLen]
		if kind == 0xFC || kind == 0xFE || kind == 0xFD {
			if _, err := x.out.Write(payload); err != nil {
				return err
			}
		}
		x.buf = x.buf[hdrLen+dataLen:]
	}
}

// buf 안에서 start 위치 이후의 00 00 01 마커 인덱스를 찾는다.
func findMarker(buf []byte, start int) int {
	for i := start; i+3 < len(buf); i++ {
		if buf[i] == 0 && buf[i+1] == 0 && buf[i+2] == 1 {
			return i
		}
	}
	return -1
}

// 한 채널에 접속·로그인·모니터 시작 후 비디오 Annex-B 를 out 으로 흘려보낸다.
// 블로킹 함수이며, 연결이 끊기면 반환한다.
func StreamChannel(host string, port int, user, pass string, channel int, stream string, out io.Writer, onReady func()) error {
	hostport := net.JoinHostPort(host, strconv.Itoa(port))
	tcp, err := net.DialTimeout("tcp", hostport, 10*time.Second)
	if err != nil {
		return fmt.Errorf("접속 실패: %w", err)
	}
	defer tcp.Close()
	d := &conn{c: tcp}
	if err := d.login(user, pass); err != nil {
		return err
	}
	if err := d.monitorStart(channel, stream); err != nil {
		return err
	}
	if onReady != nil {
		onReady()
	}
	dx := &demuxer{out: out}
	gotData := false
	for {
		// 스트림 시작 후 10초 안에 영상이 안 오면(슬롯 소진 등) 무한 대기하지 않고 종료.
		if !gotData {
			d.c.SetReadDeadline(time.Now().Add(10 * time.Second))
		}
		msgID, body, err := d.recv()
		if err != nil {
			if !gotData {
				return fmt.Errorf("영상 수신 없음(장비 스트림 슬롯 소진/권한 가능): %w", err)
			}
			return err
		}
		if msgID == msgMonitorData {
			if !gotData {
				gotData = true
				d.c.SetReadDeadline(time.Time{}) // 데이터가 오기 시작하면 데드라인 해제
			}
			if err := dx.feed(body); err != nil {
				return err
			}
		}
	}
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case string:
		i, err := strconv.Atoi(n)
		return i, err == nil
	}
	return 0, false
}
