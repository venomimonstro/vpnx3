package store

import (
	"context"
	"time"
)

type RequeuedBuildJob struct {
	JobID string
	ReleaseID string
	WorkerID string
}

func (s *Store) RequeueStaleBuildJobs(ctx context.Context,staleBefore time.Time,limit int)([]RequeuedBuildJob,error){
	if limit<=0||limit>500{limit=100}
	tx,err:=s.DB.Begin(ctx)
	if err!=nil{return nil,err}
	defer tx.Rollback(ctx)

	rows,err:=tx.Query(ctx,`
		SELECT j.id::text,j.release_id::text,j.build_worker_id::text
		FROM build_jobs j
		LEFT JOIN nodes n ON n.id=j.build_worker_id
		WHERE j.status='running'
		  AND j.build_worker_id IS NOT NULL
		  AND (
		    n.id IS NULL
		    OR n.status<>'active'
		    OR n.last_heartbeat_at IS NULL
		    OR n.last_heartbeat_at<$1
		  )
		ORDER BY j.started_at NULLS FIRST,j.created_at
		FOR UPDATE OF j SKIP LOCKED
		LIMIT $2
	`,staleBefore,limit)
	if err!=nil{return nil,err}

	out:=make([]RequeuedBuildJob,0)
	for rows.Next(){
		var row RequeuedBuildJob
		if err:=rows.Scan(&row.JobID,&row.ReleaseID,&row.WorkerID);err!=nil{
			rows.Close()
			return nil,err
		}
		out=append(out,row)
	}
	rows.Close()
	if err:=rows.Err();err!=nil{return nil,err}

	for _,row:=range out{
		if _,err:=tx.Exec(ctx,`
			UPDATE build_jobs
			SET status='queued',
			    build_worker_id=NULL,
			    started_at=NULL,
			    finished_at=NULL,
			    error_summary='automatically requeued after build worker heartbeat timeout'
			WHERE id=$1 AND status='running'
		`,row.JobID);err!=nil{return nil,err}

		if _,err:=tx.Exec(ctx,`
			UPDATE releases
			SET status='building',updated_at=now()
			WHERE id=$1 AND status IN ('building','failed')
		`,row.ReleaseID);err!=nil{return nil,err}
	}

	if err:=tx.Commit(ctx);err!=nil{return nil,err}
	return out,nil
}
