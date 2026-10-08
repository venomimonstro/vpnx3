package httpapi

import (
	"net/http"
	"sync/atomic"
	"time"
)

type httpMetrics struct{
	active atomic.Int64
	total atomic.Int64
	status2xx atomic.Int64
	status3xx atomic.Int64
	status4xx atomic.Int64
	status5xx atomic.Int64
	bytesOut atomic.Int64
	lt50 atomic.Int64
	lt100 atomic.Int64
	lt250 atomic.Int64
	lt500 atomic.Int64
	lt1000 atomic.Int64
	lt3000 atomic.Int64
	ge3000 atomic.Int64
}

type httpMetricsSnapshot struct{
	Active int64 `json:"active"`
	Total int64 `json:"total"`
	Status2xx int64 `json:"status_2xx"`
	Status3xx int64 `json:"status_3xx"`
	Status4xx int64 `json:"status_4xx"`
	Status5xx int64 `json:"status_5xx"`
	BytesOut int64 `json:"bytes_out"`
	Latency map[string]int64 `json:"latency_buckets_ms"`
}

func (m *httpMetrics) observe(status int,bytes int64,d time.Duration){
	m.total.Add(1)
	m.bytesOut.Add(bytes)
	switch{
	case status>=200&&status<300:m.status2xx.Add(1)
	case status>=300&&status<400:m.status3xx.Add(1)
	case status>=400&&status<500:m.status4xx.Add(1)
	case status>=500:m.status5xx.Add(1)
	}
	ms:=d.Milliseconds()
	switch{
	case ms<50:m.lt50.Add(1)
	case ms<100:m.lt100.Add(1)
	case ms<250:m.lt250.Add(1)
	case ms<500:m.lt500.Add(1)
	case ms<1000:m.lt1000.Add(1)
	case ms<3000:m.lt3000.Add(1)
	default:m.ge3000.Add(1)
	}
}

func (m *httpMetrics) snapshot()httpMetricsSnapshot{
	return httpMetricsSnapshot{
		Active:m.active.Load(),Total:m.total.Load(),
		Status2xx:m.status2xx.Load(),Status3xx:m.status3xx.Load(),
		Status4xx:m.status4xx.Load(),Status5xx:m.status5xx.Load(),
		BytesOut:m.bytesOut.Load(),
		Latency:map[string]int64{
			"<50":m.lt50.Load(),"<100":m.lt100.Load(),"<250":m.lt250.Load(),
			"<500":m.lt500.Load(),"<1000":m.lt1000.Load(),"<3000":m.lt3000.Load(),
			">=3000":m.ge3000.Load(),
		},
	}
}

type captureWriter struct{
	http.ResponseWriter
	status int
	bytes int64
}

func (w *captureWriter) WriteHeader(status int){
	if w.status==0{w.status=status}
	w.ResponseWriter.WriteHeader(status)
}

func (w *captureWriter) Write(p []byte)(int,error){
	if w.status==0{w.status=http.StatusOK}
	n,err:=w.ResponseWriter.Write(p)
	w.bytes+=int64(n)
	return n,err
}

func (w *captureWriter) Unwrap()http.ResponseWriter{return w.ResponseWriter}

func httpMetricsMiddleware(m *httpMetrics,next http.Handler)http.Handler{
	return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		m.active.Add(1)
		defer m.active.Add(-1)
		started:=time.Now()
		cw:=&captureWriter{ResponseWriter:w}
		next.ServeHTTP(cw,r)
		if cw.status==0{cw.status=http.StatusOK}
		m.observe(cw.status,cw.bytes,time.Since(started))
	})
}

func (s *Server) handleHTTPMetrics(w http.ResponseWriter,r *http.Request){
	writeJSON(w,http.StatusOK,map[string]any{
		"scope":"this_control_plane_replica",
		"since_process_start":s.httpMetrics.snapshot(),
	})
}
