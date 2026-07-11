// SPDX-License-Identifier: AGPL-3.0-or-later

package laninstall

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientBackupUploadRestore(t *testing.T) {
	t.Helper()
	var procedures []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ubus/":
			var calls []struct {
				Params []any `json:"params"`
			}
			if err := json.NewDecoder(r.Body).Decode(&calls); err != nil || len(calls) != 1 || len(calls[0].Params) != 4 {
				t.Fatalf("invalid RPC body: %v", err)
			}
			if calls[0].Params[0] != defaultSession || calls[0].Params[1] != "zwrt_mc.device.manager" {
				t.Fatalf("unexpected RPC target: %#v", calls[0].Params)
			}
			params, _ := calls[0].Params[3].(map[string]any)
			if params["procType"] != webProcedureSource {
				t.Fatalf("procType = %#v", params["procType"])
			}
			procedures = append(procedures, calls[0].Params[2].(string))
			io.WriteString(w, `[{"jsonrpc":"2.0","id":1,"result":[0,{"result":"success"}]}]`)
		case "/backup/back_parameter":
			io.WriteString(w, "original")
		case "/cgi-bin/cgi-upload":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			if got := r.FormValue("filename"); got != "/tmp/back_parameter" {
				t.Fatalf("filename = %q", got)
			}
			file, header, err := r.FormFile("filedata")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			data, _ := io.ReadAll(file)
			if header.Filename != "backup_restore.bin" || string(data) != "candidate" {
				t.Fatalf("unexpected upload %q %q", header.Filename, data)
			}
			io.WriteString(w, "checksum=ok sha256sum=ok")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	backup, err := client.Backup(context.Background())
	if err != nil || string(backup) != "original" {
		t.Fatalf("Backup() = %q, %v", backup, err)
	}
	if err := client.Upload(context.Background(), []byte("candidate")); err != nil {
		t.Fatal(err)
	}
	if err := client.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(procedures) != 2 || procedures[0] != "device_backup_proc" || procedures[1] != "device_restore_proc" {
		t.Fatalf("procedures = %#v", procedures)
	}
}

func TestNewClientRejectsPublicTarget(t *testing.T) {
	if _, err := NewClient("https://example.com", "", nil); err == nil {
		t.Fatal("expected public target rejection")
	}
}

func TestLoginHashesPasswordAndAuthenticatesUpload(t *testing.T) {
	const session = "0123456789abcdef0123456789abcdef"
	var loginCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ubus/":
			var calls []struct {
				Params []json.RawMessage `json:"params"`
			}
			if err := json.NewDecoder(r.Body).Decode(&calls); err != nil || len(calls) != 1 || len(calls[0].Params) != 4 {
				t.Fatalf("invalid login RPC: %v", err)
			}
			var procedure string
			if err := json.Unmarshal(calls[0].Params[2], &procedure); err != nil {
				t.Fatal(err)
			}
			switch procedure {
			case "web_login_info":
				loginCalls++
				io.WriteString(w, `[{"result":[0,{"zte_web_sault":"SALT123"}]}]`)
			case "web_login":
				loginCalls++
				var arguments map[string]string
				if err := json.Unmarshal(calls[0].Params[3], &arguments); err != nil {
					t.Fatal(err)
				}
				if arguments["password"] != "F855CD3E3B093D1382B386AF79DD06970492422EBB53D7B1B0EB243883B09A6A" {
					t.Fatalf("password digest = %q", arguments["password"])
				}
				io.WriteString(w, `[{"result":[0,{"result":"0","ubus_rpc_session":"`+session+`"}]}]`)
			default:
				t.Fatalf("unexpected procedure %q", procedure)
			}
		case "/cgi-bin/cgi-upload":
			cookie, err := r.Cookie("webtoken")
			if err != nil || cookie.Value != session {
				t.Fatalf("webtoken cookie = %#v, %v", cookie, err)
			}
			io.WriteString(w, "checksum sha256sum")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Login(context.Background(), "router-secret"); err != nil {
		t.Fatal(err)
	}
	if err := client.Upload(context.Background(), []byte("candidate")); err != nil {
		t.Fatal(err)
	}
	if loginCalls != 2 {
		t.Fatalf("login calls = %d", loginCalls)
	}
}
