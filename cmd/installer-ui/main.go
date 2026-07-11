// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/laninstall"
)

const maxUpload = 128 << 20

type server struct{ csrf string }

func main() {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	fatal(err)
	tokenBytes := make([]byte, 24)
	_, err = rand.Read(tokenBytes)
	fatal(err)
	s := &server{csrf: hex.EncodeToString(tokenBytes)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.index)
	mux.HandleFunc("POST /install", s.install)
	address := "http://" + listener.Addr().String()
	log.Printf("U60 Pro installer UI: %s", address)
	go openBrowser(address)
	fatal(http.Serve(listener, mux))
}

func (s *server) index(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'")
	_, _ = io.WriteString(w, strings.ReplaceAll(page, "{{CSRF}}", s.csrf))
}

func (s *server) install(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Header.Get("Origin") != "http://"+r.Host {
		http.Error(w, `{"error":"invalid local request"}`, http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+(1<<20))
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if r.FormValue("csrf") != s.csrf {
		http.Error(w, `{"error":"invalid local request"}`, http.StatusForbidden)
		return
	}
	file, _, err := r.FormFile("container")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	defer file.Close()
	container, err := io.ReadAll(io.LimitReader(file, maxUpload+1))
	if err != nil || len(container) > maxUpload {
		writeError(w, http.StatusBadRequest, fmt.Errorf("container is too large"))
		return
	}
	var rollback []byte
	if rollbackFile, _, rollbackErr := r.FormFile("rollback_container"); rollbackErr == nil {
		defer rollbackFile.Close()
		rollback, err = io.ReadAll(io.LimitReader(rollbackFile, maxUpload+1))
		if err != nil || len(rollback) > maxUpload {
			writeError(w, http.StatusBadRequest, fmt.Errorf("rollback container is too large"))
			return
		}
	}
	base := strings.TrimSpace(r.FormValue("base"))
	session := strings.TrimSpace(r.FormValue("session"))
	password := r.FormValue("router_password")
	if session != "" && password != "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("use either a UBUS session or router password, not both"))
		return
	}
	if r.FormValue("execute") == "yes" && session == "" && password == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("execution requires the router password or an authorized UBUS session"))
		return
	}
	client, err := laninstall.NewClient(base, session, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if password != "" && r.FormValue("execute") == "yes" {
		if err := client.Login(r.Context(), password); err != nil {
			writeError(w, http.StatusUnauthorized, err)
			return
		}
	}
	parsed, _ := url.Parse(base)
	health := parsed.Scheme + "://" + net.JoinHostPort(parsed.Hostname(), "2333") + "/healthz"
	backupDir, err := os.UserConfigDir()
	if err != nil {
		backupDir = "."
	}
	backupDir = filepath.Join(backupDir, "UFI-TOOLS-U60-PRO-RE", "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Minute)
	defer cancel()
	result, installErr := client.Install(ctx, laninstall.InstallOptions{
		Container: container, RollbackContainer: rollback, BackupPath: filepath.Join(backupDir, "rollback-"+time.Now().UTC().Format("20060102T150405Z")+".bin"),
		HealthURL: health, Execute: r.FormValue("execute") == "yes", Acknowledged: r.FormValue("ack") == "RESTORE U60 PRO",
	})
	status := http.StatusOK
	if installErr != nil {
		status = http.StatusBadGateway
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"result": result, "error": errorText(installErr)})
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
func fatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
func openBrowser(target string) {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	case "darwin":
		command = exec.Command("open", target)
	default:
		command = exec.Command("xdg-open", target)
	}
	_ = command.Start()
}

const page = `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>U60 Pro 开源安装器</title><style>body{font:16px system-ui;background:#10151d;color:#edf2f7;margin:0}main{max-width:720px;margin:5vh auto;padding:28px;background:#18212d;border-radius:18px}label{display:block;margin:18px 0 7px}input,button{box-sizing:border-box;width:100%;padding:12px;border-radius:9px;border:1px solid #445;background:#0d131b;color:white}button{background:#1976d2;border:0;font-weight:700;margin-top:20px}small{color:#aab7c4}.danger{border:1px solid #b94a48;padding:14px;border-radius:10px}pre{white-space:pre-wrap;background:#090d12;padding:14px;border-radius:9px}</style><main><h1>U60 Pro 开源安装器</h1><p>默认只预览。执行必须提供能恢复当前服务与 rc.local 的已验证回滚容器；候选或回滚未恢复健康都会明确报错。</p><form id="form"><input type="hidden" name="csrf" value="{{CSRF}}"><label>设备管理地址</label><input name="base" value="http://192.168.98.1" required><label>路由器 Web 密码（推荐；不保存）</label><input type="password" name="router_password" autocomplete="current-password"><label>已有 UBUS 会话（与密码二选一）</label><input name="session" maxlength="32" autocomplete="off"><label>候选 backup_restore.bin</label><input type="file" name="container" required><label>匹配当前设备状态的回滚容器（执行时必选）</label><input type="file" name="rollback_container"><label><input type="checkbox" name="execute" value="yes" style="width:auto"> 执行设备恢复</label><div class="danger"><label>执行确认（输入 RESTORE U60 PRO）</label><input name="ack" autocomplete="off"><small>恢复会重启并修改设备。请保持供电和局域网连接。</small></div><button>开始</button></form><pre id="out">等待操作</pre></main><script>form.onsubmit=async e=>{e.preventDefault();out.textContent='处理中，请勿断电…';try{let r=await fetch('/install',{method:'POST',body:new FormData(form)});out.textContent=JSON.stringify(await r.json(),null,2)}catch(e){out.textContent=e.message}}</script></html>`
