package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/venomimonstro/vpnx3/internal/accountcleaner"
	"github.com/venomimonstro/vpnx3/internal/alertnotifier"
	"github.com/venomimonstro/vpnx3/internal/artifactcleaner"
	"github.com/venomimonstro/vpnx3/internal/artifactstorage"
	"github.com/venomimonstro/vpnx3/internal/bootstrap"
	"github.com/venomimonstro/vpnx3/internal/buildwatchdog"
	"github.com/venomimonstro/vpnx3/internal/config"
	"github.com/venomimonstro/vpnx3/internal/configpublisher"
	"github.com/venomimonstro/vpnx3/internal/configservice"
	"github.com/venomimonstro/vpnx3/internal/database"
	"github.com/venomimonstro/vpnx3/internal/httpapi"
	"github.com/venomimonstro/vpnx3/internal/nodemonitor"
	"github.com/venomimonstro/vpnx3/internal/loginthrottle"
	"github.com/venomimonstro/vpnx3/internal/leadership"
	"github.com/venomimonstro/vpnx3/internal/probemonitor"
	"github.com/venomimonstro/vpnx3/internal/renewal"
	"github.com/venomimonstro/vpnx3/internal/billing"
	"github.com/venomimonstro/vpnx3/internal/billing/yookassa"
	"github.com/venomimonstro/vpnx3/internal/signing"
	"github.com/venomimonstro/vpnx3/internal/securityexport"
	"github.com/venomimonstro/vpnx3/internal/store"
	"github.com/venomimonstro/vpnx3/internal/systemmonitor"
	"github.com/venomimonstro/vpnx3/internal/trustbundle"
)

func main() {
	cfg, err := config.Load()
	if err != nil { panic(err) }

	logger := slog.New(slog.NewJSONHandler(os.Stdout,&slog.HandlerOptions{Level:cfg.LogLevel}))

	startupCtx, startupCancel := context.WithTimeout(context.Background(),15*time.Second)
	defer startupCancel()

	db, err := database.Open(startupCtx,cfg.DatabaseURL)
	if err != nil {
		logger.Error("database initialization failed","error",err)
		os.Exit(1)
	}
	defer db.Close()

	if err := bootstrap.EnsureOwner(startupCtx,store.New(db),logger,cfg.BootstrapOwnerEmail,cfg.BootstrapOwnerPassword); err != nil {
		logger.Error("bootstrap initialization failed","error",err)
		os.Exit(1)
	}

	configSigner,err:=signing.FromSeedBase64(cfg.ConfigSigningKey)
	if err!=nil {
		logger.Error("config signing key initialization failed","error",err)
		os.Exit(1)
	}

	accessSigner,err:=signing.FromSeedBase64(cfg.AccessSigningKey)
	if err!=nil {
		logger.Error("access signing key initialization failed","error",err)
		os.Exit(1)
	}

	var releaseSigner *signing.Signer
	if cfg.ReleaseSigningKey!="" {
		releaseSigner,err=signing.FromSeedBase64(cfg.ReleaseSigningKey)
		if err!=nil {
			logger.Error("release signing key initialization failed","error",err)
			os.Exit(1)
		}
	}

	var trustEnvelope *trustbundle.Envelope
	if cfg.TrustRootPublicKey!=""{
		env,payload,trustErr:=trustbundle.LoadFile(
			cfg.TrustBundleFile,cfg.TrustRootPublicKey,0,time.Now().UTC(),
		)
		if trustErr!=nil{
			logger.Error("trust bundle verification failed","error",trustErr)
			os.Exit(1)
		}
		if err:=trustbundle.MatchesSigner(payload,"config",configSigner);err!=nil{
			logger.Error("config signer is not authorized by trust bundle","error",err);os.Exit(1)
		}
		if err:=trustbundle.MatchesSigner(payload,"access",accessSigner);err!=nil{
			logger.Error("access signer is not authorized by trust bundle","error",err);os.Exit(1)
		}
		if releaseSigner!=nil{
			if err:=trustbundle.MatchesSigner(payload,"release",releaseSigner);err!=nil{
				logger.Error("release signer is not authorized by trust bundle","error",err);os.Exit(1)
			}
		}
		trustEnvelope=&env
		logger.Info("offline-root trust bundle verified","version",payload.Version,"expires_at",payload.ExpiresAt)
	}

	var artifactStorage artifactstorage.Storage
	switch cfg.ArtifactStorage {
	case "local":
		artifactStorage,err=artifactstorage.NewLocal(cfg.ArtifactDir)
	case "s3":
		artifactStorage,err=artifactstorage.NewS3(artifactstorage.S3Config{
			Endpoint:cfg.ArtifactS3Endpoint,
			Region:cfg.ArtifactS3Region,
			Bucket:cfg.ArtifactS3Bucket,
			AccessKey:cfg.ArtifactS3AccessKey,
			SecretKey:cfg.ArtifactS3SecretKey,
			TempDir:cfg.ArtifactS3TempDir,
		})
	default:
		err=fmt.Errorf("unsupported artifact storage %q",cfg.ArtifactStorage)
	}
	if err!=nil{
		logger.Error("artifact storage initialization failed","backend",cfg.ArtifactStorage,"error",err)
		os.Exit(1)
	}

	nodeStore := store.New(db)
	configurationService:=configservice.New(
		nodeStore,
		configSigner,
		configservice.NetworkPolicy{
			DNSServers:cfg.ClientDNS,
			MTU:cfg.WireGuardMTU,
			PersistentKeepalive:cfg.WireGuardKeepalive,
			GatewayIPv4:cfg.WireGuardGateway,
		},
	)
	publisher:=configpublisher.New(configurationService,logger,15*time.Minute)

	monitorCtx, monitorCancel := context.WithCancel(context.Background())
	defer monitorCancel()
	go publisher.Run(monitorCtx)

	const singletonLeadershipLock int64 = 0x56504e5833434841
	go leadership.New(db,logger,singletonLeadershipLock).Run(monitorCtx,func(leaderCtx context.Context){
		var wg sync.WaitGroup
		jobs:=[]func(context.Context){
			func(ctx context.Context){nodemonitor.New(nodeStore,logger,publisher.Trigger).Run(ctx)},
			func(ctx context.Context){loginthrottle.New(nodeStore,logger).Run(ctx)},
			func(ctx context.Context){probemonitor.New(nodeStore,logger,publisher.Trigger).Run(ctx)},
			func(ctx context.Context){artifactcleaner.New(nodeStore,logger,artifactStorage,cfg.ArtifactRetentionDays).Run(ctx)},
			func(ctx context.Context){accountcleaner.New(nodeStore,logger).Run(ctx)},
			func(ctx context.Context){buildwatchdog.New(nodeStore,logger,2*time.Minute).Run(ctx)},
			func(ctx context.Context){systemmonitor.New(nodeStore,logger,cfg.BackupStatusFile).Run(ctx)},
		}
		for _,job:=range jobs{
			wg.Add(1)
			go func(run func(context.Context)){defer wg.Done();run(leaderCtx)}(job)
		}
		<-leaderCtx.Done()
		wg.Wait()
	})
	if cfg.AlertWebhookURL!="" {
		go alertnotifier.New(nodeStore,cfg.AlertWebhookURL,cfg.AlertWebhookSecret,logger).Run(monitorCtx)
	}
	if cfg.SecurityExportURL!="" {
		securitySigner,signErr:=signing.FromSeedBase64(cfg.SecurityExportSigningKey)
		if signErr!=nil{
			logger.Error("security export signing key initialization failed","error",signErr)
			os.Exit(1)
		}
		go securityexport.New(nodeStore,securitySigner,cfg.SecurityExportURL,logger).Run(monitorCtx)
	}
	if cfg.YooKassaShopID!="" {
		yoo,renewErr:=yookassa.New(cfg.YooKassaShopID,cfg.YooKassaSecretKey,cfg.YooKassaReturnURL)
		if renewErr!=nil{
			logger.Error("renewal provider initialization failed","error",renewErr)
			os.Exit(1)
		}
		go renewal.New(nodeStore,billing.New(nodeStore),yoo,logger,5*time.Minute).Run(monitorCtx)
	}

	srv := httpapi.NewServer(cfg,logger,db,configSigner,accessSigner,releaseSigner,trustEnvelope,artifactStorage,publisher.Trigger)
	serverErr := make(chan error,1)
	go func() {
		logger.Info("control plane starting","addr",cfg.HTTPAddr,"env",cfg.Environment)
		serverErr <- srv.ListenAndServe()
	}()

	stop := make(chan os.Signal,1)
	signal.Notify(stop,syscall.SIGINT,syscall.SIGTERM)
	select {
	case sig := <-stop:
		logger.Info("shutdown signal received","signal",sig.String())
	case err := <-serverErr:
		if err != nil {
			logger.Error("http server stopped unexpectedly","error",err)
			os.Exit(1)
		}
		return
	}

	ctx,cancel := context.WithTimeout(context.Background(),10*time.Second)
	defer cancel()
	logger.Info("control plane shutting down")
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed","error",err)
		os.Exit(1)
	}
}
