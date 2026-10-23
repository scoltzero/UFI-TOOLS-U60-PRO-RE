// SPDX-License-Identifier: AGPL-3.0-or-later
package device

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

func (u *U60UBUS) smsCommand(ctx context.Context, v map[string]string) (map[string]any, error) {
	method := v["func"]
	id := strings.ToUpper(v["goformId"])
	switch id {
	case "SEND_SMS":
		method = "zte_libwms_send_sms"
	case "DELETE_SMS":
		method = "zwrt_wms_delete_sms"
	case "SET_MSG_READ":
		method = "zwrt_wms_modify_tag"
	}
	p := map[string]any{}
	switch method {
	case "zte_libwms_get_sms_data":
		page, err := integer(v["page"], 0, 10000)
		if err != nil {
			return nil, err
		}
		count, err := integer(v["data_per_page"], 1, 500)
		if err != nil {
			return nil, err
		}
		p = map[string]any{"page": page, "data_per_page": count, "mem_store": 1, "tags": 100, "order_by": "order by id desc", "sms_no_encode_flag": "1"}
	case "zte_libwms_send_sms":
		if !u.allowWrites {
			return nil, ErrWriteDisabled
		}
		number := field(v, "Number", "number")
		if number == "" || len(number) > 128 || strings.Trim(number, "+0123456789;, ") != "" {
			return nil, invalid("SMS recipient")
		}
		body := v["MessageText"]
		if body == "" {
			body = v["message_body"]
		}
		if body == "" && v["MessageBody"] != "" {
			data, err := hex.DecodeString(v["MessageBody"])
			if err != nil || len(data)%2 != 0 {
				return nil, invalid("SMS UTF-16 body")
			}
			units := []uint16{}
			for i := 0; i < len(data); i += 2 {
				units = append(units, uint16(data[i])<<8|uint16(data[i+1]))
			}
			body = string(utf16.Decode(units))
		}
		if body == "" || len(body) > 4096 || strings.ContainsRune(body, 0) {
			return nil, invalid("SMS body")
		}
		p = map[string]any{"number": number, "message_body": body, "id": "-1", "encode_type": "UNICODE", "sms_time": time.Now().UTC().Format(time.RFC3339), "sms_no_decode_flag": "1"}
	case "zwrt_wms_delete_sms", "zwrt_wms_modify_tag":
		if !u.allowWrites {
			return nil, ErrWriteDisabled
		}
		msgID := field(v, "msg_id", "id")
		if msgID == "" || strings.Trim(msgID, "0123456789;") != "" {
			return nil, invalid("SMS identifiers")
		}
		p["id"] = msgID
		if method == "zwrt_wms_modify_tag" {
			p["tag"] = 1
		}
	default:
		return nil, ErrUnsupported
	}
	data, err := u.call(ctx, "zwrt_wms", method, p)
	if err != nil {
		return nil, err
	}
	if method == "zte_libwms_get_sms_data" {
		raw := firstValue(data, "messages", "sms", "sms_data", "data")
		if text, ok := raw.(string); ok {
			var decoded any
			if json.Unmarshal([]byte(text), &decoded) != nil {
				return nil, invalid("SMS response")
			}
			raw = decoded
		}
		rows, ok := raw.([]any)
		if !ok {
			return nil, invalid("unrecognized SMS list response")
		}
		messages := []map[string]any{}
		for _, row := range rows {
			item, ok := row.(map[string]any)
			if !ok {
				return nil, invalid("SMS response row")
			}
			messages = append(messages, map[string]any{"id": textValue(firstValue(item, "id", "sms_id")), "number": firstValue(item, "number", "address", "from"), "content_encoding": "text", "content": firstValue(item, "content", "body", "message"), "date": firstValue(item, "date", "time"), "tag": textValue(item["tag"])})
		}
		return map[string]any{"messages": messages, "sms_data_total": len(messages)}, nil
	}
	return map[string]any{"result": "success"}, nil
}
func (s *Simulator) smsCommand(v map[string]string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initControls()
	if s.messages == nil {
		s.messages = []map[string]any{{"id": "1", "number": "10000", "content_encoding": "text", "content": "模拟短信，仅用于本地验证", "date": "2026-10-22 10:00:00", "tag": "0"}}
	}
	id := strings.ToUpper(v["goformId"])
	method := v["func"]
	if id == "ZWRT_WMS" && method == "zte_libwms_get_sms_data" {
		list := []map[string]any{}
		for _, m := range s.messages {
			cp := map[string]any{}
			for k, x := range m {
				cp[k] = x
			}
			list = append(list, cp)
		}
		return map[string]any{"messages": list, "sms_data_total": len(list), "simulated": true}, nil
	}
	if id == "SEND_SMS" || method == "zte_libwms_send_sms" {
		return map[string]any{"result": "success", "simulated": true}, nil
	}
	for i := len(s.messages) - 1; i >= 0; i-- {
		m := s.messages[i]
		if textValue(m["id"]) != field(v, "msg_id", "id") {
			continue
		}
		if id == "DELETE_SMS" || method == "zwrt_wms_delete_sms" {
			s.messages = append(s.messages[:i], s.messages[i+1:]...)
		} else if id == "SET_MSG_READ" || method == "zwrt_wms_modify_tag" {
			m["tag"] = strconv.Itoa(1)
		}
	}
	return map[string]any{"result": "success", "simulated": true}, nil
}
