package leadership

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Runner struct{
	db *pgxpool.Pool
	logger *slog.Logger
	lockID int64
	retry time.Duration
	ping time.Duration
	name string
	instanceID string
}

func New(db *pgxpool.Pool,logger *slog.Logger,lockID int64)*Runner{
	return NewNamed(db,logger,lockID,"singleton-jobs")
}

func NewNamed(db *pgxpool.Pool,logger *slog.Logger,lockID int64,name string)*Runner{
	return &Runner{
		db:db,logger:logger,lockID:lockID,retry:5*time.Second,ping:10*time.Second,
		name:name,instanceID:instanceID(),
	}
}

func (r *Runner) Run(ctx context.Context,run func(context.Context)){
	for{
		if ctx.Err()!=nil{return}
		conn,err:=r.db.Acquire(ctx)
		if err!=nil{
			r.logger.Warn("leadership connection acquire failed","error",err)
			if !sleep(ctx,r.retry){return}
			continue
		}

		var leader bool
		err=conn.QueryRow(ctx,`SELECT pg_try_advisory_lock($1)`,r.lockID).Scan(&leader)
		if err!=nil{
			conn.Release()
			r.logger.Warn("leadership lock check failed","error",err)
			if !sleep(ctx,r.retry){return}
			continue
		}
		if !leader{
			conn.Release()
			if !sleep(ctx,r.retry){return}
			continue
		}

		if err:=r.markAcquired(ctx,conn);err!=nil{
			r.logger.Warn("leadership state persist failed","error",err)
		}
		r.logger.Info("control plane leadership acquired","name",r.name,"holder_id",r.instanceID)
		leaderCtx,cancel:=context.WithCancel(ctx)
		done:=make(chan struct{})
		go func(){defer close(done);run(leaderCtx)}()

		ticker:=time.NewTicker(r.ping)
		lost:=false
		for !lost{
			select{
			case <-ctx.Done():
				lost=true
			case <-done:
				lost=true
			case <-ticker.C:
				pingCtx,pingCancel:=context.WithTimeout(ctx,3*time.Second)
				var one int
				pingErr:=conn.QueryRow(pingCtx,`SELECT 1`).Scan(&one)
				if pingErr==nil&&one==1{
					pingErr=r.markHeartbeat(pingCtx,conn)
				}
				pingCancel()
				if pingErr!=nil||one!=1{
					r.logger.Warn("control plane leadership lost","error",pingErr)
					lost=true
				}
			}
		}
		ticker.Stop()
		cancel()
		select{
		case <-done:
		case <-time.After(5*time.Second):
			r.logger.Warn("leader jobs did not stop within grace period")
		}

		unlockCtx,unlockCancel:=context.WithTimeout(context.Background(),2*time.Second)
		_ = r.markReleased(unlockCtx,conn)
		_,_ = conn.Exec(unlockCtx,`SELECT pg_advisory_unlock($1)`,r.lockID)
		unlockCancel()
		conn.Release()
		r.logger.Info("control plane leadership released","name",r.name,"holder_id",r.instanceID)

		if ctx.Err()!=nil{return}
		if !sleep(ctx,r.retry){return}
	}
}

func sleep(ctx context.Context,d time.Duration)bool{
	t:=time.NewTimer(d);defer t.Stop()
	select{
	case <-ctx.Done():return false
	case <-t.C:return true
	}
}


func (r *Runner) markAcquired(ctx context.Context,conn *pgxpool.Conn)error{
	_,err:=conn.Exec(ctx,`
		INSERT INTO control_plane_leadership(name,holder_id,acquired_at,heartbeat_at,transitions,updated_at)
		VALUES($1,$2,now(),now(),1,now())
		ON CONFLICT(name) DO UPDATE
		SET transitions=control_plane_leadership.transitions+
		      CASE WHEN control_plane_leadership.holder_id IS DISTINCT FROM EXCLUDED.holder_id THEN 1 ELSE 0 END,
		    holder_id=EXCLUDED.holder_id,
		    acquired_at=CASE
		      WHEN control_plane_leadership.holder_id IS DISTINCT FROM EXCLUDED.holder_id THEN now()
		      ELSE COALESCE(control_plane_leadership.acquired_at,now())
		    END,
		    heartbeat_at=now(),
		    updated_at=now()
	`,r.name,r.instanceID)
	return err
}

func (r *Runner) markHeartbeat(ctx context.Context,conn *pgxpool.Conn)error{
	tag,err:=conn.Exec(ctx,`
		UPDATE control_plane_leadership
		SET heartbeat_at=now(),updated_at=now()
		WHERE name=$1 AND holder_id=$2
	`,r.name,r.instanceID)
	if err!=nil{return err}
	if tag.RowsAffected()!=1{return fmt.Errorf("leadership holder changed")}
	return nil
}

func (r *Runner) markReleased(ctx context.Context,conn *pgxpool.Conn)error{
	_,err:=conn.Exec(ctx,`
		UPDATE control_plane_leadership
		SET holder_id=NULL,heartbeat_at=NULL,updated_at=now()
		WHERE name=$1 AND holder_id=$2
	`,r.name,r.instanceID)
	return err
}

func instanceID()string{
	host,_:=os.Hostname()
	if host==""{host="unknown"}
	raw:=make([]byte,6)
	if _,err:=rand.Read(raw);err!=nil{
		return fmt.Sprintf("%s-%d",host,os.Getpid())
	}
	return fmt.Sprintf("%s-%d-%s",host,os.Getpid(),hex.EncodeToString(raw))
}
