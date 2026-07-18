package signing

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const blobVersion = 1

// Blob is the pasteable signing secret (JSON, usually base64-wrapped for env safety).
type Blob struct {
	V              int    `json:"v"`
	KeystoreP12B64 string `json:"keystore_p12_b64"`
	StorePass      string `json:"storepass"`
	KeyPass        string `json:"keypass"`
	Alias          string `json:"alias"`
}

// Encode returns a single-line base64(JSON) string for pasting into a secret.
func (b *Blob) Encode() (string, error) {
	b.V = blobVersion
	raw, err := json.Marshal(b)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// DecodeBlob parses a pasteable secret (raw JSON or base64 JSON).
func DecodeBlob(s string) (*Blob, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty signing blob")
	}
	var raw []byte
	if strings.HasPrefix(s, "{") {
		raw = []byte(s)
	} else {
		decoded, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("decode signing blob: %w", err)
		}
		raw = decoded
	}
	var b Blob
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("parse signing blob JSON: %w", err)
	}
	if b.V != blobVersion {
		return nil, fmt.Errorf("unsupported signing blob version %d (want %d)", b.V, blobVersion)
	}
	if b.KeystoreP12B64 == "" || b.StorePass == "" || b.KeyPass == "" || b.Alias == "" {
		return nil, fmt.Errorf("signing blob missing required fields")
	}
	return &b, nil
}

// Materialize writes the PKCS12 keystore to path and returns the blob for further use.
func (b *Blob) Materialize(keystorePath string) error {
	bin, err := base64.StdEncoding.DecodeString(b.KeystoreP12B64)
	if err != nil {
		return fmt.Errorf("decode keystore bytes: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keystorePath), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(keystorePath, bin, 0o600); err != nil {
		return err
	}
	return b.ValidateKeystore(keystorePath)
}

// ValidateKeystore runs keytool -list to ensure the alias is usable.
func (b *Blob) ValidateKeystore(keystorePath string) error {
	cmd := exec.Command("keytool",
		"-list",
		"-keystore", keystorePath,
		"-storetype", "PKCS12",
		"-storepass", b.StorePass,
		"-alias", b.Alias,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("keytool validate: %w\n%s", err, out)
	}
	return nil
}

// Generate creates a new keystore via keytool and returns an encoded blob.
func Generate(alias string) (encoded string, err error) {
	if alias == "" {
		alias = "revancedbot"
	}
	storePass, err := randomPass(24)
	if err != nil {
		return "", err
	}
	keyPass := storePass

	dir, err := os.MkdirTemp("", "revancedbot-keys-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	ks := filepath.Join(dir, "keystore.p12")
	// 4096-bit RSA, 10000-day validity — operator key for repo + APKs.
	cmd := exec.Command("keytool",
		"-genkeypair",
		"-v",
		"-keystore", ks,
		"-storetype", "PKCS12",
		"-storepass", storePass,
		"-keypass", keyPass,
		"-alias", alias,
		"-keyalg", "RSA",
		"-keysize", "4096",
		"-validity", "10000",
		"-dname", "CN=revancedbot, OU=F-Droid, O=revancedbot, L=Unknown, ST=Unknown, C=US",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("keytool genkeypair: %w\n%s", err, out)
	}

	bin, err := os.ReadFile(ks)
	if err != nil {
		return "", err
	}
	b := &Blob{
		V:              blobVersion,
		KeystoreP12B64: base64.StdEncoding.EncodeToString(bin),
		StorePass:      storePass,
		KeyPass:        keyPass,
		Alias:          alias,
	}
	if err := b.ValidateKeystore(ks); err != nil {
		return "", err
	}
	return b.Encode()
}

func randomPass(n int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(buf), nil
}
