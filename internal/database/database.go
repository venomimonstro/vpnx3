package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PoolOptions struct {
	MaxConns int32
	MinConns int32
	MaxConnLifetime time.Duration
	MaxConnLifetimeJitter time.Duration
	MaxConnIdleTime time.Duration
	HealthCheckPeriod time.Duration
}

func DefaultPoolOptions()PoolOptions{
	return PoolOptions{
		MaxConns:20,MinConns:2,
		MaxConnLifetime:30*time.Minute,
		MaxConnLifetimeJitter:5*time.Minute,
		MaxConnIdleTime:5*time.Minute,
		HealthCheckPeriod:30*time.Second,
	}
}

func Open(ctx context.Context,databaseURL string)(*pgxpool.Pool,error){
	return OpenWithOptions(ctx,databaseURL,DefaultPoolOptions())
}

func OpenWithOptions(ctx context.Context,databaseURL string,opts PoolOptions)(*pgxpool.Pool,error){
	if databaseURL==""{return nil,fmt.Errorf("VPNX3_DATABASE_URL must not be empty")}
	cfg,err:=pgxpool.ParseConfig(databaseURL)
	if err!=nil{return nil,fmt.Errorf("parse database config: %w",err)}

	defaults:=DefaultPoolOptions()
	if opts.MaxConns<=0{opts.MaxConns=defaults.MaxConns}
	if opts.MinConns<0{opts.MinConns=0}
	if opts.MinConns>opts.MaxConns{opts.MinConns=opts.MaxConns}
	if opts.MaxConnLifetime<=0{opts.MaxConnLifetime=defaults.MaxConnLifetime}
	if opts.MaxConnLifetimeJitter<0{opts.MaxConnLifetimeJitter=0}
	if opts.MaxConnIdleTime<=0{opts.MaxConnIdleTime=defaults.MaxConnIdleTime}
	if opts.HealthCheckPeriod<=0{opts.HealthCheckPeriod=defaults.HealthCheckPeriod}

	cfg.MaxConns=opts.MaxConns
	cfg.MinConns=opts.MinConns
	cfg.MaxConnLifetime=opts.MaxConnLifetime
	cfg.MaxConnLifetimeJitter=opts.MaxConnLifetimeJitter
	cfg.MaxConnIdleTime=opts.MaxConnIdleTime
	cfg.HealthCheckPeriod=opts.HealthCheckPeriod

	pool,err:=pgxpool.NewWithConfig(ctx,cfg)
	if err!=nil{return nil,fmt.Errorf("create database pool: %w",err)}

	pingCtx,cancel:=context.WithTimeout(ctx,5*time.Second)
	defer cancel()
	if err:=pool.Ping(pingCtx);err!=nil{
		pool.Close()
		return nil,fmt.Errorf("database ping: %w",err)
	}
	return pool,nil
}

func OpenHealth(ctx context.Context,databaseURL string)(*pgxpool.Pool,error){
	if databaseURL==""{return nil,fmt.Errorf("database health URL must not be empty")}
	cfg,err:=pgxpool.ParseConfig(databaseURL)
	if err!=nil{return nil,fmt.Errorf("parse health database config: %w",err)}
	cfg.MaxConns=2
	cfg.MinConns=0
	cfg.MaxConnLifetime=15*time.Minute
	cfg.MaxConnLifetimeJitter=2*time.Minute
	cfg.MaxConnIdleTime=2*time.Minute
	cfg.HealthCheckPeriod=30*time.Second
	pool,err:=pgxpool.NewWithConfig(ctx,cfg)
	if err!=nil{return nil,fmt.Errorf("create health database pool: %w",err)}
	pingCtx,cancel:=context.WithTimeout(ctx,5*time.Second);defer cancel()
	if err:=pool.Ping(pingCtx);err!=nil{return pool,fmt.Errorf("health database ping: %w",err)}
	return pool,nil
}
