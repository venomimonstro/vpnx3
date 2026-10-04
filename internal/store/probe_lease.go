package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type ProbeLeaseAuthState struct {
	PublicKey []byte
	Sequence int64
	Status string
	Role string
}

func (s *Store) ProbeLeaseAuthState(ctx context.Context,nodeID string)(ProbeLeaseAuthState,error){
	var state ProbeLeaseAuthState
	err:=s.DB.QueryRow(ctx,`
		SELECT identity_public_key,probe_lease_sequence,status::text,role::text
		FROM nodes WHERE id=$1
	`,nodeID).Scan(&state.PublicKey,&state.Sequence,&state.Status,&state.Role)
	if errors.Is(err,pgx.ErrNoRows){return ProbeLeaseAuthState{},ErrNotFound}
	if err!=nil{return ProbeLeaseAuthState{},fmt.Errorf("load probe lease auth state: %w",err)}
	return state,nil
}

func (s *Store) AdvanceProbeLeaseSequence(ctx context.Context,nodeID string,sequence int64)error{
	tag,err:=s.DB.Exec(ctx,`
		UPDATE nodes
		SET probe_lease_sequence=$2,last_seen_at=now(),updated_at=now()
		WHERE id=$1 AND role='probe' AND probe_lease_sequence<$2
	`,nodeID,sequence)
	if err!=nil{return err}
	if tag.RowsAffected()!=1{return fmt.Errorf("stale probe lease sequence")}
	return nil
}
