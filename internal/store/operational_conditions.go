package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type OperationalConditionResult struct {
	Code string
	State string
	OpenedIncidentID string
	ResolvedIncidentID string
	ConsecutiveBad int
	ConsecutiveGood int
}

func (s *Store) EvaluateOperationalCondition(
	ctx context.Context,
	code,title,detail string,
	healthy bool,
	openAfter,recoverAfter int,
	now time.Time,
)(OperationalConditionResult,error){
	if code==""||title==""{return OperationalConditionResult{},fmt.Errorf("invalid operational condition")}
	if openAfter<1{openAfter=3}
	if recoverAfter<1{recoverAfter=3}
	tx,err:=s.DB.Begin(ctx)
	if err!=nil{return OperationalConditionResult{},err}
	defer tx.Rollback(ctx)

	if _,err:=tx.Exec(ctx,`
		INSERT INTO operational_condition_states(code,last_checked_at,last_detail)
		VALUES($1,$2,$3)
		ON CONFLICT(code) DO NOTHING
	`,code,now,detail);err!=nil{return OperationalConditionResult{},err}

	var state string
	var bad,good int
	var incidentID *string
	err=tx.QueryRow(ctx,`
		SELECT state,consecutive_bad,consecutive_good,active_incident_id::text
		FROM operational_condition_states
		WHERE code=$1
		FOR UPDATE
	`,code).Scan(&state,&bad,&good,&incidentID)
	if errors.Is(err,pgx.ErrNoRows){return OperationalConditionResult{},ErrNotFound}
	if err!=nil{return OperationalConditionResult{},err}

	result:=OperationalConditionResult{Code:code,State:state}
	if healthy{
		bad=0
		good++
		if incidentID!=nil && good>=recoverAfter{
			var resolved string
			err:=tx.QueryRow(ctx,`
				UPDATE incidents
				SET status='resolved',resolved_at=$2,
				    root_cause=COALESCE(root_cause,'Автоматически закрыт после устойчивого восстановления SLO-сигнала.')
				WHERE id=$1 AND status<>'resolved'
				RETURNING id::text
			`,*incidentID,now).Scan(&resolved)
			if err!=nil && !errors.Is(err,pgx.ErrNoRows){return OperationalConditionResult{},err}
			if err==nil{result.ResolvedIncidentID=resolved}
			incidentID=nil
		}
		newState:="healthy"
		if incidentID!=nil{newState="unhealthy"}
		if _,err:=tx.Exec(ctx,`
			UPDATE operational_condition_states
			SET state=$2,consecutive_bad=0,consecutive_good=$3,
			    active_incident_id=$4,last_detail=$5,last_checked_at=$6,
			    changed_at=CASE WHEN state<>$2 THEN $6 ELSE changed_at END,
			    updated_at=$6
			WHERE code=$1
		`,code,newState,good,incidentID,detail,now);err!=nil{return OperationalConditionResult{},err}
		result.State=newState
		result.ConsecutiveGood=good
	}else{
		good=0
		bad++
		if incidentID==nil && bad>=openAfter{
			var created string
			err:=tx.QueryRow(ctx,`
				INSERT INTO incidents(severity,status,title,summary,scope,detected_at)
				VALUES('critical','open',$1,$2,jsonb_build_object('condition_code',$3,'automation',true),$4)
				RETURNING id::text
			`,title,detail,code,now).Scan(&created)
			if err!=nil{return OperationalConditionResult{},err}
			incidentID=&created
			result.OpenedIncidentID=created
		}
		newState:="unknown"
		if incidentID!=nil||bad>=openAfter{newState="unhealthy"}
		if _,err:=tx.Exec(ctx,`
			UPDATE operational_condition_states
			SET state=$2,consecutive_bad=$3,consecutive_good=0,
			    active_incident_id=$4,last_detail=$5,last_checked_at=$6,
			    changed_at=CASE WHEN state<>$2 THEN $6 ELSE changed_at END,
			    updated_at=$6
			WHERE code=$1
		`,code,newState,bad,incidentID,detail,now);err!=nil{return OperationalConditionResult{},err}
		result.State=newState
		result.ConsecutiveBad=bad
	}
	if err:=tx.Commit(ctx);err!=nil{return OperationalConditionResult{},err}
	return result,nil
}

type OperationalConditionState struct {
	Code string `json:"code"`
	State string `json:"state"`
	ConsecutiveBad int `json:"consecutive_bad"`
	ConsecutiveGood int `json:"consecutive_good"`
	ActiveIncidentID *string `json:"active_incident_id,omitempty"`
	LastDetail *string `json:"last_detail,omitempty"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
	ChangedAt time.Time `json:"changed_at"`
}

func (s *Store) OperationalConditionStates(ctx context.Context)([]OperationalConditionState,error){
	rows,err:=s.DB.Query(ctx,`
		SELECT code,state,consecutive_bad,consecutive_good,active_incident_id::text,
		       last_detail,last_checked_at,changed_at
		FROM operational_condition_states
		ORDER BY code
	`)
	if err!=nil{return nil,err}
	defer rows.Close()
	out:=make([]OperationalConditionState,0)
	for rows.Next(){
		var v OperationalConditionState
		if err:=rows.Scan(
			&v.Code,&v.State,&v.ConsecutiveBad,&v.ConsecutiveGood,&v.ActiveIncidentID,
			&v.LastDetail,&v.LastCheckedAt,&v.ChangedAt,
		);err!=nil{return nil,err}
		out=append(out,v)
	}
	return out,rows.Err()
}
