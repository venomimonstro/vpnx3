package deviceauth

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/nodeauth"
)

const (
	Ed25519          = "ed25519"
	ECDSAP256SHA256  = "ecdsa-p256-sha256"
)

func ValidatePublicKey(algorithm string,publicKey []byte) error {
	switch algorithm {
	case Ed25519:
		if len(publicKey)!=ed25519.PublicKeySize { return fmt.Errorf("invalid ed25519 public key") }
		return nil
	case ECDSAP256SHA256:
		key,err:=x509.ParsePKIXPublicKey(publicKey)
		if err!=nil { return fmt.Errorf("parse ecdsa public key: %w",err) }
		ec,ok:=key.(*ecdsa.PublicKey)
		if !ok || ec.Curve.Params().Name!="P-256" { return fmt.Errorf("public key is not P-256") }
		return nil
	default:
		return fmt.Errorf("unsupported identity algorithm")
	}
}

func Verify(algorithm string,publicKey []byte,method,path,timestamp,signature string,body []byte,now time.Time) error {
	unix,err:=strconv.ParseInt(timestamp,10,64)
	if err!=nil { return fmt.Errorf("invalid timestamp") }
	sent:=time.Unix(unix,0)
	if now.Sub(sent)>2*time.Minute || sent.Sub(now)>2*time.Minute {
		return fmt.Errorf("timestamp outside allowed window")
	}
	sig,err:=base64.RawURLEncoding.DecodeString(strings.TrimSpace(signature))
	if err!=nil { return fmt.Errorf("invalid signature encoding") }
	canonical:=nodeauth.Canonical(method,path,timestamp,body)

	switch algorithm {
	case Ed25519:
		if err:=ValidatePublicKey(algorithm,publicKey); err!=nil { return err }
		if !ed25519.Verify(ed25519.PublicKey(publicKey),canonical,sig) {
			return fmt.Errorf("signature verification failed")
		}
		return nil
	case ECDSAP256SHA256:
		if err:=ValidatePublicKey(algorithm,publicKey); err!=nil { return err }
		keyAny,_:=x509.ParsePKIXPublicKey(publicKey)
		key:=keyAny.(*ecdsa.PublicKey)
		digest:=sha256.Sum256(canonical)
		if !ecdsa.VerifyASN1(key,digest[:],sig) {
			return fmt.Errorf("signature verification failed")
		}
		return nil
	default:
		return fmt.Errorf("unsupported identity algorithm")
	}
}
