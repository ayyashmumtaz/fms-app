package handlers

import (
	"html/template"
	"strings"
	"time"
)

func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"upper": strings.ToUpper,
		"formatDate": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("2006-01-02")
		},
		"dict": func(values ...any) map[string]any {
			m := make(map[string]any)
			for i := 0; i+1 < len(values); i += 2 {
				key, _ := values[i].(string)
				m[key] = values[i+1]
			}
			return m
		},
		"lookupSensor": func(m map[string]bool, key string) bool {
			if val, ok := m[key]; ok {
				return val
			}
			return false
		},
	}
}
