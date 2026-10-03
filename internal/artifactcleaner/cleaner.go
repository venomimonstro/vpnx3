package artifactcleaner

import (
	"context"
	"log/slog"
	"time"

	"github.com/venomimonstro/vpnx3/internal/artifactstorage"
	"github.com/venomimonstro/vpnx3/internal/store"
)

type Cleaner struct {
	store *store.Store
	logger *slog.Logger
	artifacts artifactstorage.Storage
	retention time.Duration
}

func New(s *store.Store,l *slog.Logger,artifacts artifactstorage.Storage,days int)*Cleaner{
	return &Cleaner{
		store:s,logger:l,artifacts:artifacts,
		retention:time.Duration(days)*24*time.Hour,
	}
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

	candidates,err:=c.store.WithdrawnArtifactsForRetention(
		ctx,time.Now().UTC().Add(-c.retention),500,
	)
	if err!=nil{c.logger.Warn("artifact retention query failed","error",err);return}
	for _,a:=range candidates{
		if err:=c.artifacts.Delete(ctx,a.StorageKey);err!=nil{
			c.logger.Warn("artifact retention remove failed","artifact_id",a.ID,"error",err)
			continue
		}
		if err:=c.store.DeleteRetainedArtifact(ctx,a.ID);err!=nil{
			c.logger.Warn("artifact retention db cleanup failed","artifact_id",a.ID,"error",err)
		}
	}

	if err:=c.artifacts.CleanupTemporary(ctx,time.Now().Add(-24*time.Hour));err!=nil{
		c.logger.Warn("temporary artifact cleanup failed","error",err)
	}
}
