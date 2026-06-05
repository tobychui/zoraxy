package acme

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/x509"
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

// acmeAccountTable is the database table reusable ACME accounts are stored in.
// It is intentionally the same table used for this module's DNS and EAB
// credentials so all per-CA ACME state lives together.
const acmeAccountTable = "acme"

// acmeAccountStore is the stored representation of a reusable ACME account.
type acmeAccountStore struct {
	Email         string                 `json:"email"`
	Registration  *registration.Resource `json:"registration"`
	PrivateKeyPEM string                 `json:"private_key_pem"`
}

// accountDBKey returns the database key for the account of a given CA
// directory URL. It mirrors the "<caDirURL>_<field>" key convention this
// module already uses for EAB (kid/hmacEncoded) credentials.
func accountDBKey(caDirURL string) string {
	return caDirURL + "_account"
}

// loadACMEAccount tries to load a previously persisted account for the given
// CA directory URL. It returns (key, registration, true) only when a complete,
// reusable account is found; otherwise ok is false and the caller should
// register a new account.
func (a *ACMEHandler) loadACMEAccount(caDirURL string) (*ecdsa.PrivateKey, *registration.Resource, bool) {
	if !a.Database.TableExists(acmeAccountTable) {
		// No ACME table yet (first run) - not an error.
		return nil, nil, false
	}

	dbKey := accountDBKey(caDirURL)
	if !a.Database.KeyExists(acmeAccountTable, dbKey) {
		// No stored account for this CA yet - not an error.
		return nil, nil, false
	}

	var store acmeAccountStore
	if err := a.Database.Read(acmeAccountTable, dbKey, &store); err != nil {
		a.Logf("Stored ACME account for "+caDirURL+" is invalid, registering a new account", err)
		return nil, nil, false
	}

	block, _ := pem.Decode([]byte(store.PrivateKeyPEM))
	if block == nil {
		a.Logf("Stored ACME account key for "+caDirURL+" could not be decoded, registering a new account", nil)
		return nil, nil, false
	}

	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		a.Logf("Stored ACME account key for "+caDirURL+" could not be parsed, registering a new account", err)
		return nil, nil, false
	}

	if store.Registration == nil || store.Registration.URI == "" {
		// A key without a usable registration URI cannot be reused on its own.
		return nil, nil, false
	}

	return key, store.Registration, true
}

// saveACMEAccount persists the account (key + registration) so that subsequent
// certificate requests and renewals reuse it instead of registering a new
// ACME account every time.
func (a *ACMEHandler) saveACMEAccount(caDirURL string, user *ACMEUser) error {
	if user == nil || user.GetRegistration() == nil {
		return errors.New("no ACME registration to persist")
	}

	ecKey, ok := user.GetPrivateKey().(*ecdsa.PrivateKey)
	if !ok {
		return errors.New("ACME account private key is not an ECDSA key")
	}

	der, err := x509.MarshalECPrivateKey(ecKey)
	if err != nil {
		return err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})

	store := acmeAccountStore{
		Email:         user.GetEmail(),
		Registration:  user.GetRegistration(),
		PrivateKeyPEM: string(keyPEM),
	}

	if !a.Database.TableExists(acmeAccountTable) {
		if err := a.Database.NewTable(acmeAccountTable); err != nil {
			return err
		}
	}

	return a.Database.Write(acmeAccountTable, accountDBKey(caDirURL), &store)
}
