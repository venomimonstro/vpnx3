package store

import (
	"context"
	"time"

	"github.com/venomimonstro/vpnx3/internal/revocations"
)

func (s *Store) RevokedDeviceHashes(ctx context.Context,since time.Time,limit int)([]string,error){
	if limit<=0||limit>10000{limit=10000}
	rows,err:=s.DB.Query(ctx,`
		SELECT id::text
		FROM devices
		WHERE status='revoked'
		  AND revoked_at IS NOT NULL
		  AND revoked_at >= $1
		ORDER BY revoked_at DESC
		LIMIT $2
	`,since,limit)
	if err!=nil{return nil,err}
	defer rows.Close()
	out:=make([]string,0)
	for rows.Next(){
		var id string
		if err:=rows.Scan(&id);err!=nil{return nil,err}
		out=append(out,revocations.DeviceHash(id))
	}
	return out,rows.Err()
}
