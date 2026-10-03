package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type ClientTelemetryEvent struct {
	Platform string
	ClientVersion string
	EventType string
	ConfigVersion int64
	WorkerNodeID string
	NetworkType string
	DurationMS int64
}

func (s *Store) RecordClientTelemetry(ctx context.Context,e ClientTelemetryEvent) error {
	e.Platform=strings.ToLower(strings.TrimSpace(e.Platform))
	e.ClientVersion=strings.TrimSpace(e.ClientVersion)
	e.EventType=strings.ToLower(strings.TrimSpace(e.EventType))
	e.NetworkType=strings.ToLower(strings.TrimSpace(e.NetworkType))
	if e.Platform==""||len(e.Platform)>32||e.ClientVersion==""||len(e.ClientVersion)>64 {
		return fmt.Errorf("invalid telemetry identity")
	}
	switch e.EventType {
	case "connect_success","connect_failed","disconnect","recovered","stale_session_cleaned":
	default:return fmt.Errorf("invalid telemetry event")
	}
	switch e.NetworkType {
	case "wifi","cellular","ethernet","vpn","unknown":
	default:e.NetworkType="unknown"
	}
	if e.ConfigVersion<0 || e.DurationMS<0 || e.DurationMS>24*60*60*1000 {
		return fmt.Errorf("invalid telemetry metrics")
	}
	_,err:=s.DB.Exec(ctx,`
		INSERT INTO client_telemetry_daily(
		  day,platform,client_version,event_type,config_version,worker_node_id,network_type,
		  event_count,duration_ms_sum
		)
		VALUES(current_date,$1,$2,$3,$4,NULLIF($5,'')::uuid,$6,1,$7)
		ON CONFLICT(day,platform,client_version,event_type,config_version,worker_node_id,network_type)
		DO UPDATE SET
		  event_count=client_telemetry_daily.event_count+1,
		  duration_ms_sum=client_telemetry_daily.duration_ms_sum+EXCLUDED.duration_ms_sum,
		  updated_at=now()
	`,e.Platform,e.ClientVersion,e.EventType,e.ConfigVersion,e.WorkerNodeID,e.NetworkType,e.DurationMS)
	return err
}

type TelemetrySummary struct {
	Day time.Time `json:"day"`
	Platform string `json:"platform"`
	ClientVersion string `json:"client_version"`
	EventType string `json:"event_type"`
	ConfigVersion int64 `json:"config_version"`
	WorkerNodeID *string `json:"worker_node_id,omitempty"`
	NetworkType string `json:"network_type"`
	EventCount int64 `json:"event_count"`
	AverageDurationMS float64 `json:"average_duration_ms"`
}

func (s *Store) TelemetrySummary(ctx context.Context,days int) ([]TelemetrySummary,error) {
	if days<=0||days>90{days=14}
	rows,err:=s.DB.Query(ctx,`
		SELECT day::timestamptz,platform,client_version,event_type,config_version,
		       worker_node_id::text,network_type,event_count,
		       CASE WHEN event_count>0 THEN duration_ms_sum::float8/event_count ELSE 0 END
		FROM client_telemetry_daily
		WHERE day >= current_date-($1::int-1)
		ORDER BY day DESC,event_type,client_version
	`,days)
	if err!=nil{return nil,err}
	defer rows.Close()
	out:=make([]TelemetrySummary,0)
	for rows.Next(){
		var x TelemetrySummary
		if err:=rows.Scan(&x.Day,&x.Platform,&x.ClientVersion,&x.EventType,&x.ConfigVersion,&x.WorkerNodeID,&x.NetworkType,&x.EventCount,&x.AverageDurationMS);err!=nil{return nil,err}
		out=append(out,x)
	}
	return out,rows.Err()
}
