package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestFromZipBufferRejectsTooManyEntries(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for index := 0; index < maxZipEntries+1; index++ {
		file, err := writer.Create(fmt.Sprintf("file-%d.txt", index))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = file.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	_, err := fromZipBuffer(buffer.Bytes(), "ZIP")
	if err == nil {
		t.Fatal("expected too-many-entries error")
	}
	if !strings.Contains(errorMessage(err), "entri") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFromZipBufferRejectsExpandingTotal(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	chunk := bytes.Repeat([]byte("a"), 9<<20)
	for index := 0; index < 6; index++ {
		file, err := writer.Create(fmt.Sprintf("big-%d.txt", index))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = file.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	_, err := fromZipBuffer(buffer.Bytes(), "ZIP")
	if err == nil {
		t.Fatal("expected zip bomb error")
	}
	if !strings.Contains(errorMessage(err), "50 MB") {
		t.Fatalf("unexpected error: %v", err)
	}
}
