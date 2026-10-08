package store

import (
	"context"
	"fmt"
	"time"
)

func (s *Store) WithAdvisoryLock(ctx context.Context,key int64,fn func(context.Context)error)error{
	conn,err:=s.DB.Acquire(ctx)
	if err!=nil{return fmt.Errorf("acquire advisory connection: %w",err)}
	defer conn.Release()

	if _,err:=conn.Exec(ctx,`SELECT pg_advisory_lock($1)`,key);err!=nil{
		return fmt.Errorf("acquire advisory lock: %w",err)
	}
	defer func(){
		unlockCtx,cancel:=context.WithTimeout(context.Background(),2*time.Second)
		defer cancel()
		_,_ = conn.Exec(unlockCtx,`SELECT pg_advisory_unlock($1)`,key)
	}()
	return fn(ctx)
}
