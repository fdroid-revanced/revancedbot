package signing

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestBlobRoundTrip(t *testing.T) {
	b := &Blob{
		V:           1,
		KeystoreB64: "AAAA",
		StorePass:   "store",
		KeyPass:     "key",
		Alias:       "a",
		StoreType:   "JKS",
	}
	enc, err := b.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(enc, "\n") {
		t.Fatalf("encode must be one line: %q", enc)
	}
	got, err := DecodeBlob(enc)
	if err != nil {
		t.Fatal(err)
	}
	if got.V != 1 || got.Alias != "a" || got.StorePass != "store" || got.KeystoreB64 != "AAAA" || got.StoreType != "JKS" || got.KeystoreP12B64 != "" {
		t.Fatalf("%+v", got)
	}
}

func TestEncode_v1JKS(t *testing.T) {
	b := &Blob{
		KeystoreB64: "AAAA",
		StorePass:   "store",
		KeyPass:     "key",
		Alias:       "a",
	}
	enc, err := b.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(enc, "\r\n") {
		t.Fatalf("encode must be one line: %q", enc)
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["v"] != float64(1) {
		t.Fatalf("v = %v; want 1", m["v"])
	}
	if m["keystore_b64"] != "AAAA" {
		t.Fatalf("keystore_b64 = %v", m["keystore_b64"])
	}
	if m["storetype"] != "JKS" {
		t.Fatalf("storetype = %v; want JKS", m["storetype"])
	}
	if _, ok := m["keystore_p12_b64"]; ok {
		t.Fatalf("encode must omit keystore_p12_b64: %v", m)
	}
}

func TestDecodeRejectsEmpty(t *testing.T) {
	if _, err := DecodeBlob(""); err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeBlob(t *testing.T) {
	valid := `{"v":1,"keystore_b64":"AAAA","storepass":"s","keypass":"k","alias":"a"}`
	tests := []struct {
		name    string
		in      string
		wantErr error
	}{
		{name: "jks json empty storetype", in: valid},
		{name: "jks storetype", in: `{"v":1,"keystore_b64":"AAAA","storepass":"s","keypass":"k","alias":"a","storetype":"JKS"}`},
		{name: "legacy p12 field", in: `{"v":1,"keystore_p12_b64":"AAAA","storepass":"s","keypass":"k","alias":"a"}`, wantErr: ErrPKCS12},
		{name: "p12 field with keystore_b64", in: `{"v":1,"keystore_b64":"AAAA","keystore_p12_b64":"BBBB","storepass":"s","keypass":"k","alias":"a"}`, wantErr: ErrPKCS12},
		{name: "storetype PKCS12", in: `{"v":1,"keystore_b64":"AAAA","storepass":"s","keypass":"k","alias":"a","storetype":"PKCS12"}`, wantErr: ErrPKCS12},
		{name: "missing keystore_b64", in: `{"v":1,"storepass":"s","keypass":"k","alias":"a"}`, wantErr: ErrBase},
		{name: "unsupported storetype", in: `{"v":1,"keystore_b64":"AAAA","storepass":"s","keypass":"k","alias":"a","storetype":"BKS"}`, wantErr: ErrBase},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := DecodeBlob(tt.in)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("DecodeBlob(%s) err = %v; want %v", tt.name, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("DecodeBlob(%s) unexpected err %v", tt.name, err)
			}
			if got.KeystoreB64 != "AAAA" || got.storeType() != "JKS" || got.KeystoreP12B64 != "" {
				t.Fatalf("%+v", got)
			}
		})
	}
}

func TestEncode_rejectsPKCS12(t *testing.T) {
	tests := []struct {
		name string
		b    Blob
	}{
		{name: "legacy p12 field", b: Blob{KeystoreP12B64: "AAAA", StorePass: "s", KeyPass: "k", Alias: "a"}},
		{name: "storetype PKCS12", b: Blob{KeystoreB64: "AAAA", StorePass: "s", KeyPass: "k", Alias: "a", StoreType: "PKCS12"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := tt.b.Encode(); !errors.Is(err, ErrPKCS12) {
				t.Fatalf("Encode(%s) err = %v; want %v", tt.name, err, ErrPKCS12)
			}
		})
	}
}
