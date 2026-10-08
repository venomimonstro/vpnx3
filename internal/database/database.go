package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("VPNX3_DATABASE_URL must not be empty")
	}

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}

	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database ping: %w", err)
	}

	return pool, nil
}


func OpenHealth(ctx context.Context,databaseURL string)(*pgxpool.Pool,error){
	if databaseURL==""{return nil,fmt.Errorf("database health URL must not be empty")}
	cfg,err:=pgxpool.ParseConfig(databaseURL)
	if err!=nil{return nil,fmt.Errorf("parse health database config: %w",err)}
	cfg.MaxConns=2
	cfg.MinConns=0
	cfg.MaxConnLifetime=15*time.Minute
	cfg.MaxConnIdleTime=2*time.Minute
	cfg.HealthCheckPeriod=30*time.Second
	pool,err:=pgxpool.NewWithConfig(ctx,cfg)
	if err!=nil{return nil,fmt.Errorf("create health database pool: %w",err)}
	pingCtx,cancel:=context.WithTimeout(ctx,5*time.Second);defer cancel()
	if err:=pool.Ping(pingCtx);err!=nil{
		return pool,fmt.Errorf("health database ping: %w",err)
	}
	return pool,nil
}
