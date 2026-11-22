package main

import (
	"html/template"
	"strconv"
	"strings"
	"time"
)

func itoa(value int) string { return strconv.Itoa(value) }

// maskName hides all but the first two characters of a project name for the
// public list. Names of 0-1 characters become stars entirely.
func maskName(name string) string {
	if name == "" {
		return "?"
	}
	shown := len(name) - 1
	if shown > 2 {
		shown = 2
	}
	if shown < 0 {
		shown = 0
	}
	return name[:shown] + strings.Repeat("*", len(name)-shown)
}

func formatDateShort(value time.Time) string {
	if value.IsZero() {
		return "—"
	}
	months := [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}
	return strconv.Itoa(value.Day()) + " " + months[int(value.Month())-1]
}

func formatDateLong(value time.Time) string {
	if value.IsZero() {
		return "—"
	}
	months := [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}
	return two(value.Day()) + " " + months[int(value.Month())-1] + " " + strconv.Itoa(value.Year()%100) +
		" " + two(value.Hour()) + "." + two(value.Minute())
}

func two(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

func prettyURL(rawURL string) string {
	value := strings.TrimPrefix(strings.TrimPrefix(rawURL, "https://"), "http://")
	return strings.TrimSuffix(value, "/")
}

func jsonForScript(value any) template.JS {
	data, err := jsonMarshal(value)
	if err != nil {
		return template.JS("{}")
	}
	return template.JS(strings.ReplaceAll(string(data), "<", `\u003c`))
}
