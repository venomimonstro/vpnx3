package store

import (
	"context"
	"time"
)

func (s *Store) AllowPaymentCreation(ctx context.Context,userID string,now time.Time,limit int)(bool,int,error){
	if limit<=0{limit=10}
	var count int
	err:=s.DB.QueryRow(ctx,`
		SELECT count(*)::int
		FROM payments
		WHERE user_id=$1 AND created_at >= $2
	`,userID,now.UTC().Add(-time.Hour)).Scan(&count)
	if err!=nil{return false,0,err}
	return count<limit,count,nil
}
