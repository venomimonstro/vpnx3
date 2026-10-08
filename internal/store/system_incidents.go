package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) SyncSystemIncident(
	ctx context.Context,
	code,severity,title,summary string,
	active bool,
	now time.Time,
)error{
	tx,err:=s.DB.Begin(ctx);if err!=nil{return err};defer tx.Rollback(ctx)

	var incidentID *string
	var currentActive bool
	err=tx.QueryRow(ctx,`
		SELECT incident_id::text,active
		FROM system_incident_state
		WHERE code=$1
		FOR UPDATE
	`,code).Scan(&incidentID,&currentActive)
	if errors.Is(err,pgx.ErrNoRows){
		if !active{
			if _,err:=tx.Exec(ctx,`
				INSERT INTO system_incident_state(code,active,last_transition_at,updated_at)
				VALUES($1,false,$2,$2)
			`,code,now);err!=nil{return err}
			return tx.Commit(ctx)
		}
		scope,_:=json.Marshal(map[string]string{"system_code":code})
		var id string
		if err:=tx.QueryRow(ctx,`
			INSERT INTO incidents(severity,status,title,summary,scope,detected_at)
			VALUES($1,'open',$2,$3,$4,$5)
			RETURNING id::text
		`,severity,title,summary,scope,now).Scan(&id);err!=nil{return fmt.Errorf("create system incident: %w",err)}
		if _,err:=tx.Exec(ctx,`
			INSERT INTO system_incident_state(code,incident_id,active,last_transition_at,updated_at)
			VALUES($1,$2,true,$3,$3)
		`,code,id,now);err!=nil{return err}
		return tx.Commit(ctx)
	}
	if err!=nil{return err}

	if active{
		if currentActive{return tx.Commit(ctx)}
		scope,_:=json.Marshal(map[string]string{"system_code":code})
		var id string
		if err:=tx.QueryRow(ctx,`
			INSERT INTO incidents(severity,status,title,summary,scope,detected_at)
			VALUES($1,'open',$2,$3,$4,$5)
			RETURNING id::text
		`,severity,title,summary,scope,now).Scan(&id);err!=nil{return err}
		if _,err:=tx.Exec(ctx,`
			UPDATE system_incident_state
			SET incident_id=$2,active=true,last_transition_at=$3,updated_at=$3
			WHERE code=$1
		`,code,id,now);err!=nil{return err}
		return tx.Commit(ctx)
	}

	if !currentActive{return tx.Commit(ctx)}
	if incidentID!=nil{
		if _,err:=tx.Exec(ctx,`
			UPDATE incidents
			SET status='resolved',resolved_at=$2,root_cause='Recovered automatically after sustained healthy observations'
			WHERE id=$1 AND status<>'resolved'
		`,*incidentID,now);err!=nil{return err}
	}
	if _,err:=tx.Exec(ctx,`
		UPDATE system_incident_state
		SET active=false,last_transition_at=$2,updated_at=$2
		WHERE code=$1
	`,code,now);err!=nil{return err}
	return tx.Commit(ctx)
}
