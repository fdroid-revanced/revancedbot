package signing

import (
	"os/exec"
	"strings"
	"testing"
)

func TestGenerate_oneNonEmptyLine(t *testing.T) {
	if _, err := exec.LookPath("keytool"); err != nil {
		t.Skip(err)
	}
	enc, err := Generate("t40")
	if err != nil {
		t.Fatal(err)
	}
	if enc == "" {
		t.Fatal("empty blob")
	}
	if strings.ContainsAny(enc, "\n\r") {
		t.Fatalf("Generate must return one line, got %q", enc)
	}
	got, err := DecodeBlob(enc)
	if err != nil {
		t.Fatalf("DecodeBlob: %v", err)
	}
	if got.Alias != "t40" {
		t.Fatalf("alias = %q; want t40", got.Alias)
	}
}
