package configservice

import (
	"encoding/base64"
	"encoding/json"
)

func encodeEnvelope(payload,signature []byte,keyID string) Envelope {
	return Envelope{
		Manifest:json.RawMessage(payload),
		Signature:base64.RawURLEncoding.EncodeToString(signature),
		KeyID:keyID,
	}
}
