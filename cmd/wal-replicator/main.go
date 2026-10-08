package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/artifactstorage"
)

func main(){if err:=run();err!=nil{log.Fatal(err)}}

func run()error{
	source:=strings.TrimSpace(os.Getenv("VPNX3_WAL_ARCHIVE_DIR"))
	status:=strings.TrimSpace(os.Getenv("VPNX3_WAL_OFFSITE_STATUS_FILE"))
	if !filepath.IsAbs(source)||!filepath.IsAbs(status){return fmt.Errorf("WAL archive/status paths must be absolute")}
	maxFiles:=intEnv("VPNX3_WAL_REPLICATE_MAX_FILES",128,1,1000)
	retention:=intEnv("VPNX3_WAL_LOCAL_RETENTION_DAYS",7,2,365)
	prefix:=strings.Trim(strings.TrimSpace(envDefault("VPNX3_WAL_S3_PREFIX","postgres-wal")),"/")
	if prefix==""||strings.Contains(prefix,".."){return fmt.Errorf("invalid WAL S3 prefix")}

	s3,err:=artifactstorage.NewS3(artifactstorage.S3Config{
		Endpoint:os.Getenv("VPNX3_WAL_S3_ENDPOINT"),
		Region:os.Getenv("VPNX3_WAL_S3_REGION"),
		Bucket:os.Getenv("VPNX3_WAL_S3_BUCKET"),
		AccessKey:os.Getenv("VPNX3_WAL_S3_ACCESS_KEY"),
		SecretKey:os.Getenv("VPNX3_WAL_S3_SECRET_KEY"),
		TempDir:envDefault("VPNX3_WAL_S3_TEMP_DIR","/var/lib/vpnx3/wal-s3tmp"),
	})
	if err!=nil{return err}

	files,err:=completed(source)
	if err!=nil{return err}
	if len(files)==0{return fmt.Errorf("no completed encrypted WAL files")}
	latestLocal:=filepath.Base(files[len(files)-1])
	totalFiles:=len(files)
	if len(files)>maxFiles{files=files[:maxFiles]}

	ctx,cancel:=context.WithTimeout(context.Background(),20*time.Minute)
	defer cancel()
	verified:=0
	newest:=""
	for _,path:=range files{
		side:=path+".sha256"
		expected,err:=readSidecar(side,filepath.Base(path));if err!=nil{return err}
		size,actual,err:=hashFile(path);if err!=nil{return err}
		if actual!=expected{return fmt.Errorf("local WAL ciphertext hash mismatch: %s",filepath.Base(path))}
		key:=prefix+"/"+filepath.Base(path)

		if remote,_,openErr:=s3.Open(ctx,key);openErr==nil{
			remoteHash,hashErr:=hashReader(remote);_ = remote.Close()
			if hashErr!=nil{return hashErr}
			if remoteHash!=actual{return fmt.Errorf("remote WAL object differs: %s",key)}
		}else if errors.Is(openErr,artifactstorage.ErrNotFound){
			f,openFileErr:=os.Open(path);if openFileErr!=nil{return openFileErr}
			_,putErr:=s3.PutVerified(ctx,key,f,size,actual);_ = f.Close()
			if putErr!=nil{return fmt.Errorf("upload WAL %s: %w",filepath.Base(path),putErr)}
			remote,_,verifyErr:=s3.Open(ctx,key)
			if verifyErr!=nil{return verifyErr}
			remoteHash,hashErr:=hashReader(remote);_ = remote.Close()
			if hashErr!=nil{return hashErr}
			if remoteHash!=actual{return fmt.Errorf("remote WAL verification failed: %s",key)}
		}else{
			return openErr
		}
		verified++
		newest=filepath.Base(path)
		if info,statErr:=os.Stat(path);statErr==nil&&info.ModTime().Before(time.Now().Add(-time.Duration(retention)*24*time.Hour)){
			_ = os.Remove(path)
			_ = os.Remove(side)
		}
	}
	remaining:=totalFiles-verified
	if remaining<0{remaining=0}
	return writeStatus(status,latestLocal,newest,verified,remaining)
}

func completed(dir string)([]string,error){
	entries,err:=os.ReadDir(dir);if err!=nil{return nil,err}
	out:=make([]string,0)
	for _,entry:=range entries{
		if entry.IsDir()||!strings.HasSuffix(entry.Name(),".age"){continue}
		path:=filepath.Join(dir,entry.Name())
		if _,err:=os.Stat(path+".sha256");err==nil{out=append(out,path)}
	}
	sort.Strings(out)
	return out,nil
}

func readSidecar(path,name string)(string,error){
	f,err:=os.Open(path);if err!=nil{return "",err};defer f.Close()
	sc:=bufio.NewScanner(io.LimitReader(f,4096))
	if !sc.Scan(){return "",fmt.Errorf("empty WAL sha256 sidecar")}
	fields:=strings.Fields(sc.Text())
	if len(fields)<1||len(fields[0])!=64{return "",fmt.Errorf("invalid WAL sha256")}
	if _,err:=hex.DecodeString(fields[0]);err!=nil{return "",err}
	if len(fields)>=2&&filepath.Base(strings.TrimPrefix(fields[1],"*"))!=name{return "",fmt.Errorf("WAL sidecar filename mismatch")}
	return strings.ToLower(fields[0]),nil
}

func hashFile(path string)(int64,string,error){
	f,err:=os.Open(path);if err!=nil{return 0,"",err};defer f.Close()
	h:=sha256.New();n,err:=io.Copy(h,f);if err!=nil{return 0,"",err}
	return n,hex.EncodeToString(h.Sum(nil)),nil
}
func hashReader(r io.Reader)(string,error){
	h:=sha256.New();_,err:=io.Copy(h,r);if err!=nil{return "",err}
	return hex.EncodeToString(h.Sum(nil)),nil
}
func writeStatus(path,latestLocal,newestVerified string,count,backlog int)error{
	if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil{return err}
	tmp:=path+".tmp"
	body:=fmt.Sprintf(
		"verified_at=%s\nlatest_local=%s\nnewest_verified=%s\nverified_files=%d\nbacklog_files=%d\n",
		time.Now().UTC().Format(time.RFC3339),latestLocal,newestVerified,count,backlog,
	)
	if err:=os.WriteFile(tmp,[]byte(body),0600);err!=nil{return err}
	return os.Rename(tmp,path)
}
func envDefault(k,d string)string{if v:=strings.TrimSpace(os.Getenv(k));v!=""{return v};return d}
func intEnv(k string,d,min,max int)int{
	raw:=strings.TrimSpace(os.Getenv(k));if raw==""{return d}
	n,err:=strconv.Atoi(raw);if err!=nil||n<min||n>max{log.Fatalf("%s must be %d..%d",k,min,max)}
	return n
}
