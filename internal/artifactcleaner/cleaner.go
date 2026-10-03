package artifactcleaner

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/venomimonstro/vpnx3/internal/store"
)

type Cleaner struct {
	store *store.Store
	logger *slog.Logger
	root string
	retention time.Duration
}

func New(s *store.Store,l *slog.Logger,root string,days int)*Cleaner{
	return &Cleaner{store:s,logger:l,root:root,retention:time.Duration(days)*24*time.Hour}
}

func (c *Cleaner) Run(ctx context.Context){
	c.cleanup(ctx)
	t:=time.NewTicker(6*time.Hour);defer t.Stop()
	for{
		select{
		case <-ctx.Done():return
		case <-t.C:c.cleanup(ctx)
		}
	}
}

func (c *Cleaner) cleanup(parent context.Context){
	ctx,cancel:=context.WithTimeout(parent,2*time.Minute);defer cancel()
	root,err:=filepath.Abs(c.root)
	if err!=nil{c.logger.Warn("artifact retention root invalid","error",err);return}

	candidates,err:=c.store.WithdrawnArtifactsForRetention(ctx,time.Now().UTC().Add(-c.retention),500)
	if err!=nil{c.logger.Warn("artifact retention query failed","error",err);return}
	for _,a:=range candidates{
		full,err:=filepath.Abs(filepath.Join(root,filepath.FromSlash(a.StorageKey)))
		if err!=nil{continue}
		rel,err:=filepath.Rel(root,full)
		if err!=nil||rel==".."||strings.HasPrefix(rel,".."+string(os.PathSeparator)){
			c.logger.Error("artifact retention path escaped root","artifact_id",a.ID)
			continue
		}
		if err:=os.Remove(full);err!=nil && !os.IsNotExist(err){
			c.logger.Warn("artifact retention remove failed","artifact_id",a.ID,"error",err)
			continue
		}
		if err:=c.store.DeleteRetainedArtifact(ctx,a.ID);err!=nil{
			c.logger.Warn("artifact retention db cleanup failed","artifact_id",a.ID,"error",err)
		}
	}

	// Temporary upload files are never referenced by the database.
	_ = filepath.WalkDir(root,func(path string,d os.DirEntry,walkErr error)error{
		if walkErr!=nil||d==nil||d.IsDir(){return nil}
		if !strings.HasPrefix(d.Name(),"upload-"){return nil}
		info,err:=d.Info();if err!=nil{return nil}
		if info.ModTime().Before(time.Now().Add(-24*time.Hour)){_ = os.Remove(path)}
		return nil
	})
}
