package httpapi

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestAdmissionRejectsWhenFull(t *testing.T){
	controller:=newAdmissionController(1)
	started:=make(chan struct{})
	release:=make(chan struct{})
	var once sync.Once
	handler:=controller.wrap(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		once.Do(func(){close(started)})
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))

	done:=make(chan struct{})
	go func(){
		defer close(done)
		req:=httptest.NewRequest(http.MethodGet,"/api/v1/test",nil)
		rec:=httptest.NewRecorder()
		handler.ServeHTTP(rec,req)
		if rec.Code!=http.StatusNoContent{t.Errorf("first request status=%d",rec.Code)}
	}()
	<-started

	req:=httptest.NewRequest(http.MethodGet,"/api/v1/test",nil)
	rec:=httptest.NewRecorder()
	handler.ServeHTTP(rec,req)
	if rec.Code!=http.StatusServiceUnavailable{t.Fatalf("second status=%d",rec.Code)}
	if rec.Header().Get("Retry-After")!="1"{t.Fatalf("missing retry-after")}
	close(release)
	<-done

	current,limit,peak,rejected:=controller.snapshot()
	if current!=0||limit!=1||peak!=1||rejected!=1{
		t.Fatalf("unexpected snapshot %d %d %d %d",current,limit,peak,rejected)
	}
}

func TestAdmissionNeverBlocksHealth(t *testing.T){
	controller:=newAdmissionController(1)
	controller.slots<-struct{}{}
	controller.current.Store(1)
	defer func(){controller.current.Store(0);<-controller.slots}()

	handler:=controller.wrap(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		w.WriteHeader(http.StatusNoContent)
	}))
	for _,path:=range []string{"/health/live","/health/ready"}{
		req:=httptest.NewRequest(http.MethodGet,path,nil)
		rec:=httptest.NewRecorder()
		handler.ServeHTTP(rec,req)
		if rec.Code!=http.StatusNoContent{t.Fatalf("%s status=%d",path,rec.Code)}
	}
}
