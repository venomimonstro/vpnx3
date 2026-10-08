package config

import (
	"crypto/ed25519"
	"encoding/base64"
	"strings"
	"testing"
)

func testSeed(fill byte) string {
	b:=make([]byte,ed25519.SeedSize)
	for i:=range b{b[i]=fill}
	return base64.RawURLEncoding.EncodeToString(b)
}

func TestValidateSigningSeedsAcceptsDistinctSeeds(t *testing.T){
	if err:=validateSigningSeeds(testSeed(1),testSeed(2),testSeed(3));err!=nil{
		t.Fatalf("unexpected error: %v",err)
	}
}

func TestValidateSigningSeedsRejectsDuplicateRoleKey(t *testing.T){
	seed:=testSeed(9)
	err:=validateSigningSeeds(seed,testSeed(2),seed)
	if err==nil||!strings.Contains(err.Error(),"must differ"){
		t.Fatalf("expected duplicate-key error, got %v",err)
	}
}

func TestValidateSigningSeedsRejectsInvalidSeedLength(t *testing.T){
	short:=base64.RawURLEncoding.EncodeToString(make([]byte,16))
	err:=validateSigningSeeds(short,testSeed(2),"")
	if err==nil||!strings.Contains(err.Error(),"exactly 32 bytes"){
		t.Fatalf("expected size error, got %v",err)
	}
}

func TestValidateSigningSeedsRejectsPaddedBase64(t *testing.T){
	padded:=base64.URLEncoding.EncodeToString(make([]byte,32))
	err:=validateSigningSeeds(padded,testSeed(2),"")
	if err==nil{
		t.Fatal("expected raw base64url validation error")
	}
}
