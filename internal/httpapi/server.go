package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/venomimonstro/vpnx3/internal/artifactstorage"
	"github.com/venomimonstro/vpnx3/internal/config"
	"github.com/venomimonstro/vpnx3/internal/configservice"
	"github.com/venomimonstro/vpnx3/internal/billing"
	"github.com/venomimonstro/vpnx3/internal/adminui"
	"github.com/venomimonstro/vpnx3/internal/billing/yookassa"
	"github.com/venomimonstro/vpnx3/internal/signing"
	"github.com/venomimonstro/vpnx3/internal/store"
)

type Server struct {
	http *http.Server
	logger *slog.Logger
	db *pgxpool.Pool
	store *store.Store
	cfg config.Config
	configService *configservice.Service
	configSigner *signing.Signer
	accessSigner *signing.Signer
	releaseSigner *signing.Signer
	billing *billing.Service
	yooKassa *yookassa.Adapter
	artifacts artifactstorage.Storage
	onConfigChange func()
}

func NewServer(
	cfg config.Config,
	logger *slog.Logger,
	db *pgxpool.Pool,
	configSigner,accessSigner,releaseSigner *signing.Signer,
	artifacts artifactstorage.Storage,
	onConfigChange ...func(),
) *Server {
	mux:=http.NewServeMux()
	var yoo *yookassa.Adapter
	if cfg.YooKassaShopID!="" {
		var err error
		yoo,err=yookassa.New(cfg.YooKassaShopID,cfg.YooKassaSecretKey,cfg.YooKassaReturnURL)
		if err!=nil { panic(err) }
	}
	s:=&Server{
		logger:logger,db:db,store:store.New(db),cfg:cfg,
		configService:configservice.New(
			store.New(db),
			configSigner,
			configservice.NetworkPolicy{
				DNSServers:cfg.ClientDNS,
				MTU:cfg.WireGuardMTU,
				PersistentKeepalive:cfg.WireGuardKeepalive,
				GatewayIPv4:cfg.WireGuardGateway,
			},
		),
		configSigner:configSigner,accessSigner:accessSigner,releaseSigner:releaseSigner,
		billing:billing.New(store.New(db)),yooKassa:yoo,artifacts:artifacts,
	}
	if len(onConfigChange)>0 {
		s.onConfigChange=onConfigChange[0]
	}
	adminHandler:=http.StripPrefix("/admin/",adminui.Handler())
	mux.HandleFunc("GET /admin",func(w http.ResponseWriter,r *http.Request){ http.Redirect(w,r,"/admin/",http.StatusTemporaryRedirect) })
	mux.Handle("GET /admin/",adminHandler)
	mux.HandleFunc("GET /health/live",s.handleLive)
	mux.HandleFunc("GET /health/ready",s.handleReady)
	mux.HandleFunc("GET /api/v1/meta",func(w http.ResponseWriter,r *http.Request){
		writeJSON(w,http.StatusOK,map[string]any{"name":"VPNX3 Control Plane","api_version":"v1","environment":cfg.Environment,"time":time.Now().UTC()})
	})
	mux.HandleFunc("POST /api/v1/admin/login",s.handleAdminLogin)
	mux.HandleFunc("POST /api/v1/node/enroll",s.handleNodeEnroll)
	mux.HandleFunc("POST /api/v1/node/heartbeat",s.handleNodeHeartbeat)
	mux.HandleFunc("POST /api/v1/node/probe-results",s.handleProbeReport)
	mux.HandleFunc("POST /api/v1/node/probe-lease",s.handleProbeLease)
	mux.HandleFunc("POST /api/v1/build/claim",s.handleBuildClaim)
	mux.HandleFunc("POST /api/v1/build/jobs/{id}/complete",s.handleBuildComplete)
	mux.HandleFunc("PUT /api/v1/build/jobs/{id}/artifact",s.handleBuildArtifact)
	mux.HandleFunc("GET /api/v1/config/latest",s.handleLatestConfig)
	mux.HandleFunc("GET /api/v1/config/signing-key",s.handleConfigSigningKey)
	mux.HandleFunc("POST /api/v1/client/register",s.handleClientRegister)
	mux.HandleFunc("POST /api/v1/client/lease",s.handleClientLease)
	mux.HandleFunc("POST /api/v1/client/proxy-lease",s.handleClientProxyLease)
	mux.HandleFunc("POST /api/v1/client/account/status",s.handleClientAccountStatus)
	mux.HandleFunc("POST /api/v1/client/account/auto-renew",s.handleSetAutoRenew)
	mux.HandleFunc("POST /api/v1/client/devices",s.handleClientDevices)
	mux.HandleFunc("POST /api/v1/client/devices/revoke",s.handleClientRevokeDevice)
	mux.HandleFunc("POST /api/v1/client/pairing-code",s.handleCreatePairingCode)
	mux.HandleFunc("POST /api/v1/client/pairing-claim",s.handleClaimPairingCode)
	mux.HandleFunc("POST /api/v1/client/referral/code",s.handleReferralCode)
	mux.HandleFunc("POST /api/v1/client/referral/claim",s.handleReferralClaim)
	mux.HandleFunc("POST /api/v1/client/referral/status",s.handleReferralStatus)
	mux.HandleFunc("GET /api/v1/access/signing-key",s.handleAccessSigningKey)
	mux.HandleFunc("GET /api/v1/plans",s.handlePublicPlans)
	mux.HandleFunc("GET /api/v1/releases/latest",s.handleLatestRelease)
	mux.HandleFunc("GET /api/v1/releases/policy",s.handleReleasePolicy)
	mux.HandleFunc("GET /api/v1/releases/signing-key",s.handleReleaseSigningKey)
	mux.HandleFunc("GET /api/v1/releases/{id}/artifacts/{artifactId}/download",s.handlePublicArtifactDownload)
	mux.HandleFunc("POST /api/v1/client/payments",s.handleClientCreatePayment)
	mux.HandleFunc("POST /api/v1/client/telemetry",s.handleClientTelemetry)
	mux.HandleFunc("POST /api/v1/webhooks/yookassa",s.handleYooKassaWebhook)
	mux.Handle("GET /api/v1/admin/me",s.requireAdmin(http.HandlerFunc(s.handleAdminMe)))
	mux.Handle("GET /api/v1/admin/dashboard",s.requireAdmin(requirePermission("analytics.read",http.HandlerFunc(s.handleDashboard))))
	mux.Handle("GET /api/v1/admin/issues",s.requireAdmin(requirePermission("analytics.read",http.HandlerFunc(s.handleOperationalIssues))))
	mux.Handle("GET /api/v1/admin/readiness",s.requireAdmin(requirePermission("analytics.read",http.HandlerFunc(s.handleLaunchReadiness))))
	mux.Handle("GET /api/v1/admin/network-risk",s.requireAdmin(requirePermission("analytics.read",http.HandlerFunc(s.handleNetworkRisk))))
	mux.Handle("GET /api/v1/admin/telemetry",s.requireAdmin(requirePermission("analytics.read",http.HandlerFunc(s.handleTelemetrySummary))))
	mux.Handle("GET /api/v1/admin/users",s.requireAdmin(requirePermission("users.read",http.HandlerFunc(s.handleUsers))))
	mux.Handle("GET /api/v1/admin/users/{id}",s.requireAdmin(requirePermission("users.read",http.HandlerFunc(s.handleUserDetail))))
	mux.Handle("GET /api/v1/admin/users/{id}/devices",s.requireAdmin(requirePermission("users.read",http.HandlerFunc(s.handleUserDevices))))
	mux.Handle("POST /api/v1/admin/users/{id}/devices/{deviceId}/revoke",s.requireAdmin(requirePermission("users.manage",http.HandlerFunc(s.handleRevokeUserDevice))))
	mux.Handle("POST /api/v1/admin/users/{id}/devices/{deviceId}/reactivate",s.requireAdmin(requirePermission("users.manage",http.HandlerFunc(s.handleReactivateUserDevice))))
	mux.Handle("GET /api/v1/admin/payments",s.requireAdmin(requirePermission("billing.read",http.HandlerFunc(s.handlePayments))))
	mux.Handle("GET /api/v1/admin/audit",s.requireAdmin(requirePermission("audit.read",http.HandlerFunc(s.handleAudit))))
	mux.Handle("GET /api/v1/admin/security-export",s.requireAdmin(requirePermission("audit.read",http.HandlerFunc(s.handleSecurityExportStatus))))
	mux.Handle("POST /api/v1/admin/security-export/requeue",s.requireAdmin(requirePermission("security.export.manage",http.HandlerFunc(s.handleRequeueSecurityExport))))
	mux.Handle("GET /api/v1/admin/incidents",s.requireAdmin(requirePermission("incidents.read",http.HandlerFunc(s.handleIncidents))))
	mux.Handle("GET /api/v1/admin/incident-notifications",s.requireAdmin(requirePermission("incidents.read",http.HandlerFunc(s.handleIncidentNotificationStatus))))
	mux.Handle("POST /api/v1/admin/incident-notifications/requeue",s.requireAdmin(requirePermission("incidents.manage",http.HandlerFunc(s.handleRequeueIncidentNotifications))))
	mux.Handle("GET /api/v1/admin/releases",s.requireAdmin(requirePermission("releases.read",http.HandlerFunc(s.handleReleases))))
	mux.Handle("GET /api/v1/admin/release-policy",s.requireAdmin(requirePermission("releases.read",http.HandlerFunc(s.handleAdminReleasePolicy))))
	mux.Handle("PUT /api/v1/admin/release-policy",s.requireAdmin(requirePermission("releases.manage",http.HandlerFunc(s.handleSetReleasePolicy))))
	mux.Handle("GET /api/v1/admin/releases/{id}/jobs",s.requireAdmin(requirePermission("releases.read",http.HandlerFunc(s.handleReleaseJobs))))
	mux.Handle("GET /api/v1/admin/releases/{id}/artifacts",s.requireAdmin(requirePermission("releases.read",http.HandlerFunc(s.handleReleaseArtifacts))))
	mux.Handle("GET /api/v1/admin/releases/{id}/gate",s.requireAdmin(requirePermission("releases.read",http.HandlerFunc(s.handleReleaseGate))))
	mux.Handle("GET /api/v1/admin/releases/{id}/artifacts/{artifactId}/download",s.requireAdmin(requirePermission("releases.read",http.HandlerFunc(s.handleDownloadArtifact))))
	mux.Handle("POST /api/v1/admin/releases/{id}/publish",s.requireAdmin(requirePermission("releases.manage",http.HandlerFunc(s.handlePublishRelease))))
	mux.Handle("POST /api/v1/admin/releases/{id}/withdraw",s.requireAdmin(requirePermission("releases.manage",http.HandlerFunc(s.handleWithdrawRelease))))
	mux.Handle("POST /api/v1/admin/releases",s.requireAdmin(requirePermission("releases.manage",http.HandlerFunc(s.handleCreateRelease))))
	mux.Handle("POST /api/v1/admin/releases/{id}/jobs/{jobId}/retry",s.requireAdmin(requirePermission("releases.manage",http.HandlerFunc(s.handleRetryBuildJob))))
	mux.Handle("POST /api/v1/admin/incidents",s.requireAdmin(requirePermission("incidents.manage",http.HandlerFunc(s.handleCreateIncident))))
	mux.Handle("POST /api/v1/admin/incidents/{id}/resolve",s.requireAdmin(requirePermission("incidents.manage",http.HandlerFunc(s.handleResolveIncident))))
	mux.Handle("POST /api/v1/admin/logout",s.requireAdmin(http.HandlerFunc(s.handleAdminLogout)))
	mux.Handle("GET /api/v1/admin/admins",s.requireAdmin(requirePermission("admin.manage",http.HandlerFunc(s.handleListAdmins))))
	mux.Handle("POST /api/v1/admin/admins",s.requireAdmin(requirePermission("admin.manage",http.HandlerFunc(s.handleCreateAdmin))))
	mux.Handle("PUT /api/v1/admin/admins/{id}/roles",s.requireAdmin(requirePermission("admin.manage",http.HandlerFunc(s.handleSetAdminRoles))))
	mux.Handle("PUT /api/v1/admin/admins/{id}/status",s.requireAdmin(requirePermission("admin.manage",http.HandlerFunc(s.handleSetAdminStatus))))
	mux.Handle("POST /api/v1/config/publish",s.requireAdmin(requirePermission("config.manage",http.HandlerFunc(s.handlePublishConfig))))
	mux.Handle("GET /api/v1/probes/recent",s.requireAdmin(requirePermission("nodes.read",http.HandlerFunc(s.handleRecentProbeResults))))
	mux.Handle("GET /api/v1/billing/plans",s.requireAdmin(requirePermission("billing.read",http.HandlerFunc(s.handleListPlans))))
	mux.Handle("GET /api/v1/admin/finance/summary",s.requireAdmin(requirePermission("billing.read",http.HandlerFunc(s.handleFinanceSummary))))
	mux.Handle("GET /api/v1/admin/referrals",s.requireAdmin(requirePermission("analytics.read",http.HandlerFunc(s.handleAdminReferrals))))
	mux.Handle("POST /api/v1/billing/plans",s.requireAdmin(requirePermission("billing.manage",http.HandlerFunc(s.handleCreatePlan))))
	mux.Handle("GET /api/v1/nodes",s.requireAdmin(requirePermission("nodes.read",http.HandlerFunc(s.handleListNodes))))
	mux.Handle("GET /api/v1/nodes/{id}",s.requireAdmin(requirePermission("nodes.read",http.HandlerFunc(s.handleGetNode))))
	mux.Handle("GET /api/v1/nodes/{id}/events",s.requireAdmin(requirePermission("nodes.read",http.HandlerFunc(s.handleNodeEvents))))
	mux.Handle("GET /api/v1/nodes/{id}/endpoints",s.requireAdmin(requirePermission("nodes.read",http.HandlerFunc(s.handleListNodeEndpoints))))
	mux.Handle("POST /api/v1/nodes/{id}/endpoints",s.requireAdmin(requirePermission("nodes.manage",http.HandlerFunc(s.handleCreateNodeEndpoint))))
	mux.Handle("DELETE /api/v1/nodes/{id}/endpoints/{endpointId}",s.requireAdmin(requirePermission("nodes.manage",http.HandlerFunc(s.handleDeleteNodeEndpoint))))
	mux.Handle("POST /api/v1/nodes/enrollment-tokens",s.requireAdmin(requirePermission("nodes.manage",http.HandlerFunc(s.handleCreateEnrollmentToken))))
	mux.Handle("POST /api/v1/nodes/{id}/approve",s.requireAdmin(requirePermission("nodes.manage",s.handleNodeTransition("draft"))))
	mux.Handle("POST /api/v1/nodes/{id}/publish",s.requireAdmin(requirePermission("nodes.manage",s.handleNodeTransition("active"))))
	mux.Handle("POST /api/v1/nodes/{id}/drain",s.requireAdmin(requirePermission("nodes.manage",s.handleNodeTransition("draining"))))
	mux.Handle("POST /api/v1/nodes/{id}/maintenance",s.requireAdmin(requirePermission("nodes.manage",s.handleNodeTransition("maintenance"))))
	mux.Handle("POST /api/v1/nodes/{id}/quarantine",s.requireAdmin(requirePermission("nodes.manage",s.handleNodeTransition("quarantined"))))
	mux.Handle("POST /api/v1/nodes/{id}/retire",s.requireAdmin(requirePermission("nodes.manage",s.handleNodeTransition("retired"))))

	handler:=trustedProxyContext(cfg.TrustedProxyCIDRs,requestContext(securityHeaders(requestLog(logger,recoverer(logger,mux)))))
	s.http=&http.Server{Addr:cfg.HTTPAddr,Handler:handler,ReadTimeout:cfg.ReadTimeout,WriteTimeout:cfg.WriteTimeout,IdleTimeout:cfg.IdleTimeout,ReadHeaderTimeout:5*time.Second}
	return s
}

func (s *Server) handleLive(w http.ResponseWriter,r *http.Request){ writeJSON(w,http.StatusOK,map[string]any{"status":"ok","service":"control-plane"}) }
func (s *Server) handleReady(w http.ResponseWriter,r *http.Request){
	ctx,cancel:=context.WithTimeout(r.Context(),3*time.Second); defer cancel()
	if err:=s.db.Ping(ctx); err!=nil {
		s.logger.Warn("readiness database check failed","request_id",requestIDFromContext(r.Context()),"error",err)
		writeJSON(w,http.StatusServiceUnavailable,map[string]any{"status":"not_ready","database":"unavailable","artifacts":"unknown"}); return
	}
	if checker,ok:=s.artifacts.(artifactstorage.ReadinessChecker);ok{
		if err:=checker.Check(ctx);err!=nil{
			s.logger.Warn("readiness artifact storage check failed","request_id",requestIDFromContext(r.Context()),"error",err)
			writeJSON(w,http.StatusServiceUnavailable,map[string]any{"status":"not_ready","database":"ok","artifacts":"unavailable"});return
		}
	}
	writeJSON(w,http.StatusOK,map[string]any{"status":"ready","database":"ok","artifacts":"ok"})
}
func (s *Server) internalError(w http.ResponseWriter,r *http.Request,err error){
	s.logger.Error("request failed","request_id",requestIDFromContext(r.Context()),"error",err)
	writeError(w,http.StatusInternalServerError,"internal_server_error")
}
func (s *Server) ListenAndServe() error { err:=s.http.ListenAndServe(); if err==http.ErrServerClosed{return nil}; return err }
func (s *Server) Shutdown(ctx context.Context) error { return s.http.Shutdown(ctx) }
func writeJSON(w http.ResponseWriter,status int,payload any){
	w.Header().Set("Content-Type","application/json; charset=utf-8"); w.WriteHeader(status); _=json.NewEncoder(w).Encode(payload)
}
