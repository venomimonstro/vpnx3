package store

import (
	"context"
	"fmt"
)

type CapacityBreakdown struct {
	Name string `json:"name"`
	Nodes int64 `json:"nodes"`
	Capacity int64 `json:"capacity"`
	Sessions int64 `json:"sessions"`
	CapacityShare float64 `json:"capacity_share"`
}

type NetworkRiskSnapshot struct {
	ActiveWorkers int64 `json:"active_workers"`
	ConfiguredCapacity int64 `json:"configured_capacity"`
	CurrentSessions int64 `json:"current_sessions"`
	WorkersWithoutCapacity int64 `json:"workers_without_capacity"`
	UtilizationPercent float64 `json:"utilization_percent"`
	HeadroomPercent float64 `json:"headroom_percent"`
	MaxProviderShare float64 `json:"max_provider_share"`
	MaxCountryShare float64 `json:"max_country_share"`
	TopProvider string `json:"top_provider"`
	TopCountry string `json:"top_country"`
	Providers []CapacityBreakdown `json:"providers"`
	Countries []CapacityBreakdown `json:"countries"`
}

func (s *Store) NetworkRisk(ctx context.Context)(NetworkRiskSnapshot,error){
	var out NetworkRiskSnapshot
	if err:=s.DB.QueryRow(ctx,`
		SELECT
		  count(*)::bigint,
		  COALESCE(sum(capacity_sessions),0)::bigint,
		  COALESCE(sum(current_sessions),0)::bigint,
		  count(*) FILTER (WHERE capacity_sessions IS NULL)::bigint
		FROM nodes
		WHERE role='worker' AND status='active'
	`).Scan(&out.ActiveWorkers,&out.ConfiguredCapacity,&out.CurrentSessions,&out.WorkersWithoutCapacity);err!=nil{
		return NetworkRiskSnapshot{},fmt.Errorf("network risk totals: %w",err)
	}
	if out.ConfiguredCapacity>0{
		out.UtilizationPercent=float64(out.CurrentSessions)/float64(out.ConfiguredCapacity)*100
		out.HeadroomPercent=100-out.UtilizationPercent
		if out.HeadroomPercent<0{out.HeadroomPercent=0}
	}

	providers,err:=s.capacityBreakdown(ctx,`
		SELECT COALESCE(NULLIF(provider,''),'unknown') AS name,
		       count(*)::bigint,
		       COALESCE(sum(capacity_sessions),0)::bigint,
		       COALESCE(sum(current_sessions),0)::bigint
		FROM nodes
		WHERE role='worker' AND status='active'
		GROUP BY 1
		ORDER BY 3 DESC,2 DESC,1
	`,out.ConfiguredCapacity)
	if err!=nil{return NetworkRiskSnapshot{},err}
	out.Providers=providers
	if len(providers)>0{
		out.TopProvider=providers[0].Name
		out.MaxProviderShare=providers[0].CapacityShare
	}

	countries,err:=s.capacityBreakdown(ctx,`
		SELECT COALESCE(NULLIF(country_code,''),'unknown') AS name,
		       count(*)::bigint,
		       COALESCE(sum(capacity_sessions),0)::bigint,
		       COALESCE(sum(current_sessions),0)::bigint
		FROM nodes
		WHERE role='worker' AND status='active'
		GROUP BY 1
		ORDER BY 3 DESC,2 DESC,1
	`,out.ConfiguredCapacity)
	if err!=nil{return NetworkRiskSnapshot{},err}
	out.Countries=countries
	if len(countries)>0{
		out.TopCountry=countries[0].Name
		out.MaxCountryShare=countries[0].CapacityShare
	}
	return out,nil
}

func (s *Store) capacityBreakdown(ctx context.Context,query string,totalCapacity int64)([]CapacityBreakdown,error){
	rows,err:=s.DB.Query(ctx,query)
	if err!=nil{return nil,fmt.Errorf("capacity breakdown: %w",err)}
	defer rows.Close()
	out:=make([]CapacityBreakdown,0)
	for rows.Next(){
		var row CapacityBreakdown
		if err:=rows.Scan(&row.Name,&row.Nodes,&row.Capacity,&row.Sessions);err!=nil{return nil,err}
		if totalCapacity>0{row.CapacityShare=float64(row.Capacity)/float64(totalCapacity)*100}
		out=append(out,row)
	}
	return out,rows.Err()
}
