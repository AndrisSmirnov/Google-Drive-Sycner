// Package drive is a read-only client for Google Drive. It signs in with a
// Service Account key and asks for the drive.readonly scope only, so nothing
// here can change what is on Drive.
package drive

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
)

// Key is a Service Account key: the identity the client signs in with.
type Key struct {
	// Email is the address a Drive folder has to be shared with.
	Email string

	private  *rsa.PrivateKey
	tokenURI string
}

// ParseKey reads the JSON key file that Google Cloud gives for a Service
// Account. Errors never quote the file: it holds a private key.
func ParseKey(data []byte) (*Key, error) {
	var raw struct {
		Type        string `json:"type"`
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
		TokenURI    string `json:"token_uri"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, errors.New("the key file is not valid JSON")
	}
	if raw.Type != "service_account" {
		return nil, fmt.Errorf("the key file is of type %q, a Service Account key is needed", raw.Type)
	}
	if raw.ClientEmail == "" || raw.TokenURI == "" {
		return nil, errors.New("the key file has no account email or token address")
	}
	block, _ := pem.Decode([]byte(raw.PrivateKey))
	if block == nil {
		return nil, errors.New("the key file holds no private key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("the private key in the key file cannot be read")
	}
	private, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("the private key in the key file is not an RSA key")
	}
	return &Key{Email: raw.ClientEmail, private: private, tokenURI: raw.TokenURI}, nil
}
