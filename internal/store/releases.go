package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type ReleaseArtifact struct {
	ID        string    `json:"id"`
	ReleaseID string    `json:"release_id"`
	BuildJobID string   `json:"build_job_id"`
	Target    string    `json:"target"`
	FileName  string    `json:"file_name"`
	StorageKey string   `json:"-"`
	SHA256    string    `json:"sha256"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) ReleaseArtifacts(ctx context.Context,releaseID string) ([]ReleaseArtifact,error) {
	rows,err:=s.DB.Query(ctx,`
		SELECT id::text,release_id::text,build_job_id::text,target,file_name,storage_key,sha256,size_bytes,created_at
		FROM release_artifacts WHERE release_id=$1 ORDER BY target
	`,releaseID)
	if err!=nil{return nil,err}
	defer rows.Close()
	out:=make([]ReleaseArtifact,0)
	for rows.Next(){
		var a ReleaseArtifact
		if err:=rows.Scan(&a.ID,&a.ReleaseID,&a.BuildJobID,&a.Target,&a.FileName,&a.StorageKey,&a.SHA256,&a.SizeBytes,&a.CreatedAt);err!=nil{return nil,err}
		out=append(out,a)
	}
	return out,rows.Err()
}

func (s *Store) ReleaseArtifactByID(ctx context.Context,releaseID,artifactID string) (ReleaseArtifact,error) {
	var a ReleaseArtifact
	err:=s.DB.QueryRow(ctx,`
		SELECT id::text,release_id::text,build_job_id::text,target,file_name,storage_key,sha256,size_bytes,created_at
		FROM release_artifacts WHERE id=$1 AND release_id=$2
	`,artifactID,releaseID).Scan(&a.ID,&a.ReleaseID,&a.BuildJobID,&a.Target,&a.FileName,&a.StorageKey,&a.SHA256,&a.SizeBytes,&a.CreatedAt)
	if errors.Is(err,pgx.ErrNoRows){return ReleaseArtifact{},ErrNotFound}
	return a,err
}

func (s *Store) PublishRelease(ctx context.Context,releaseID string) (Release,error) {
	tx,err:=s.DB.Begin(ctx);if err!=nil{return Release{},err};defer tx.Rollback(ctx)
	var status string
	if err:=tx.QueryRow(ctx,"SELECT status FROM releases WHERE id=$1 FOR UPDATE",releaseID).Scan(&status);err!=nil{
		if errors.Is(err,pgx.ErrNoRows){return Release{},ErrNotFound};return Release{},err
	}
	if status!="ready"{return Release{},fmt.Errorf("release is not ready")}

	var jobs,success,artifacts int
	if err:=tx.QueryRow(ctx,`
		SELECT count(*)::int,count(*) FILTER(WHERE status='succeeded')::int
		FROM build_jobs WHERE release_id=$1
	`,releaseID).Scan(&jobs,&success);err!=nil{return Release{},err}
	if err:=tx.QueryRow(ctx,"SELECT count(*)::int FROM release_artifacts WHERE release_id=$1",releaseID).Scan(&artifacts);err!=nil{return Release{},err}
	if jobs==0 || jobs!=success || jobs!=artifacts{
		return Release{},fmt.Errorf("release artifacts are incomplete")
	}

	if _,err:=tx.Exec(ctx,`
		UPDATE releases SET status='published',published_at=now(),updated_at=now() WHERE id=$1
	`,releaseID);err!=nil{return Release{},err}
	if err:=tx.Commit(ctx);err!=nil{return Release{},err}
	return s.releaseByID(ctx,releaseID)
}

func (s *Store) WithdrawRelease(ctx context.Context,releaseID string) (Release,error) {
	tag,err:=s.DB.Exec(ctx,`
		UPDATE releases SET status='withdrawn',updated_at=now()
		WHERE id=$1 AND status='published'
	`,releaseID)
	if err!=nil{return Release{},err}
	if tag.RowsAffected()!=1{return Release{},fmt.Errorf("release is not published")}
	return s.releaseByID(ctx,releaseID)
}

func (s *Store) releaseByID(ctx context.Context,id string)(Release,error){
	var r Release
	err:=s.DB.QueryRow(ctx,`
		SELECT id::text,version,source_commit,status,notes,created_at,updated_at,published_at
		FROM releases WHERE id=$1
	`,id).Scan(&r.ID,&r.Version,&r.SourceCommit,&r.Status,&r.Notes,&r.CreatedAt,&r.UpdatedAt,&r.PublishedAt)
	if errors.Is(err,pgx.ErrNoRows){return Release{},ErrNotFound}
	return r,err
}
