package artifactstorage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type S3Config struct {
	Endpoint string
	Region string
	Bucket string
	AccessKey string
	SecretKey string
	TempDir string
}

type S3 struct {
	endpoint *url.URL
	region string
	bucket string
	accessKey string
	secretKey string
	tempDir string
	client *http.Client
}

func NewS3(cfg S3Config)(*S3,error){
	endpoint,err:=url.Parse(strings.TrimSpace(cfg.Endpoint))
	if err!=nil||endpoint.Scheme==""||endpoint.Host==""{
		return nil,fmt.Errorf("invalid S3 endpoint")
	}
	if endpoint.Scheme!="https"{
		return nil,fmt.Errorf("S3 endpoint must use https")
	}
	region:=strings.TrimSpace(cfg.Region)
	bucket:=strings.TrimSpace(cfg.Bucket)
	access:=strings.TrimSpace(cfg.AccessKey)
	secret:=strings.TrimSpace(cfg.SecretKey)
	if region==""||bucket==""||access==""||secret==""{
		return nil,fmt.Errorf("S3 region, bucket, access key and secret key are required")
	}
	if strings.Contains(bucket,"/")||strings.Contains(bucket,".."){
		return nil,fmt.Errorf("invalid S3 bucket")
	}
	temp:=strings.TrimSpace(cfg.TempDir)
	if temp==""{temp=os.TempDir()}
	if err:=os.MkdirAll(temp,0750);err!=nil{return nil,err}
	return &S3{
		endpoint:endpoint,region:region,bucket:bucket,accessKey:access,secretKey:secret,tempDir:temp,
		client:&http.Client{},
	},nil
}

func cleanObjectKey(key string)(string,error){
	key=strings.Trim(strings.TrimSpace(key),"/")
	if key==""{return "",fmt.Errorf("invalid storage key")}
	parts:=strings.Split(key,"/")
	for _,p:=range parts{
		if p==""||p=="."||p==".."{return "",fmt.Errorf("invalid storage key")}
	}
	return strings.Join(parts,"/"),nil
}

func (s *S3) objectURL(key string)(*url.URL,error){
	key,err:=cleanObjectKey(key);if err!=nil{return nil,err}
	u:=*s.endpoint
	parts:=strings.Split(key,"/")
	escaped:=make([]string,0,len(parts)+1)
	escaped=append(escaped,url.PathEscape(s.bucket))
	for _,p:=range parts{escaped=append(escaped,url.PathEscape(p))}
	base:=strings.TrimSuffix(u.EscapedPath(),"/")
	u.RawPath=base+"/"+strings.Join(escaped,"/")
	decoded,err:=url.PathUnescape(u.RawPath);if err!=nil{return nil,err}
	u.Path=decoded
	u.RawQuery=""
	u.Fragment=""
	return &u,nil
}

func (s *S3) PutVerified(ctx context.Context,key string,src io.Reader,maxBytes int64,expectedSHA256 string)(ObjectInfo,error){
	if maxBytes<=0{return ObjectInfo{},fmt.Errorf("invalid artifact size limit")}
	expectedSHA256=strings.ToLower(strings.TrimSpace(expectedSHA256))
	if len(expectedSHA256)!=64{return ObjectInfo{},fmt.Errorf("invalid expected sha256")}
	if _,err:=hex.DecodeString(expectedSHA256);err!=nil{return ObjectInfo{},fmt.Errorf("invalid expected sha256")}

	tmp,err:=os.CreateTemp(s.tempDir,"vpnx3-s3-upload-*")
	if err!=nil{return ObjectInfo{},err}
	tmpPath:=tmp.Name()
	defer func(){_ = tmp.Close();_ = os.Remove(tmpPath)}()

	h:=sha256.New()
	written,err:=io.Copy(io.MultiWriter(tmp,h),io.LimitReader(src,maxBytes+1))
	if err!=nil{return ObjectInfo{},err}
	if written>maxBytes{return ObjectInfo{},fmt.Errorf("artifact exceeds size limit")}
	if err:=ctx.Err();err!=nil{return ObjectInfo{},err}
	if err:=tmp.Sync();err!=nil{return ObjectInfo{},err}
	actual:=hex.EncodeToString(h.Sum(nil))
	if actual!=expectedSHA256{return ObjectInfo{},fmt.Errorf("artifact hash mismatch")}
	if _,err:=tmp.Seek(0,io.SeekStart);err!=nil{return ObjectInfo{},err}

	u,err:=s.objectURL(key);if err!=nil{return ObjectInfo{},err}
	req,err:=http.NewRequestWithContext(ctx,http.MethodPut,u.String(),tmp)
	if err!=nil{return ObjectInfo{},err}
	req.ContentLength=written
	req.Header.Set("Content-Type","application/octet-stream")
	req.Header.Set("Content-Length",strconv.FormatInt(written,10))
	if err:=s.sign(req,expectedSHA256,time.Now().UTC());err!=nil{return ObjectInfo{},err}

	resp,err:=s.client.Do(req)
	if err!=nil{return ObjectInfo{},fmt.Errorf("S3 put: %w",err)}
	defer resp.Body.Close()
	if resp.StatusCode<200||resp.StatusCode>=300{
		body,_:=io.ReadAll(io.LimitReader(resp.Body,4096))
		return ObjectInfo{},fmt.Errorf("S3 put status %d: %s",resp.StatusCode,strings.TrimSpace(string(body)))
	}
	return ObjectInfo{Size:written,ModTime:time.Now().UTC()},nil
}

func (s *S3) Open(ctx context.Context,key string)(io.ReadCloser,ObjectInfo,error){
	u,err:=s.objectURL(key);if err!=nil{return nil,ObjectInfo{},err}
	req,err:=http.NewRequestWithContext(ctx,http.MethodGet,u.String(),nil)
	if err!=nil{return nil,ObjectInfo{},err}
	emptyHash:=sha256.Sum256(nil)
	if err:=s.sign(req,hex.EncodeToString(emptyHash[:]),time.Now().UTC());err!=nil{return nil,ObjectInfo{},err}
	resp,err:=s.client.Do(req)
	if err!=nil{return nil,ObjectInfo{},fmt.Errorf("S3 get: %w",err)}
	if resp.StatusCode==http.StatusNotFound{
		resp.Body.Close()
		return nil,ObjectInfo{},ErrNotFound
	}
	if resp.StatusCode<200||resp.StatusCode>=300{
		body,_:=io.ReadAll(io.LimitReader(resp.Body,4096));resp.Body.Close()
		return nil,ObjectInfo{},fmt.Errorf("S3 get status %d: %s",resp.StatusCode,strings.TrimSpace(string(body)))
	}
	mod:=time.Now().UTC()
	if raw:=resp.Header.Get("Last-Modified");raw!=""{
		if parsed,err:=http.ParseTime(raw);err==nil{mod=parsed}
	}
	return resp.Body,ObjectInfo{Size:resp.ContentLength,ModTime:mod},nil
}

func (s *S3) Delete(ctx context.Context,key string)error{
	u,err:=s.objectURL(key);if err!=nil{return err}
	req,err:=http.NewRequestWithContext(ctx,http.MethodDelete,u.String(),nil)
	if err!=nil{return err}
	emptyHash:=sha256.Sum256(nil)
	if err:=s.sign(req,hex.EncodeToString(emptyHash[:]),time.Now().UTC());err!=nil{return err}
	resp,err:=s.client.Do(req)
	if err!=nil{return fmt.Errorf("S3 delete: %w",err)}
	defer resp.Body.Close()
	if resp.StatusCode==http.StatusNotFound{return nil}
	if resp.StatusCode<200||resp.StatusCode>=300{
		body,_:=io.ReadAll(io.LimitReader(resp.Body,4096))
		return fmt.Errorf("S3 delete status %d: %s",resp.StatusCode,strings.TrimSpace(string(body)))
	}
	return nil
}

func (s *S3) CleanupTemporary(ctx context.Context,olderThan time.Time)error{
	if err:=ctx.Err();err!=nil{return err}
	entries,err:=os.ReadDir(s.tempDir);if err!=nil{return err}
	for _,entry:=range entries{
		if entry.IsDir()||!strings.HasPrefix(entry.Name(),"vpnx3-s3-upload-"){continue}
		info,err:=entry.Info();if err!=nil{continue}
		if info.ModTime().Before(olderThan){_ = os.Remove(filepath.Join(s.tempDir,entry.Name()))}
	}
	return nil
}

func (s *S3) sign(req *http.Request,payloadHash string,now time.Time)error{
	if req.URL==nil{return fmt.Errorf("missing S3 request URL")}
	amzDate:=now.UTC().Format("20060102T150405Z")
	shortDate:=now.UTC().Format("20060102")
	req.Header.Set("X-Amz-Date",amzDate)
	req.Header.Set("X-Amz-Content-Sha256",payloadHash)

	canonicalURI:=req.URL.EscapedPath()
	if canonicalURI==""{canonicalURI="/"}
	canonicalQuery:=req.URL.Query().Encode()
	host:=req.URL.Host
	canonicalHeaders:="host:"+host+"\n"+"x-amz-content-sha256:"+payloadHash+"\n"+"x-amz-date:"+amzDate+"\n"
	signedHeaders:="host;x-amz-content-sha256;x-amz-date"
	canonicalRequest:=strings.Join([]string{
		req.Method,canonicalURI,canonicalQuery,canonicalHeaders,signedHeaders,payloadHash,
	},"\n")
	sum:=sha256.Sum256([]byte(canonicalRequest))
	scope:=shortDate+"/"+s.region+"/s3/aws4_request"
	stringToSign:="AWS4-HMAC-SHA256\n"+amzDate+"\n"+scope+"\n"+hex.EncodeToString(sum[:])

	kDate:=hmacSHA256([]byte("AWS4"+s.secretKey),[]byte(shortDate))
	kRegion:=hmacSHA256(kDate,[]byte(s.region))
	kService:=hmacSHA256(kRegion,[]byte("s3"))
	kSigning:=hmacSHA256(kService,[]byte("aws4_request"))
	signature:=hex.EncodeToString(hmacSHA256(kSigning,[]byte(stringToSign)))
	req.Header.Set("Authorization","AWS4-HMAC-SHA256 Credential="+s.accessKey+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)
	return nil
}

func hmacSHA256(key,data []byte)[]byte{
	mac:=hmac.New(sha256.New,key)
	_,_=mac.Write(data)
	return mac.Sum(nil)
}

var _ Storage = (*S3)(nil)

func (s *S3) Check(ctx context.Context) error {
	u,err:=s.objectURL("__vpnx3/readiness/nonexistent")
	if err!=nil{return err}
	req,err:=http.NewRequestWithContext(ctx,http.MethodHead,u.String(),nil)
	if err!=nil{return err}
	emptyHash:=sha256.Sum256(nil)
	if err:=s.sign(req,hex.EncodeToString(emptyHash[:]),time.Now().UTC());err!=nil{return err}
	resp,err:=s.client.Do(req)
	if err!=nil{return fmt.Errorf("S3 readiness request failed: %w",err)}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode==http.StatusNotFound:
		return nil
	case resp.StatusCode>=200&&resp.StatusCode<300:
		return nil
	default:
		return fmt.Errorf("S3 readiness status %d",resp.StatusCode)
	}
}

var _ ReadinessChecker = (*S3)(nil)
