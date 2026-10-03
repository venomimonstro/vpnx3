package artifactstorage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ObjectInfo struct {
	Size int64
	ModTime time.Time
}

type Storage interface {
	PutVerified(context.Context,string,io.Reader,int64,string)(ObjectInfo,error)
	Open(context.Context,string)(io.ReadCloser,ObjectInfo,error)
	Delete(context.Context,string) error
	CleanupTemporary(context.Context,time.Time) error
}

type Local struct {
	root string
}

func NewLocal(root string)(*Local,error){
	abs,err:=filepath.Abs(strings.TrimSpace(root))
	if err!=nil{return nil,err}
	if abs==""||abs=="."{return nil,fmt.Errorf("invalid artifact storage root")}
	if err:=os.MkdirAll(abs,0750);err!=nil{return nil,err}
	return &Local{root:abs},nil
}

func (l *Local) safePath(key string)(string,error){
	key=filepath.FromSlash(strings.TrimSpace(key))
	if key==""||filepath.IsAbs(key){return "",fmt.Errorf("invalid storage key")}
	full,err:=filepath.Abs(filepath.Join(l.root,key))
	if err!=nil{return "",err}
	rel,err:=filepath.Rel(l.root,full)
	if err!=nil||rel==".."||strings.HasPrefix(rel,".."+string(os.PathSeparator)){
		return "",fmt.Errorf("storage key escaped root")
	}
	return full,nil
}

func (l *Local) PutVerified(ctx context.Context,key string,src io.Reader,maxBytes int64,expectedSHA256 string)(ObjectInfo,error){
	if maxBytes<=0{return ObjectInfo{},fmt.Errorf("invalid artifact size limit")}
	expectedSHA256=strings.ToLower(strings.TrimSpace(expectedSHA256))
	if len(expectedSHA256)!=64{return ObjectInfo{},fmt.Errorf("invalid expected sha256")}
	if _,err:=hex.DecodeString(expectedSHA256);err!=nil{return ObjectInfo{},fmt.Errorf("invalid expected sha256")}

	finalPath,err:=l.safePath(key);if err!=nil{return ObjectInfo{},err}
	dir:=filepath.Dir(finalPath)
	if err:=os.MkdirAll(dir,0750);err!=nil{return ObjectInfo{},err}

	tmp,err:=os.CreateTemp(dir,"upload-*")
	if err!=nil{return ObjectInfo{},err}
	tmpPath:=tmp.Name()
	committed:=false
	defer func(){
		_ = tmp.Close()
		if !committed{_ = os.Remove(tmpPath)}
	}()

	h:=sha256.New()
	limited:=io.LimitReader(src,maxBytes+1)
	written,err:=io.Copy(io.MultiWriter(tmp,h),limited)
	if err!=nil{return ObjectInfo{},err}
	if written>maxBytes{return ObjectInfo{},fmt.Errorf("artifact exceeds size limit")}
	if err:=ctx.Err();err!=nil{return ObjectInfo{},err}
	if err:=tmp.Sync();err!=nil{return ObjectInfo{},err}
	if err:=tmp.Close();err!=nil{return ObjectInfo{},err}

	actual:=hex.EncodeToString(h.Sum(nil))
	if actual!=expectedSHA256{return ObjectInfo{},fmt.Errorf("artifact hash mismatch")}
	if err:=os.Chmod(tmpPath,0640);err!=nil{return ObjectInfo{},err}
	if err:=os.Rename(tmpPath,finalPath);err!=nil{return ObjectInfo{},err}
	committed=true

	stat,err:=os.Stat(finalPath);if err!=nil{return ObjectInfo{},err}
	return ObjectInfo{Size:stat.Size(),ModTime:stat.ModTime()},nil
}

func (l *Local) Open(ctx context.Context,key string)(io.ReadCloser,ObjectInfo,error){
	if err:=ctx.Err();err!=nil{return nil,ObjectInfo{},err}
	full,err:=l.safePath(key);if err!=nil{return nil,ObjectInfo{},err}
	f,err:=os.Open(full);if err!=nil{return nil,ObjectInfo{},err}
	stat,err:=f.Stat()
	if err!=nil{f.Close();return nil,ObjectInfo{},err}
	return f,ObjectInfo{Size:stat.Size(),ModTime:stat.ModTime()},nil
}

func (l *Local) Delete(ctx context.Context,key string) error {
	if err:=ctx.Err();err!=nil{return err}
	full,err:=l.safePath(key);if err!=nil{return err}
	err=os.Remove(full)
	if os.IsNotExist(err){return nil}
	return err
}

func (l *Local) CleanupTemporary(ctx context.Context,olderThan time.Time) error {
	return filepath.WalkDir(l.root,func(path string,d os.DirEntry,walkErr error)error{
		if walkErr!=nil{return nil}
		if err:=ctx.Err();err!=nil{return err}
		if d==nil||d.IsDir()||!strings.HasPrefix(d.Name(),"upload-"){return nil}
		info,err:=d.Info();if err!=nil{return nil}
		if info.ModTime().Before(olderThan){_ = os.Remove(path)}
		return nil
	})
}
