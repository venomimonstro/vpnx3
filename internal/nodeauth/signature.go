package nodeauth

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func Canonical(method,path,timestamp string,body []byte) []byte {
	sum := sha256.Sum256(body)
	return []byte(strings.Join([]string{
		method,
		path,
		timestamp,
		fmt.Sprintf("%x",sum[:]),
	},"\n"))
}

func Verify(publicKey []byte, method,path,timestamp,signature string,body []byte,now time.Time) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid node public key")
	}
	unix, err := strconv.ParseInt(timestamp,10,64)
	if err != nil { return fmt.Errorf("invalid timestamp") }
	sent := time.Unix(unix,0)
	if now.Sub(sent) > 2*time.Minute || sent.Sub(now) > 2*time.Minute {
		return fmt.Errorf("timestamp outside allowed window")
	}
	sig, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil { return fmt.Errorf("invalid signature encoding") }
	if !ed25519.Verify(ed25519.PublicKey(publicKey),Canonical(method,path,timestamp,body),sig) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

func Sign(privateKey ed25519.PrivateKey, method,path,timestamp string,body []byte) string {
	sig := ed25519.Sign(privateKey,Canonical(method,path,timestamp,body))
	return base64.RawURLEncoding.EncodeToString(sig)
}
