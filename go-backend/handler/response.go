package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// ── 统一响应 ──

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func okResponse(w http.ResponseWriter, data interface{}) {
	writeJSON(w, map[string]interface{}{"ok": true, "data": data})
}

func errResponse(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": msg})
}

// ── 类型转换辅助 ──

func intFloat(v interface{}) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		n, _ := strconv.Atoi(t)
		return n
	}
	return 0
}

func intFloatStr(v interface{}) string {
	switch t := v.(type) {
	case float64:
		return strconv.Itoa(int(t))
	case int:
		return strconv.Itoa(t)
	case string:
		return t
	}
	return ""
}

func strVal(v interface{}) string {
	s, _ := v.(string)
	return s
}
