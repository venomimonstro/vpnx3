package clientconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Cache struct {
	Path string
	Verifier *Verifier
}

type cachedEnvelope struct {
	Envelope Envelope `json:"envelope"`
	Version int64 `json:"version"`
	SavedAt time.Time `json:"saved_at"`
}

func (c Cache) Load(now time.Time) (Envelope,ManifestMeta,error) {
	raw,err:=os.ReadFile(c.Path)
	if err!=nil { return Envelope{},ManifestMeta{},err }
	var stored cachedEnvelope
	if err:=json.Unmarshal(raw,&stored); err!=nil {
		return Envelope{},ManifestMeta{},fmt.Errorf("decode cached config: %w",err)
	}
	meta,err:=c.Verifier.Verify(stored.Envelope,stored.Version,now)
	if err!=nil { return Envelope{},ManifestMeta{},err }
	return stored.Envelope,meta,nil
}

func (c Cache) Save(env Envelope,minimumVersion int64,now time.Time) (ManifestMeta,error) {
	meta,err:=c.Verifier.Verify(env,minimumVersion,now)
	if err!=nil { return ManifestMeta{},err }
	stored:=cachedEnvelope{Envelope:env,Version:meta.Version,SavedAt:now.UTC()}
	raw,err:=json.Marshal(stored)
	if err!=nil { return ManifestMeta{},err }
	if err:=os.MkdirAll(filepath.Dir(c.Path),0700); err!=nil { return ManifestMeta{},err }
	tmp:=c.Path+".tmp"
	if err:=os.WriteFile(tmp,raw,0600); err!=nil { return ManifestMeta{},err }
	if err:=os.Rename(tmp,c.Path); err!=nil { return ManifestMeta{},err }
	return meta,nil
}
