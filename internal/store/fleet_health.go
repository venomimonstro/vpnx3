package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type FleetHealthSummary struct {
	TrackedNodes int64 `json:"tracked_nodes"`
	LocalWarningNodes int64 `json:"local_warning_nodes"`
	LocalDegradedNodes int64 `json:"local_degraded_nodes"`
	MissingDiskTelemetry int64 `json:"missing_disk_telemetry"`
	UpdaterUnhealthyNodes int64 `json:"updater_unhealthy_nodes"`
}

type FleetHealthRow struct {
	ID string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
	Status string `json:"status"`
	Provider *string `json:"provider,omitempty"`
	CountryCode *string `json:"country_code,omitempty"`
	LastHeartbeatAt *time.Time `json:"last_heartbeat_at,omitempty"`
	LocalBadStreak int `json:"local_bad_streak"`
	LocalGoodStreak int `json:"local_good_streak"`
	WorkerHealthy *bool `json:"worker_healthy,omitempty"`
	DiskTotalBytes uint64 `json:"disk_total_bytes,omitempty"`
	DiskAvailableBytes uint64 `json:"disk_available_bytes,omitempty"`
	DiskFreePercent float64 `json:"disk_free_percent,omitempty"`
	UptimeSeconds int64 `json:"uptime_seconds,omitempty"`
	KernelRelease string `json:"kernel_release,omitempty"`
	RuntimeUpdaterCount int `json:"runtime_updater_count"`
	RuntimeUpdaterUnhealthy int `json:"runtime_updater_unhealthy"`
}

type fleetMetadata struct {
	WorkerHealthy *bool `json:"worker_healthy"`
	DiskTotalBytes uint64 `json:"disk_total_bytes"`
	DiskAvailableBytes uint64 `json:"disk_available_bytes"`
	UptimeSeconds int64 `json:"uptime_seconds"`
	KernelRelease string `json:"kernel_release"`
	RuntimeUpdates []struct{
		Healthy bool `json:"healthy"`
	} `json:"runtime_updates"`
}

func (s *Store) FleetHealth(ctx context.Context)(FleetHealthSummary,[]FleetHealthRow,error){
	rows,err:=s.DB.Query(ctx,`
		SELECT id::text,name,role::text,status::text,provider,country_code,last_heartbeat_at,
		       local_health_bad_streak,local_health_good_streak,metadata
		FROM nodes
		WHERE status NOT IN ('retired','destroyed')
		  AND role IN ('worker','ingress','probe','config_mirror')
		ORDER BY
		  CASE status WHEN 'degraded' THEN 0 WHEN 'active' THEN 1 ELSE 2 END,
		  local_health_bad_streak DESC,name
	`)
	if err!=nil{return FleetHealthSummary{},nil,fmt.Errorf("fleet health rows: %w",err)}
	defer rows.Close()

	out:=make([]FleetHealthRow,0)
	var summary FleetHealthSummary
	for rows.Next(){
		var row FleetHealthRow
		var raw []byte
		if err:=rows.Scan(
			&row.ID,&row.Name,&row.Role,&row.Status,&row.Provider,&row.CountryCode,&row.LastHeartbeatAt,
			&row.LocalBadStreak,&row.LocalGoodStreak,&raw,
		);err!=nil{return FleetHealthSummary{},nil,err}

		var meta fleetMetadata
		if len(raw)>0{_ = json.Unmarshal(raw,&meta)}
		row.WorkerHealthy=meta.WorkerHealthy
		row.DiskTotalBytes=meta.DiskTotalBytes
		row.DiskAvailableBytes=meta.DiskAvailableBytes
		row.UptimeSeconds=meta.UptimeSeconds
		if len(meta.KernelRelease)<=128{row.KernelRelease=meta.KernelRelease}
		if row.DiskTotalBytes>0{
			row.DiskFreePercent=float64(row.DiskAvailableBytes)/float64(row.DiskTotalBytes)*100
		}
		row.RuntimeUpdaterCount=len(meta.RuntimeUpdates)
		for _,u:=range meta.RuntimeUpdates{if !u.Healthy{row.RuntimeUpdaterUnhealthy++}}

		summary.TrackedNodes++
		if row.Status=="active"&&row.LocalBadStreak>=2{summary.LocalWarningNodes++}
		if row.Status=="degraded"&&row.LocalBadStreak>=3{summary.LocalDegradedNodes++}
		if row.Status=="active"&&row.DiskTotalBytes==0{summary.MissingDiskTelemetry++}
		if row.RuntimeUpdaterUnhealthy>0{summary.UpdaterUnhealthyNodes++}
		out=append(out,row)
	}
	if err:=rows.Err();err!=nil{return FleetHealthSummary{},nil,err}
	return summary,out,nil
}
