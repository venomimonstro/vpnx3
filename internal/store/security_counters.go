package store

import "context"

func (s *Store) IncrementSecurityCounter(ctx context.Context,counter string) error {
	_,err:=s.DB.Exec(ctx,`
		INSERT INTO security_counters_daily(day,counter,count,updated_at)
		VALUES(current_date,$1,1,now())
		ON CONFLICT(day,counter) DO UPDATE
		SET count=security_counters_daily.count+1,
		    updated_at=now()
	`,counter)
	return err
}

func (s *Store) SecurityCounter24h(ctx context.Context,counter string)(int64,error){
	var count int64
	err:=s.DB.QueryRow(ctx,`
		SELECT COALESCE(sum(count),0)::bigint
		FROM security_counters_daily
		WHERE counter=$1 AND day>=current_date-1
	`,counter).Scan(&count)
	return count,err
}

func (s *Store) CleanupSecurityCounters(ctx context.Context)(int64,error){
	tag,err:=s.DB.Exec(ctx,`
		DELETE FROM security_counters_daily
		WHERE day < current_date-90
	`)
	if err!=nil{return 0,err}
	return tag.RowsAffected(),nil
}
