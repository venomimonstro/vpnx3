package store

import (
	"context"
	"time"
)

type RetentionArtifact struct {
	ID string
	StorageKey string
}

func (s *Store) WithdrawnArtifactsForRetention(ctx context.Context,olderThan time.Time,limit int)([]RetentionArtifact,error){
	if limit<=0||limit>1000{limit=200}
	rows,err:=s.DB.Query(ctx,`
		SELECT a.id::text,a.storage_key
		FROM release_artifacts a
		JOIN releases r ON r.id=a.release_id
		WHERE r.status='withdrawn' AND r.updated_at < $1
		ORDER BY r.updated_at,a.created_at
		LIMIT $2
	`,olderThan,limit)
	if err!=nil{return nil,err}
	defer rows.Close()
	out:=make([]RetentionArtifact,0)
	for rows.Next(){var a RetentionArtifact;if err:=rows.Scan(&a.ID,&a.StorageKey);err!=nil{return nil,err};out=append(out,a)}
	return out,rows.Err()
}

func (s *Store) DeleteRetainedArtifact(ctx context.Context,id string)error{
	_,err:=s.DB.Exec(ctx,"DELETE FROM release_artifacts WHERE id=$1",id)
	return err
}
