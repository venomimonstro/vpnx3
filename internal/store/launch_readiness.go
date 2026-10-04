package store

import (
	"context"
	"fmt"
	"time"
)

type LaunchReadinessData struct {
	LatestManifestAt *time.Time
	ActiveWorkers int64
	RoutableWorkers int64
	ActiveIngresses int64
	ActiveProbes int64
	FreshProbeNodes int64
	FreshDataPlaneWorkers int64
	QueuedBuildJobs int64
	RunningBuildJobs int64
	ActiveBuildWorkers int64
	PublishedReleases int64
}

func (s *Store) LaunchReadiness(ctx context.Context)(LaunchReadinessData,error){
	var d LaunchReadinessData
	err:=s.DB.QueryRow(ctx,`
		SELECT
		  (SELECT max(created_at) FROM config_manifests WHERE payload_raw IS NOT NULL),
		  (SELECT count(*)::bigint FROM nodes WHERE role='worker' AND status='active'),
		  (
		    SELECT count(*)::bigint
		    FROM nodes n
		    WHERE n.role='worker' AND n.status='active'
		      AND EXISTS (
		        SELECT 1 FROM node_endpoints e
		        WHERE e.node_id=n.id AND e.enabled=true AND e.kind='session_api' AND e.scheme='https'
		      )
		      AND EXISTS (
		        SELECT 1 FROM node_endpoints e
		        WHERE e.node_id=n.id AND e.enabled=true AND e.kind='wireguard' AND e.scheme='udp'
		      )
		  ),
		  (
		    SELECT count(*)::bigint
		    FROM nodes n
		    WHERE n.role='ingress' AND n.status='active'
		      AND EXISTS (
		        SELECT 1 FROM node_endpoints e
		        WHERE e.node_id=n.id AND e.enabled=true AND e.kind='ingress' AND e.scheme='https'
		      )
		  ),
		  (SELECT count(*)::bigint FROM nodes WHERE role='probe' AND status='active'),
		  (
		    SELECT count(DISTINCT probe_node_id)::bigint
		    FROM probe_results
		    WHERE observed_at>=now()-interval '5 minutes'
		  ),
		  (
		    SELECT count(DISTINCT target_node_id)::bigint
		    FROM probe_results
		    WHERE observed_at>=now()-interval '5 minutes'
		      AND endpoint_kind='wireguard_data_plane'
		      AND success=true
		  ),
		  (SELECT count(*)::bigint FROM build_jobs WHERE status='queued'),
		  (SELECT count(*)::bigint FROM build_jobs WHERE status='running'),
		  (SELECT count(*)::bigint FROM nodes WHERE role='build_worker' AND status='active'),
		  (SELECT count(*)::bigint FROM releases WHERE status='published')
	`).Scan(
		&d.LatestManifestAt,&d.ActiveWorkers,&d.RoutableWorkers,&d.ActiveIngresses,
		&d.ActiveProbes,&d.FreshProbeNodes,&d.FreshDataPlaneWorkers,
		&d.QueuedBuildJobs,&d.RunningBuildJobs,&d.ActiveBuildWorkers,&d.PublishedReleases,
	)
	if err!=nil{return LaunchReadinessData{},fmt.Errorf("launch readiness: %w",err)}
	return d,nil
}
