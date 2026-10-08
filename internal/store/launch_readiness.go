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
	ActiveConfigMirrors int64
	RoutableConfigMirrors int64
	ActiveProbes int64
	FreshProbeNodes int64
	FreshDataPlaneWorkers int64
	QueuedBuildJobs int64
	RunningBuildJobs int64
	ActiveBuildWorkers int64
	PublishedReleases int64
	AutoRenewActive int64
	RenewalPending int64
	RenewalFailed24h int64
	RenewalDisabledFailures int64
	AuditChainValid bool
	SecurityExportPending int64
	SecurityExportDead int64
	SecurityExportOldestPendingAt *time.Time
	SecurityExportLastDeliveredAt *time.Time
	RuntimeManagedNodes int64
	RuntimeUpdaterReportedNodes int64
	RuntimeUpdaterUnhealthyNodes int64
	LocalHealthWarningNodes int64
	LocalHealthDegradedNodes int64
	NodesMissingDiskTelemetry int64
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
		  (SELECT count(*)::bigint FROM nodes WHERE role='config_mirror' AND status='active'),
		  (
		    SELECT count(*)::bigint
		    FROM nodes n
		    WHERE n.role='config_mirror' AND n.status='active'
		      AND EXISTS (
		        SELECT 1 FROM node_endpoints e
		        WHERE e.node_id=n.id AND e.enabled=true
		          AND e.kind='config_mirror' AND e.scheme='https' AND e.transport='https'
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
		  (SELECT count(*)::bigint FROM releases WHERE status='published'),
		  (SELECT count(*)::bigint FROM subscriptions WHERE auto_renew=true AND status IN ('active','grace')),
		  (SELECT count(*)::bigint FROM subscription_renewal_attempts WHERE status IN ('claimed','pending')),
		  (SELECT count(*)::bigint FROM subscription_renewal_attempts WHERE status='failed' AND updated_at>=now()-interval '24 hours'),
		  (SELECT count(*)::bigint FROM subscriptions WHERE auto_renew=false AND renewal_failures>=3),
		  verify_audit_chain(),
		  (SELECT count(*)::bigint FROM security_event_outbox WHERE status='pending'),
		  (SELECT count(*)::bigint FROM security_event_outbox WHERE status='dead'),
		  (SELECT min(created_at) FROM security_event_outbox WHERE status='pending'),
		  (SELECT max(delivered_at) FROM security_event_outbox WHERE status='delivered'),
		  (
		    SELECT count(*)::bigint FROM nodes
		    WHERE status='active' AND role IN ('worker','ingress','probe','config_mirror')
		  ),
		  (
		    SELECT count(*)::bigint FROM nodes
		    WHERE status='active' AND role IN ('worker','ingress','probe','config_mirror')
		      AND jsonb_typeof(metadata->'runtime_updates')='array'
		      AND jsonb_array_length(metadata->'runtime_updates')>0
		  ),
		  (
		    SELECT count(DISTINCT n.id)::bigint
		    FROM nodes n
		    CROSS JOIN LATERAL jsonb_array_elements(
		      CASE
		        WHEN jsonb_typeof(n.metadata->'runtime_updates')='array'
		        THEN n.metadata->'runtime_updates'
		        ELSE '[]'::jsonb
		      END
		    ) e
		    WHERE n.status='active'
		      AND n.role IN ('worker','ingress','probe','config_mirror')
		      AND jsonb_typeof(e->'healthy')='boolean'
		      AND (e->>'healthy')::boolean=false
		  ),
		  (
		    SELECT count(*)::bigint FROM nodes
		    WHERE status='active'
		      AND role IN ('worker','ingress','probe','config_mirror')
		      AND local_health_bad_streak>=2
		  ),
		  (
		    SELECT count(*)::bigint FROM nodes
		    WHERE status='degraded'
		      AND role IN ('worker','ingress','probe','config_mirror')
		      AND local_health_bad_streak>=3
		  ),
		  (
		    SELECT count(*)::bigint FROM nodes
		    WHERE status='active'
		      AND role IN ('worker','ingress','probe','config_mirror')
		      AND NOT (
		        jsonb_typeof(metadata->'disk_total_bytes')='number'
		        AND jsonb_typeof(metadata->'disk_available_bytes')='number'
		      )
		  )
	`).Scan(
		&d.LatestManifestAt,&d.ActiveWorkers,&d.RoutableWorkers,&d.ActiveIngresses,
		&d.ActiveConfigMirrors,&d.RoutableConfigMirrors,
		&d.ActiveProbes,&d.FreshProbeNodes,&d.FreshDataPlaneWorkers,
		&d.QueuedBuildJobs,&d.RunningBuildJobs,&d.ActiveBuildWorkers,&d.PublishedReleases,
		&d.AutoRenewActive,&d.RenewalPending,&d.RenewalFailed24h,&d.RenewalDisabledFailures,
		&d.AuditChainValid,
		&d.SecurityExportPending,&d.SecurityExportDead,
		&d.SecurityExportOldestPendingAt,&d.SecurityExportLastDeliveredAt,
		&d.RuntimeManagedNodes,&d.RuntimeUpdaterReportedNodes,&d.RuntimeUpdaterUnhealthyNodes,
		&d.LocalHealthWarningNodes,&d.LocalHealthDegradedNodes,&d.NodesMissingDiskTelemetry,
	)
	if err!=nil{return LaunchReadinessData{},fmt.Errorf("launch readiness: %w",err)}
	return d,nil
}
