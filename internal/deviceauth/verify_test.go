package deviceauth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/venomimonstro/vpnx3/internal/nodeauth"
)

func TestECDSAP256RoundTrip(t *testing.T) {
	privateKey,err:=ecdsa.GenerateKey(elliptic.P256(),rand.Reader)
	if err!=nil { t.Fatal(err) }
	publicDER,err:=x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err!=nil { t.Fatal(err) }

	body:=[]byte(`{"sequence":1}`)
	now:=time.Now().UTC().Truncate(time.Second)
	ts:=strconv.FormatInt(now.Unix(),10)
	canonical:=nodeauth.Canonical(http.MethodPost,"/api/v1/client/lease",ts,body)
	digest:=sha256.Sum256(canonical)
	sig,err:=ecdsa.SignASN1(rand.Reader,privateKey,digest[:])
	if err!=nil { t.Fatal(err) }

	if err:=Verify(
		ECDSAP256SHA256,publicDER,http.MethodPost,"/api/v1/client/lease",
		ts,base64.RawURLEncoding.EncodeToString(sig),body,now,
	); err!=nil {
		t.Fatal(err)
	}
}
