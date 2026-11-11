package jwtmanager

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// LoadPrivateKey reads an Ed25519 private key from a PEM file.
func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key file: %w", err)
	}
	return ParsePrivateKeyPEM(string(data))
}

// LoadPublicKey reads an Ed25519 public key from a PEM file.
func LoadPublicKey(path string) (ed25519.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read public key file: %w", err)
	}
	return ParsePublicKeyPEM(string(data))
}

// ParsePrivateKeyPEM parses an Ed25519 private key from raw or
// base64-encoded PEM (the form an env var or secret usually holds).
func ParsePrivateKeyPEM(value string) (ed25519.PrivateKey, error) {
	data, err := decodeKeyPEM(value)
	if err != nil {
		return nil, err
	}
	key, err := jwt.ParseEdPrivateKeyFromPEM(data)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	return key.(ed25519.PrivateKey), nil
}

// ParsePublicKeyPEM parses an Ed25519 public key from raw or
// base64-encoded PEM.
func ParsePublicKeyPEM(value string) (ed25519.PublicKey, error) {
	data, err := decodeKeyPEM(value)
	if err != nil {
		return nil, err
	}
	key, err := jwt.ParseEdPublicKeyFromPEM(data)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	return key.(ed25519.PublicKey), nil
}

func decodeKeyPEM(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if strings.Contains(value, "-----BEGIN ") {
		return []byte(value), nil
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("JWT key must be raw PEM or standard base64-encoded PEM: %w", err)
	}
	return data, nil
}
