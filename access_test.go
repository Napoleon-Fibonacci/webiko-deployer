package main

import (
	"net/http"
	"testing"
)

func TestDeployAccessCodeConfig(t *testing.T) {
	t.Setenv("DEPLOY_ACCESS_CODE", "")
	if accessCodeRequired() {
		t.Fatal("empty DEPLOY_ACCESS_CODE must leave deploy open")
	}
	if !accessCodeMatches("anything", "") {
		t.Fatal("empty configured code must accept anything")
	}

	t.Setenv("DEPLOY_ACCESS_CODE", "  rahasia-123  ")
	if !accessCodeRequired() {
		t.Fatal("configured code must require access")
	}
	if got := deployAccessCode(); got != "rahasia-123" {
		t.Fatalf("deployAccessCode() = %q, want trimmed value", got)
	}
	if !accessCodeMatches("rahasia-123", deployAccessCode()) {
		t.Fatal("matching code rejected")
	}
	if !accessCodeMatches("  rahasia-123  ", deployAccessCode()) {
		t.Fatal("code with stray spaces rejected")
	}
	if accessCodeMatches("rahasia-124", deployAccessCode()) {
		t.Fatal("wrong code accepted")
	}
	if accessCodeMatches("", deployAccessCode()) {
		t.Fatal("empty code accepted while a code is configured")
	}
}

func TestCheckDeployAccessOpenWhenUnconfigured(t *testing.T) {
	t.Setenv("DEPLOY_ACCESS_CODE", "")
	if err := checkDeployAccess("", "127.0.0.1"); err != nil {
		t.Fatalf("checkDeployAccess without a configured code: %v", err)
	}
}

func TestCheckDeployAccessAcceptsCorrectCode(t *testing.T) {
	t.Setenv("DEPLOY_ACCESS_CODE", "kode-uji")
	if err := checkDeployAccess("kode-uji", "127.0.0.1"); err != nil {
		t.Fatalf("checkDeployAccess with the correct code: %v", err)
	}
}

func TestCheckDeployAccessRejectsWrongCode(t *testing.T) {
	t.Setenv("DEPLOY_ACCESS_CODE", "kode-uji")
	err := checkDeployAccess("salah", "127.0.0.1")
	if err == nil {
		t.Fatal("wrong code accepted")
	}
	if got := errorCode(err); got != http.StatusForbidden {
		t.Fatalf("errorCode = %d, want %d", got, http.StatusForbidden)
	}
	if got := errorField(err); got != FieldAccess {
		t.Fatalf("errorField = %q, want %q", got, FieldAccess)
	}
}
