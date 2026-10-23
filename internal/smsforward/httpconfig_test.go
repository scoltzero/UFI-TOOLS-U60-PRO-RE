package smsforward

import (
	"encoding/json"
	"testing"
)

func TestCurlLiteralConfigAndQuotedJSON(t *testing.T) {
	v, err := ParseHTTPConfig(`curl -X POST 'http://127.0.0.1:12345/hook' -H 'Content-Type: application/json' --data '{"body":"{{sms-body}}"}'`)
	if err != nil || v["url"] != "http://127.0.0.1:12345/hook" {
		t.Fatalf("%v %v", v, err)
	}
	for _, bad := range []string{`curl --data @/etc/passwd http://127.0.0.1/hook`, `curl --location http://127.0.0.1/hook`, `curl 'unterminated`} {
		if _, err := ParseHTTPConfig(bad); err == nil {
			t.Fatal("unsupported curl input accepted")
		}
	}
	body := replace(v["body"].(string), Message{Body: "quoted \"body\"\nnew line"})
	if !json.Valid([]byte(body)) {
		t.Fatalf("replacement broke JSON: %s", body)
	}
}
