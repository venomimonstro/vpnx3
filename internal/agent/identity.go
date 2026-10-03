package agent

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Identity struct {
	NodeID     string `json:"node_id"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
	Sequence   int64  `json:"sequence"`
}

func LoadOrCreate(path string) (Identity,error) {
	raw,err := os.ReadFile(path)
	if err == nil {
		var id Identity
		if err := json.Unmarshal(raw,&id); err != nil { return Identity{},err }
		return id,nil
	}
	if !os.IsNotExist(err) { return Identity{},err }

	pub,priv,err := ed25519.GenerateKey(rand.Reader)
	if err != nil { return Identity{},err }
	id := Identity{
		PublicKey:base64.RawURLEncoding.EncodeToString(pub),
		PrivateKey:base64.RawURLEncoding.EncodeToString(priv),
	}
	if err := Save(path,id); err != nil { return Identity{},err }
	return id,nil
}

func Save(path string,id Identity) error {
	if err := os.MkdirAll(filepath.Dir(path),0700); err != nil { return err }
	data,err := json.MarshalIndent(id,"","  ")
	if err != nil { return err }
	tmp := path+".tmp"
	if err := os.WriteFile(tmp,data,0600); err != nil { return err }
	if err := os.Rename(tmp,path); err != nil { return fmt.Errorf("replace identity: %w",err) }
	return nil
}

func (i Identity) Private() (ed25519.PrivateKey,error) {
	raw,err := base64.RawURLEncoding.DecodeString(i.PrivateKey)
	if err != nil { return nil,err }
	if len(raw)!=ed25519.PrivateKeySize { return nil,fmt.Errorf("invalid private key") }
	return ed25519.PrivateKey(raw),nil
}
