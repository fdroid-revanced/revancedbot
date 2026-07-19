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

// Blob is the pasteable signing secret.
// Keystore is JKS (ReVanced CLI BouncyCastle does not accept modern PKCS12 well).
type Blob struct {
	V           int    `json:"v"`
	KeystoreB64 string `json:"keystore_b64"`
	// KeystoreP12B64 is legacy field name (still accepted on decode).
	KeystoreP12B64 string `json:"keystore_p12_b64,omitempty"`
	StorePass      string `json:"storepass"`
	KeyPass        string `json:"keypass"`
	Alias          string `json:"alias"`
	StoreType      string `json:"storetype,omitempty"` // default JKS
}

func (b *Blob) keystoreBytes() (string, error) {
	if b.KeystoreB64 != "" {
		return b.KeystoreB64, nil
	}
	if b.KeystoreP12B64 != "" {
		return b.KeystoreP12B64, nil
	}
	return "", fmt.Errorf("signing blob missing keystore bytes")
}

func (b *Blob) storeType() string {
	if b.StoreType != "" {
		return b.StoreType
	}
	// legacy blobs were PKCS12
	if b.KeystoreP12B64 != "" && b.KeystoreB64 == "" {
		return "PKCS12"
	}
	return "JKS"
}

// Encode returns a single-line base64(JSON) string for pasting into a secret.
func (b *Blob) Encode() (string, error) {
	b.V = blobVersion
	if b.StoreType == "" {
		b.StoreType = "JKS"
	}
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
	if _, err := b.keystoreBytes(); err != nil {
		return nil, err
	}
	if b.StorePass == "" || b.KeyPass == "" || b.Alias == "" {
		return nil, fmt.Errorf("signing blob missing required fields")
	}
	return &b, nil
}

// Materialize writes the keystore to path and validates with keytool.
func (b *Blob) Materialize(keystorePath string) error {
	b64, err := b.keystoreBytes()
	if err != nil {
		return err
	}
	bin, err := base64.StdEncoding.DecodeString(b64)
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

// ValidateKeystore runs keytool -list.
func (b *Blob) ValidateKeystore(keystorePath string) error {
	cmd := exec.Command("keytool",
		"-list",
		"-keystore", keystorePath,
		"-storetype", b.storeType(),
		"-storepass", b.StorePass,
		"-alias", b.Alias,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("keytool validate: %w\n%s", err, out)
	}
	return nil
}

// Generate creates a new JKS keystore via keytool (ReVanced-compatible).
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

	ks := filepath.Join(dir, "keystore.jks")
	// JKS: ReVanced CLI signs with BouncyCastle BcKeyStoreSpi which fails on modern PKCS12.
	cmd := exec.Command("keytool",
		"-genkeypair",
		"-v",
		"-keystore", ks,
		"-storetype", "JKS",
		"-storepass", storePass,
		"-keypass", keyPass,
		"-alias", alias,
		"-keyalg", "RSA",
		"-keysize", "2048",
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
		V:           blobVersion,
		KeystoreB64: base64.StdEncoding.EncodeToString(bin),
		StorePass:   storePass,
		KeyPass:     keyPass,
		Alias:       alias,
		StoreType:   "JKS",
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
