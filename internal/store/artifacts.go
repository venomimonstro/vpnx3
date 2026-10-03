package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
)

type ArtifactJob struct {
	JobID string
	ReleaseID string
	Version string
	Target string
	Status string
}

func (s *Store) ArtifactJobForWorker(ctx context.Context,jobID,workerID string) (ArtifactJob,error) {
	var j ArtifactJob
	err:=s.DB.QueryRow(ctx,`
		SELECT j.id::text,j.release_id::text,r.version,j.target,j.status
		FROM build_jobs j JOIN releases r ON r.id=j.release_id
		WHERE j.id=$1 AND j.build_worker_id=$2
	`,jobID,workerID).Scan(&j.JobID,&j.ReleaseID,&j.Version,&j.Target,&j.Status)
	if errors.Is(err,pgx.ErrNoRows) { return ArtifactJob{},ErrNotFound }
	return j,err
}

func ArtifactExtension(target string) (string,error) {
	switch target {
	case "android_apk": return ".apk",nil
	case "android_aab": return ".aab",nil
	case "chrome_zip","firefox_zip": return ".zip",nil
	case "ios_ipa": return ".ipa",nil
	case "controlplane_linux_amd64","node_agent_linux_amd64","vpn_worker_linux_amd64","probe_agent_linux_amd64","ingress_proxy_linux_amd64":
		return ".bin",nil
	default:return "",fmt.Errorf("unsupported artifact target")
	}
}

func SafeArtifactFileName(version,target string) (string,error) {
	ext,err:=ArtifactExtension(target);if err!=nil{return "",err}
	v:=strings.Map(func(r rune) rune {
		if (r>='a'&&r<='z')||(r>='A'&&r<='Z')||(r>='0'&&r<='9')||r=='.'||r=='-'||r=='_' { return r }
		return '-'
	},version)
	v=strings.Trim(v,".-_")
	if v=="" { v="release" }
	return "vpnx3-"+v+"-"+target+ext,nil
}

func (s *Store) RegisterArtifactAndSucceed(ctx context.Context,jobID,workerID,fileName,storageKey,sha256 string,size int64) error {
	if len(sha256)!=64 || size<0 || filepath.Base(fileName)!=fileName || storageKey=="" {
		return fmt.Errorf("invalid artifact metadata")
	}
	tx,err:=s.DB.Begin(ctx);if err!=nil{return err};defer tx.Rollback(ctx)
	var releaseID,target,status string
	err=tx.QueryRow(ctx,`
		SELECT release_id::text,target,status
		FROM build_jobs WHERE id=$1 AND build_worker_id=$2 FOR UPDATE
	`,jobID,workerID).Scan(&releaseID,&target,&status)
	if errors.Is(err,pgx.ErrNoRows){return ErrNotFound}
	if err!=nil{return err}
	if status!="running"{return fmt.Errorf("build job is not running")}

	if _,err:=tx.Exec(ctx,`
		INSERT INTO release_artifacts(release_id,build_job_id,target,file_name,storage_key,sha256,size_bytes)
		VALUES($1,$2,$3,$4,$5,$6,$7)
	`,releaseID,jobID,target,fileName,storageKey,sha256,size);err!=nil{return fmt.Errorf("register artifact: %w",err)}
	if _,err:=tx.Exec(ctx,`
		UPDATE build_jobs SET status='succeeded',finished_at=now(),error_summary=NULL WHERE id=$1
	`,jobID);err!=nil{return err}

	var total,success,failed int
	if err:=tx.QueryRow(ctx,`
		SELECT count(*)::int,count(*) FILTER(WHERE status='succeeded')::int,count(*) FILTER(WHERE status='failed')::int
		FROM build_jobs WHERE release_id=$1
	`,releaseID).Scan(&total,&success,&failed);err!=nil{return err}
	releaseStatus:="building"
	if failed>0{releaseStatus="failed"}else if total>0&&success==total{releaseStatus="ready"}
	if _,err:=tx.Exec(ctx,"UPDATE releases SET status=$2,updated_at=now() WHERE id=$1",releaseID,releaseStatus);err!=nil{return err}
	return tx.Commit(ctx)
}
