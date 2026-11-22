package main

import (
	"encoding/json"
	"html/template"
	"net/url"
	"strings"
)

func jsonMarshal(value any) ([]byte, error) {
	return json.Marshal(value)
}

type iconData struct {
	Size  int
	Class string
}

func iconSVG(name string, data iconData) template.HTML {
	size := data.Size
	if size <= 0 {
		size = 18
	}
	class := ""
	if data.Class != "" {
		class = ` class="` + template.HTMLEscapeString(data.Class) + `"`
	}
	open := `<svg width="` + itoa(size) + `" height="` + itoa(size) + `" viewBox="0 0 24 24" aria-hidden="true"` + class

	strokeIcon := func(body string) template.HTML {
		return template.HTML(open + ` fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`)
	}
	fillIcon := func(body string) template.HTML {
		return template.HTML(open + ` fill="currentColor">` + body + `</svg>`)
	}

	switch name {
	case "github":
		return fillIcon(`<path d="M12 2C6.48 2 2 6.58 2 12.25c0 4.53 2.87 8.37 6.84 9.73.5.1.68-.22.68-.49l-.01-1.7c-2.78.62-3.37-1.37-3.37-1.37-.46-1.18-1.11-1.5-1.11-1.5-.91-.64.07-.62.07-.62 1 .07 1.53 1.06 1.53 1.06.9 1.57 2.34 1.12 2.91.86.09-.67.35-1.12.63-1.38-2.22-.26-4.56-1.14-4.56-5.06 0-1.12.39-2.03 1.03-2.75-.1-.26-.45-1.3.1-2.71 0 0 .84-.28 2.75 1.05a9.36 9.36 0 0 1 5 0c1.91-1.33 2.75-1.05 2.75-1.05.55 1.41.2 2.45.1 2.71.64.72 1.03 1.63 1.03 2.75 0 3.93-2.34 4.79-4.57 5.05.36.32.68.94.68 1.9l-.01 2.82c0 .27.18.6.69.49A10.26 10.26 0 0 0 22 12.25C22 6.58 17.52 2 12 2z"/>`)
	case "archive":
		return strokeIcon(`<path d="M3 8V6a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2v2"/><path d="M4 8h16v10a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2z"/><path d="M10 12h4"/>`)
	case "code":
		return strokeIcon(`<path d="m9 8-4 4 4 4"/><path d="m15 8 4 4-4 4"/>`)
	case "upload":
		return strokeIcon(`<path d="M12 15V3"/><path d="m7 8 5-5 5 5"/><path d="M5 15v4a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-4"/>`)
	case "folder":
		return strokeIcon(`<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>`)
	case "file":
		return strokeIcon(`<path d="M14 3v5h5"/><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/>`)
	case "check":
		return strokeIcon(`<path d="M20 6 9 17l-5-5"/>`)
	case "alert":
		return strokeIcon(`<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/><path d="M12 9v4"/><path d="M12 17h.01"/>`)
	case "x":
		return strokeIcon(`<path d="M6 6 18 18"/><path d="M18 6 6 18"/>`)
	case "globe":
		return strokeIcon(`<circle cx="12" cy="12" r="9"/><path d="M3 12h18"/><path d="M12 3a15 15 0 0 1 0 18 15 15 0 0 1 0-18"/>`)
	case "copy":
		return strokeIcon(`<rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>`)
	case "refresh":
		return strokeIcon(`<path d="M21 12a9 9 0 1 1-2.64-6.36L21 8"/><path d="M21 3v5h-5"/>`)
	case "external":
		return strokeIcon(`<path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>`)
	case "search":
		return strokeIcon(`<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>`)
	case "trash":
		return strokeIcon(`<path d="M4 7h16"/><path d="M9 7V5a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"/><path d="M6 7l1 13a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1l1-13"/>`)
	case "layers":
		return strokeIcon(`<path d="m12 3 9 5-9 5-9-5 9-5z"/><path d="m3 13 9 5 9-5"/>`)
	case "rocket":
		return strokeIcon(`<circle cx="12" cy="12" r="9"/><path d="M12 16V8"/><path d="m8 12 4-4 4 4"/>`)
	default:
		return strokeIcon("")
	}
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"icon": func(name string, size int, class string) template.HTML {
			return iconSVG(name, iconData{Size: size, Class: class})
		},
		"mask":          maskName,
		"formatShort":   formatDateShort,
		"formatLong":    formatDateLong,
		"prettyURL":     prettyURL,
		"itoa":          itoa,
		"statusLabel":   func(status string) string { return statusLabel[status] },
		"nonEmpty":      func(value string) bool { return strings.TrimSpace(value) != "" },
		"jsonForScript": jsonForScript,
		"mul":           func(a, b int) int { return a * b },
		"add":           func(a, b int) int { return a + b },
		"urlquery":      url.QueryEscape,
	}
}
