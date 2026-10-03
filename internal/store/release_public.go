package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type PublishedArtifact struct {
	ReleaseID string
	Version string
	ArtifactID string
	Target string
	FileName string
	StorageKey string
	SHA256 string
	SizeBytes int64
}

func (s *Store) LatestPublishedArtifact(ctx context.Context,target string) (PublishedArtifact,error) {
	var a PublishedArtifact
	err:=s.DB.QueryRow(ctx,`
		SELECT r.id::text,r.version,a.id::text,a.target,a.file_name,a.storage_key,a.sha256,a.size_bytes
		FROM releases r
		JOIN release_artifacts a ON a.release_id=r.id
		WHERE r.status='published' AND a.target=$1
		ORDER BY r.published_at DESC
		LIMIT 1
	`,target).Scan(&a.ReleaseID,&a.Version,&a.ArtifactID,&a.Target,&a.FileName,&a.StorageKey,&a.SHA256,&a.SizeBytes)
	if errors.Is(err,pgx.ErrNoRows){return PublishedArtifact{},ErrNotFound}
	return a,err
}

func (s *Store) PublishedArtifactByID(ctx context.Context,releaseID,artifactID string) (PublishedArtifact,error) {
	var a PublishedArtifact
	err:=s.DB.QueryRow(ctx,`
		SELECT r.id::text,r.version,a.id::text,a.target,a.file_name,a.storage_key,a.sha256,a.size_bytes
		FROM releases r
		JOIN release_artifacts a ON a.release_id=r.id
		WHERE r.id=$1 AND a.id=$2 AND r.status='published'
	`,releaseID,artifactID).Scan(&a.ReleaseID,&a.Version,&a.ArtifactID,&a.Target,&a.FileName,&a.StorageKey,&a.SHA256,&a.SizeBytes)
	if errors.Is(err,pgx.ErrNoRows){return PublishedArtifact{},ErrNotFound}
	return a,err
}
