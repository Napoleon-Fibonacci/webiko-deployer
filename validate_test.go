package main

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestIsValidName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"abc", true},
		{"a-b-c", true},
		{"123", true},
		{"ab", false},
		{"-abc", false},
		{"abc-", false},
		{"Abc", false},
		{"abc_def", false},
		{strings.Repeat("a", 40), true},
		{strings.Repeat("a", 41), false},
		{"", false},
	}
	for _, tc := range cases {
		if got := isValidName(tc.name); got != tc.want {
			t.Errorf("isValidName(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestValidateName(t *testing.T) {
	if _, err := validateName(""); err == nil || errorField(err) != FieldName {
		t.Fatalf("empty name should fail on the name field")
	}
	if _, err := validateName("AB"); err == nil {
		t.Fatalf("uppercase/short name should fail")
	}
	got, err := validateName("situs-ku")
	if err != nil || got != "situs-ku" {
		t.Fatalf("valid name rejected: %v %v", got, err)
	}
}

func TestAssertSafePath(t *testing.T) {
	safe := []string{"index.html", "assets/app.css", "a/b/c.js"}
	for _, value := range safe {
		if err := assertSafePath(value); err != nil {
			t.Errorf("%q rejected: %v", value, err)
		}
	}
	unsafe := []string{"../secret", "/etc/passwd", "a/../../b", "C:/Windows", `a\..\b`}
	for _, value := range unsafe {
		if err := assertSafePath(value); err == nil {
			t.Errorf("%q should be rejected", value)
		}
	}
}

func TestStripRoot(t *testing.T) {
	files := []DeployFile{
		{Path: "repo-main/index.html", Data: []byte("hi")},
		{Path: "repo-main/assets/app.css", Data: []byte("body{}")},
	}
	got := stripRoot(files)
	if len(got) != 2 || got[0].Path != "index.html" || got[1].Path != "assets/app.css" {
		t.Fatalf("stripRoot = %#v", got)
	}

	flat := []DeployFile{{Path: "index.html"}, {Path: "style.css"}}
	got = stripRoot(flat)
	if len(got) != 2 || got[0].Path != "index.html" {
		t.Fatalf("flat tree must not be stripped: %#v", got)
	}
}

func TestEnsureIndexHTML(t *testing.T) {
	files, err := ensureIndexHTML([]DeployFile{{Path: "halaman.html", Data: []byte("x")}})
	if err != nil || len(files) != 2 || files[1].Path != "index.html" {
		t.Fatalf("single root html should become index.html: %#v %v", files, err)
	}

	if _, err = ensureIndexHTML([]DeployFile{{Path: "a.html"}, {Path: "b.html"}}); err == nil {
		t.Fatalf("two root html files without index should fail")
	}

	apiOnly, err := ensureIndexHTML([]DeployFile{{Path: "api/hello.go", Data: []byte("x")}})
	if err != nil || len(apiOnly) != 1 {
		t.Fatalf("api-only tree should pass: %#v %v", apiOnly, err)
	}
}

func TestValidateFilesRejectsBuildScript(t *testing.T) {
	_, err := validateFiles([]DeployFile{
		{Path: "index.html", Data: []byte("<h1>hi</h1>")},
		{Path: "package.json", Data: []byte(`{"scripts":{"build":"next build"}}`)},
	}, "ZIP")
	if err == nil {
		t.Fatalf("package.json with build script must be rejected")
	}
}

func TestFromZipBuffer(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("site-main/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = entry.Write([]byte("<h1>halo</h1>")); err != nil {
		t.Fatal(err)
	}
	entry, err = writer.Create("site-main/style.css")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = entry.Write([]byte("body{color:red}")); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}

	files, err := fromZipBuffer(buffer.Bytes(), "ZIP")
	if err != nil {
		t.Fatalf("fromZipBuffer: %v", err)
	}
	if len(files) != 2 || files[0].Path != "index.html" {
		t.Fatalf("unexpected files: %#v", files)
	}
}

func TestFromZipBufferRejectsGarbage(t *testing.T) {
	if _, err := fromZipBuffer([]byte("not a zip"), "ZIP"); err == nil {
		t.Fatalf("garbage zip should fail")
	}
}
