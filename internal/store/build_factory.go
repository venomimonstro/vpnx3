package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var allowedBuildTargets=map[string]bool{
	"android_apk":true,
	"android_aab":true,
	"chrome_zip":true,
	"firefox_zip":true,
	"ios_ipa":true,
	"controlplane_linux_amd64":true,
	"node_agent_linux_amd64":true,
	"vpn_worker_linux_amd64":true,
	"probe_agent_linux_amd64":true,
	"ingress_proxy_linux_amd64":true,
	"build_worker_darwin_arm64":true,
	"config_mirror_linux_amd64":true,
	"runtime_updater_linux_amd64":true,
	"backup_replicator_linux_amd64":true,
	"wal_replicator_linux_amd64":true,
}

type Release struct {
	ID           string     `json:"id"`
	Version      string     `json:"version"`
	SourceCommit string     `json:"source_commit"`
	Status       string     `json:"status"`
	Notes        string     `json:"notes"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	PublishedAt  *time.Time `json:"published_at,omitempty"`
}

type BuildJob struct {
	ID            string     `json:"id"`
	ReleaseID     string     `json:"release_id"`
	Target        string     `json:"target"`
	Status        string     `json:"status"`
	BuildWorkerID *string    `json:"build_worker_id,omitempty"`
	Attempt       int        `json:"attempt"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	ErrorSummary  *string    `json:"error_summary,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	Version       string     `json:"version,omitempty"`
	SourceCommit  string     `json:"source_commit,omitempty"`
}

func (s *Store) CreateRelease(ctx context.Context,version,sourceCommit,notes,adminID string,targets []string) (Release,error) {
	version=strings.TrimSpace(version)
	sourceCommit=strings.ToLower(strings.TrimSpace(sourceCommit))
	notes=strings.TrimSpace(notes)
	if version=="" || len(version)>80 || len(sourceCommit)!=40 || len(notes)>4000 {
		return Release{},fmt.Errorf("invalid release")
	}
	if len(targets)==0 || len(targets)>5 { return Release{},fmt.Errorf("invalid build targets") }
	seen:=map[string]bool{}
	for _,target:=range targets {
		if !allowedBuildTargets[target] || seen[target] { return Release{},fmt.Errorf("invalid build target") }
		seen[target]=true
	}

	tx,err:=s.DB.Begin(ctx)
	if err!=nil { return Release{},err }
	defer tx.Rollback(ctx)
	var r Release
	err=tx.QueryRow(ctx,`
		INSERT INTO releases(version,source_commit,status,notes,created_by)
		VALUES($1,$2,'building',$3,$4)
		RETURNING id::text,version,source_commit,status,notes,created_at,updated_at,published_at
	`,version,sourceCommit,notes,adminID).Scan(&r.ID,&r.Version,&r.SourceCommit,&r.Status,&r.Notes,&r.CreatedAt,&r.UpdatedAt,&r.PublishedAt)
	if err!=nil { return Release{},fmt.Errorf("create release: %w",err) }
	for _,target:=range targets {
		if _,err:=tx.Exec(ctx,`
			INSERT INTO build_jobs(release_id,target,status) VALUES($1,$2,'queued')
		`,r.ID,target); err!=nil { return Release{},fmt.Errorf("create build job: %w",err) }
	}
	if err:=tx.Commit(ctx); err!=nil { return Release{},err }
	return r,nil
}

func (s *Store) ListReleases(ctx context.Context,limit int) ([]Release,error) {
	if limit<=0 || limit>200 { limit=100 }
	rows,err:=s.DB.Query(ctx,`
		SELECT id::text,version,source_commit,status,notes,created_at,updated_at,published_at
		FROM releases ORDER BY created_at DESC LIMIT $1
	`,limit)
	if err!=nil { return nil,err }
	defer rows.Close()
	out:=make([]Release,0)
	for rows.Next() {
		var r Release
		if err:=rows.Scan(&r.ID,&r.Version,&r.SourceCommit,&r.Status,&r.Notes,&r.CreatedAt,&r.UpdatedAt,&r.PublishedAt); err!=nil { return nil,err }
		out=append(out,r)
	}
	return out,rows.Err()
}

func (s *Store) ReleaseJobs(ctx context.Context,releaseID string) ([]BuildJob,error) {
	rows,err:=s.DB.Query(ctx,`
		SELECT id::text,release_id::text,target,status,build_worker_id::text,attempt,
		       started_at,finished_at,error_summary,created_at
		FROM build_jobs WHERE release_id=$1 ORDER BY created_at,target
	`,releaseID)
	if err!=nil { return nil,err }
	defer rows.Close()
	out:=make([]BuildJob,0)
	for rows.Next() {
		var j BuildJob
		if err:=rows.Scan(&j.ID,&j.ReleaseID,&j.Target,&j.Status,&j.BuildWorkerID,&j.Attempt,&j.StartedAt,&j.FinishedAt,&j.ErrorSummary,&j.CreatedAt); err!=nil { return nil,err }
		out=append(out,j)
	}
	return out,rows.Err()
}

type BuildWorkerAuth struct {
	PublicKey []byte
	Sequence int64
	Status string
	Role string
}

func (s *Store) BuildWorkerAuthState(ctx context.Context,nodeID string) (BuildWorkerAuth,error) {
	var a BuildWorkerAuth
	err:=s.DB.QueryRow(ctx,`
		SELECT identity_public_key,build_sequence,status::text,role::text
		FROM nodes WHERE id=$1
	`,nodeID).Scan(&a.PublicKey,&a.Sequence,&a.Status,&a.Role)
	if errors.Is(err,pgx.ErrNoRows) { return BuildWorkerAuth{},ErrNotFound }
	return a,err
}

func (s *Store) AdvanceBuildSequence(ctx context.Context,nodeID string,sequence int64) error {
	tag,err:=s.DB.Exec(ctx,`
		UPDATE nodes SET build_sequence=$2,last_seen_at=now(),updated_at=now()
		WHERE id=$1 AND role='build_worker' AND build_sequence<$2
	`,nodeID,sequence)
	if err!=nil { return err }
	if tag.RowsAffected()!=1 { return fmt.Errorf("stale build sequence") }
	return nil
}

func (s *Store) ClaimBuildJob(ctx context.Context,workerID string,targets []string) (BuildJob,error) {
	var workerStatus string
	if err:=s.DB.QueryRow(ctx,`
		SELECT status::text FROM nodes WHERE id=$1 AND role='build_worker'
	`,workerID).Scan(&workerStatus);err!=nil{return BuildJob{},ErrNotFound}
	if workerStatus!="active"{return BuildJob{},fmt.Errorf("build worker is not active")}
	valid:=make([]string,0,len(targets))
	for _,target:=range targets {
		if allowedBuildTargets[target] { valid=append(valid,target) }
	}
	if len(valid)==0 { return BuildJob{},ErrNotFound }

	tx,err:=s.DB.Begin(ctx)
	if err!=nil { return BuildJob{},err }
	defer tx.Rollback(ctx)
	var j BuildJob
	err=tx.QueryRow(ctx,`
		SELECT j.id::text,j.release_id::text,j.target,j.status,j.attempt,j.created_at,
		       r.version,r.source_commit
		FROM build_jobs j
		JOIN releases r ON r.id=j.release_id
		WHERE j.status='queued' AND j.target=ANY($1)
		ORDER BY j.created_at
		FOR UPDATE OF j SKIP LOCKED
		LIMIT 1
	`,valid).Scan(&j.ID,&j.ReleaseID,&j.Target,&j.Status,&j.Attempt,&j.CreatedAt,&j.Version,&j.SourceCommit)
	if errors.Is(err,pgx.ErrNoRows) { return BuildJob{},ErrNotFound }
	if err!=nil { return BuildJob{},err }

	if _,err:=tx.Exec(ctx,`
		UPDATE build_jobs
		SET status='running',build_worker_id=$2,attempt=attempt+1,started_at=now(),finished_at=NULL,error_summary=NULL
		WHERE id=$1
	`,j.ID,workerID); err!=nil { return BuildJob{},err }
	if err:=tx.Commit(ctx); err!=nil { return BuildJob{},err }
	j.Status="running"
	j.BuildWorkerID=&workerID
	j.Attempt++
	now:=time.Now().UTC();j.StartedAt=&now
	return j,nil
}

func (s *Store) CompleteBuildJob(ctx context.Context,workerID,jobID,status,errorSummary string) error {
	if status!="succeeded" && status!="failed" { return fmt.Errorf("invalid build result") }
	errorSummary=strings.TrimSpace(errorSummary)
	if len(errorSummary)>4000 { errorSummary=errorSummary[:4000] }
	tx,err:=s.DB.Begin(ctx)
	if err!=nil { return err }
	defer tx.Rollback(ctx)
	var releaseID string
	err=tx.QueryRow(ctx,`
		UPDATE build_jobs
		SET status=$3,finished_at=now(),error_summary=NULLIF($4,'')
		WHERE id=$1 AND build_worker_id=$2 AND status='running'
		RETURNING release_id::text
	`,jobID,workerID,status,errorSummary).Scan(&releaseID)
	if errors.Is(err,pgx.ErrNoRows) { return ErrNotFound }
	if err!=nil { return err }

	var total,success,failed int
	if err:=tx.QueryRow(ctx,`
		SELECT count(*)::int,
		       count(*) FILTER (WHERE status='succeeded')::int,
		       count(*) FILTER (WHERE status='failed')::int
		FROM build_jobs WHERE release_id=$1
	`,releaseID).Scan(&total,&success,&failed); err!=nil { return err }
	releaseStatus:="building"
	if failed>0 { releaseStatus="failed" } else if total>0 && success==total { releaseStatus="ready" }
	if _,err:=tx.Exec(ctx,"UPDATE releases SET status=$2,updated_at=now() WHERE id=$1",releaseID,releaseStatus); err!=nil { return err }
	return tx.Commit(ctx)
}


func (s *Store) RetryBuildJob(ctx context.Context,jobID string) (string,error) {
	tx,err:=s.DB.Begin(ctx)
	if err!=nil{return "",err}
	defer tx.Rollback(ctx)
	var releaseID,status string
	err=tx.QueryRow(ctx,`
		SELECT release_id::text,status FROM build_jobs WHERE id=$1 FOR UPDATE
	`,jobID).Scan(&releaseID,&status)
	if errors.Is(err,pgx.ErrNoRows){return "",ErrNotFound}
	if err!=nil{return "",err}
	if status!="failed" && status!="cancelled" {
		return "",fmt.Errorf("build job is not retryable")
	}
	if _,err:=tx.Exec(ctx,`
		UPDATE build_jobs
		SET status='queued',build_worker_id=NULL,started_at=NULL,finished_at=NULL,error_summary=NULL
		WHERE id=$1
	`,jobID);err!=nil{return "",err}
	if _,err:=tx.Exec(ctx,`
		UPDATE releases SET status='building',updated_at=now() WHERE id=$1
	`,releaseID);err!=nil{return "",err}
	if err:=tx.Commit(ctx);err!=nil{return "",err}
	return releaseID,nil
}
