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
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/artifactstorage"
)

func main(){
	if err:=run();err!=nil{log.Fatal(err)}
}

func run()error{
	sourceDir:=strings.TrimSpace(os.Getenv("VPNX3_BACKUP_SOURCE_DIR"))
	statusFile:=strings.TrimSpace(os.Getenv("VPNX3_BACKUP_OFFSITE_STATUS_FILE"))
	if sourceDir==""||statusFile==""{return fmt.Errorf("VPNX3_BACKUP_SOURCE_DIR and VPNX3_BACKUP_OFFSITE_STATUS_FILE are required")}
	if !filepath.IsAbs(sourceDir)||!filepath.IsAbs(statusFile){return fmt.Errorf("backup source/status paths must be absolute")}

	s3,err:=artifactstorage.NewS3(artifactstorage.S3Config{
		Endpoint:os.Getenv("VPNX3_BACKUP_S3_ENDPOINT"),
		Region:os.Getenv("VPNX3_BACKUP_S3_REGION"),
		Bucket:os.Getenv("VPNX3_BACKUP_S3_BUCKET"),
		AccessKey:os.Getenv("VPNX3_BACKUP_S3_ACCESS_KEY"),
		SecretKey:os.Getenv("VPNX3_BACKUP_S3_SECRET_KEY"),
		TempDir:envDefault("VPNX3_BACKUP_S3_TEMP_DIR","/var/lib/vpnx3/backup-s3tmp"),
	})
	if err!=nil{return err}

	backup,sidecar,err:=latestCompletedBackup(sourceDir)
	if err!=nil{return err}
	expected,err:=readSidecar(sidecar,filepath.Base(backup))
	if err!=nil{return err}
	size,actual,err:=hashFile(backup)
	if err!=nil{return err}
	if actual!=expected{return fmt.Errorf("local backup sha256 mismatch")}

	prefix:=strings.Trim(strings.TrimSpace(envDefault("VPNX3_BACKUP_S3_PREFIX","control-backups")),"/")
	if prefix==""||strings.Contains(prefix,".."){return fmt.Errorf("invalid backup S3 prefix")}
	key:=prefix+"/"+filepath.Base(backup)

	ctx,cancel:=context.WithTimeout(context.Background(),30*time.Minute)
	defer cancel()

	if existing,_,openErr:=s3.Open(ctx,key);openErr==nil{
		remoteHash,hashErr:=hashReader(existing)
		_ = existing.Close()
		if hashErr!=nil{return hashErr}
		if remoteHash!=actual{return fmt.Errorf("immutable remote object exists with different sha256: %s",key)}
		return writeStatus(statusFile,key,actual,size)
	}else if !errors.Is(openErr,artifactstorage.ErrNotFound){
		return fmt.Errorf("check remote backup: %w",openErr)
	}

	f,err:=os.Open(backup);if err!=nil{return err}
	_,putErr:=s3.PutVerified(ctx,key,f,size,actual)
	_ = f.Close()
	if putErr!=nil{return fmt.Errorf("upload offsite backup: %w",putErr)}

	remote,info,err:=s3.Open(ctx,key)
	if err!=nil{return fmt.Errorf("verify remote backup open: %w",err)}
	remoteHash,err:=hashReader(remote)
	_ = remote.Close()
	if err!=nil{return err}
	if remoteHash!=actual{return fmt.Errorf("remote backup sha256 mismatch")}
	if info.Size>=0&&info.Size!=size{return fmt.Errorf("remote backup size mismatch")}
	return writeStatus(statusFile,key,actual,size)
}

func latestCompletedBackup(dir string)(string,string,error){
	entries,err:=os.ReadDir(dir);if err!=nil{return "","",err}
	names:=make([]string,0)
	for _,entry:=range entries{
		if entry.IsDir(){continue}
		name:=entry.Name()
		if strings.HasPrefix(name,"vpnx3-")&&strings.HasSuffix(name,".tar.age"){
			if _,err:=os.Stat(filepath.Join(dir,name+".sha256"));err==nil{names=append(names,name)}
		}
	}
	if len(names)==0{return "","",fmt.Errorf("no completed backup bundles found")}
	sort.Strings(names)
	name:=names[len(names)-1]
	return filepath.Join(dir,name),filepath.Join(dir,name+".sha256"),nil
}

func readSidecar(path,expectedName string)(string,error){
	f,err:=os.Open(path);if err!=nil{return "",err};defer f.Close()
	sc:=bufio.NewScanner(io.LimitReader(f,4096))
	if !sc.Scan(){return "",fmt.Errorf("empty sha256 sidecar")}
	fields:=strings.Fields(sc.Text())
	if len(fields)<1||len(fields[0])!=64{return "",fmt.Errorf("invalid sha256 sidecar")}
	if _,err:=hex.DecodeString(fields[0]);err!=nil{return "",fmt.Errorf("invalid sha256 sidecar")}
	if len(fields)>=2{
		name:=strings.TrimPrefix(fields[1],"*")
		if filepath.Base(name)!=expectedName{return "",fmt.Errorf("sha256 sidecar filename mismatch")}
	}
	return strings.ToLower(fields[0]),nil
}

func hashFile(path string)(int64,string,error){
	f,err:=os.Open(path);if err!=nil{return 0,"",err};defer f.Close()
	h:=sha256.New()
	n,err:=io.Copy(h,f);if err!=nil{return 0,"",err}
	return n,hex.EncodeToString(h.Sum(nil)),nil
}

func hashReader(r io.Reader)(string,error){
	h:=sha256.New()
	if _,err:=io.Copy(h,r);err!=nil{return "",err}
	return hex.EncodeToString(h.Sum(nil)),nil
}

func writeStatus(path,key,sha string,size int64)error{
	if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil{return err}
	tmp:=path+".tmp"
	payload:=fmt.Sprintf("verified_at=%s\nobject=%s\nsha256=%s\nbytes=%d\n",time.Now().UTC().Format(time.RFC3339),key,sha,size)
	if err:=os.WriteFile(tmp,[]byte(payload),0600);err!=nil{return err}
	return os.Rename(tmp,path)
}

func envDefault(key,def string)string{
	if v:=strings.TrimSpace(os.Getenv(key));v!=""{return v}
	return def
}
