package configservice

import "encoding/base64"

func encodeEnvelope(payload,signature []byte,keyID string) Envelope {
	return Envelope{
		Payload:base64.RawURLEncoding.EncodeToString(payload),
		Signature:base64.RawURLEncoding.EncodeToString(signature),
		KeyID:keyID,
	}
}
