package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBodyLimitRejectsKnownOversize(t *testing.T){
	handler:=bodyLimitMiddleware(16,64,http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		t.Fatal("oversized request reached handler")
	}))
	req:=httptest.NewRequest(http.MethodPost,"/api/v1/client/register",strings.NewReader(strings.Repeat("x",17)))
	rec:=httptest.NewRecorder()
	handler.ServeHTTP(rec,req)
	if rec.Code!=http.StatusRequestEntityTooLarge{t.Fatalf("status=%d",rec.Code)}
}

func TestBodyLimitAllowsLargerArtifactRoute(t *testing.T){
	handler:=bodyLimitMiddleware(16,64,http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		buf:=make([]byte,32)
		if _,err:=r.Body.Read(buf);err!=nil{t.Fatalf("read body: %v",err)}
		w.WriteHeader(http.StatusNoContent)
	}))
	req:=httptest.NewRequest(http.MethodPut,"/api/v1/build/jobs/job-1/artifact",strings.NewReader(strings.Repeat("x",32)))
	rec:=httptest.NewRecorder()
	handler.ServeHTTP(rec,req)
	if rec.Code!=http.StatusNoContent{t.Fatalf("status=%d",rec.Code)}
}
