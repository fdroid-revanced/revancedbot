package signing

import "testing"

func TestBlobRoundTrip(t *testing.T) {
	b := &Blob{
		V:              1,
		KeystoreP12B64: "AAAA",
		StorePass:      "store",
		KeyPass:        "key",
		Alias:          "a",
	}
	enc, err := b.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeBlob(enc)
	if err != nil {
		t.Fatal(err)
	}
	if got.Alias != "a" || got.StorePass != "store" {
		t.Fatalf("%+v", got)
	}
}

func TestDecodeRejectsEmpty(t *testing.T) {
	if _, err := DecodeBlob(""); err == nil {
		t.Fatal("expected error")
	}
}
