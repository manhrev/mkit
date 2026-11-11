package jwtmanager

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"
)

func TestParseKeysPEM(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	privDER, _ := x509.MarshalPKCS8PrivateKey(priv)
	pubDER, _ := x509.MarshalPKIXPublicKey(pub)
	privatePEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER}))
	publicPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))

	gotPriv, err := ParsePrivateKeyPEM("Bag Attributes\n" + privatePEM)
	if err != nil || !gotPriv.Equal(priv) {
		t.Fatalf("private key with leading text: %v", err)
	}

	for _, encoded := range []bool{false, true} {
		privateValue, publicValue := privatePEM, publicPEM
		if encoded {
			privateValue = base64.StdEncoding.EncodeToString([]byte(privatePEM))
			publicValue = base64.StdEncoding.EncodeToString([]byte(publicPEM))
		}
		gotPriv, err := ParsePrivateKeyPEM(" \n" + privateValue + "\n ")
		if err != nil || !gotPriv.Equal(priv) {
			t.Fatalf("private key (base64=%v): %v", encoded, err)
		}
		gotPub, err := ParsePublicKeyPEM(" \n" + publicValue + "\n ")
		if err != nil || !gotPub.Equal(pub) {
			t.Fatalf("public key (base64=%v): %v", encoded, err)
		}
	}
}

func TestParseKeysRejectInvalidPEM(t *testing.T) {
	for _, value := range []string{"", "invalid", "/tmp/key.pem", base64.StdEncoding.EncodeToString([]byte("not a PEM key"))} {
		if _, err := ParsePrivateKeyPEM(value); err == nil {
			t.Errorf("private key accepted %q", value)
		}
		if _, err := ParsePublicKeyPEM(value); err == nil {
			t.Errorf("public key accepted %q", value)
		}
	}
}
