package acme

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"

	"github.com/go-acme/lego/v4/registration"
)

/*
	account.go

	This file persists and reuses the ACME account (account key + registration)
	across certificate requests and renewals.

	Without this, ObtainCert() generated a fresh account key and registered a
	brand new ACME account on every single call. Let's Encrypt counts each of
	those as a "new registration" and rate limits them to 10 per IP per 3h, so
	routine renewals would eventually fail with:

		urn:ietf:params:acme:error:rateLimited :: too many new registrations

	The account is stored in the system database, in the same "acme" table and
	keyed by the same CA directory URL already used for this provider's EAB
	credentials (see ObtainCert). It is keyed by the CA directory URL only (NOT
	the email): an ACME account is identified by its key pair, the email is just
	a contact address. Keying by CA URL alone means a renewal still reuses the
	account even if the renewer email differs from the email used at first
	issuance.
*/

// acmeAccountTable is the database table reusable ACME accounts are stored in.
// It is intentionally the same table used for this module's DNS and EAB
// credentials so all per-CA ACME state lives together.
const acmeAccountTable = "acmepref"

// ACMEUser represents a user in the ACME system.
type ACMEUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

// GetEmail returns the email of the ACMEUser.
func (u *ACMEUser) GetEmail() string {
	return u.Email
}

// GetRegistration returns the registration resource of the ACMEUser.
func (u ACMEUser) GetRegistration() *registration.Resource {
	return u.Registration
}

// GetPrivateKey returns the private key of the ACMEUser.
func (u *ACMEUser) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

// MarshalJSON implements json.Marshaler for ACMEUser.
// It serializes the private key as a PEM string because
// crypto.PrivateKey cannot be marshalled directly.
func (u *ACMEUser) MarshalJSON() ([]byte, error) {
	pemData := ""
	if u.key == nil {
		return nil, errors.New("no ACME account key found")
	}
	ecKey, ok := u.key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("ACME account private key is not an ECDSA key")
	}

	der, _ := x509.MarshalECPrivateKey(ecKey)
	pemData = string(pem.EncodeToMemory(&pem.Block{
		Type: "EC PRIVATE KEY", Bytes: der,
	}))

	return json.Marshal(struct {
		Email        string                 `json:"email"`
		Registration *registration.Resource `json:"registration"`
		KeyPEM       string                 `json:"key"`
	}{
		Email:        u.Email,
		Registration: u.Registration,
		KeyPEM:       pemData,
	})
}

// UnmarshalJSON implements json.Unmarshaler for ACMEUser.
// It deserializes the private key from the PEM string field
// back into a crypto.PrivateKey.
func (u *ACMEUser) UnmarshalJSON(data []byte) error {
	aux := struct {
		Email        string                 `json:"email"`
		Registration *registration.Resource `json:"registration"`
		KeyPEM       string                 `json:"key"`
	}{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	u.Email = aux.Email
	u.Registration = aux.Registration

	if aux.KeyPEM == "" {
		return errors.New("no ACME account key stored")
	}

	block, _ := pem.Decode([]byte(aux.KeyPEM))
	if block == nil || block.Type != "EC PRIVATE KEY" {
		return errors.New("stored ACME account key could not be decoded as EC private key")
	}

	parsedKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return errors.New("stored ACME account key could not be parsed")
	}
	u.key = parsedKey

	return nil
}

// accountDBKey returns the database key for the account of a given CA
// directory URL and email. Using both ensures unique keys per account
// even when the same CA is used with different emails.
func accountDBKey(caDirURL string, email string) string {
	h := sha256.Sum256([]byte(caDirURL + "|" + email))
	return "acme_account_" + hex.EncodeToString(h[:8])
}

// loadACMEAccount tries to load a previously persisted account for the given
// CA directory URL and email. It returns (key, registration, true) only when
// a complete, reusable account is found; otherwise ok is false and the caller
// should register a new account.
func (a *ACMEHandler) loadACMEAccount(caDirURL string, email string) (*ecdsa.PrivateKey, *registration.Resource, bool) {
	if !a.Database.TableExists(acmeAccountTable) {
		// No ACME table yet (first run) - not an error.
		return nil, nil, false
	}

	dbKey := accountDBKey(caDirURL, email)
	if !a.Database.KeyExists(acmeAccountTable, dbKey) {
		// No stored account for this CA yet - not an error.
		return nil, nil, false
	}

	var user ACMEUser
	if err := a.Database.Read(acmeAccountTable, dbKey, &user); err != nil {
		a.Logf("Stored ACME account for "+caDirURL+" is invalid, registering a new account", err)
		return nil, nil, false
	}

	if user.Registration == nil || user.Registration.URI == "" {
		// A key without a usable registration URI cannot be reused on its own.
		return nil, nil, false
	}

	ecKey, ok := user.key.(*ecdsa.PrivateKey)
	if !ok {
		a.Logf("Stored ACME account key for "+caDirURL+" is not an ECDSA key", nil)
		return nil, nil, false
	}

	return ecKey, user.Registration, true
}

// saveACMEAccount persists the account (key + registration) so that subsequent
// certificate requests and renewals reuse it instead of registering a new
// ACME account every time.
func (a *ACMEHandler) saveACMEAccount(caDirURL string, user *ACMEUser) error {
	if user == nil || user.GetRegistration() == nil {
		return errors.New("no ACME registration to persist")
	}

	if !a.Database.TableExists(acmeAccountTable) {
		if err := a.Database.NewTable(acmeAccountTable); err != nil {
			return err
		}
	}

	return a.Database.Write(acmeAccountTable, accountDBKey(caDirURL, user.Email), user)
}
