package accesslease

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/venomimonstro/vpnx3/internal/signing"
)

type Claims struct {
	SchemaVersion int       `json:"schema_version"`
	LeaseID       string    `json:"lease_id"`
	UserID        string    `json:"user_id"`
	DeviceID      string    `json:"device_id"`
	Entitlement   string    `json:"entitlement"`
	IssuedAt      time.Time `json:"issued_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type Envelope struct {
	Claims    json.RawMessage `json:"claims"`
	Signature string          `json:"signature"`
	KeyID     string          `json:"key_id"`
}

func Issue(signer *signing.Signer,claims Claims) (Envelope,error) {
	raw,err:=json.Marshal(claims)
	if err!=nil { return Envelope{},fmt.Errorf("marshal access lease: %w",err) }
	sig:=signer.Sign(raw)
	return Envelope{
		Claims:raw,
		Signature:base64.RawURLEncoding.EncodeToString(sig),
		KeyID:signer.KeyID(),
	},nil
}
