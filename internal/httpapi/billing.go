package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/venomimonstro/vpnx3/internal/store"
)

func (s *Server) handleListPlans(w http.ResponseWriter,r *http.Request) {
	plans,err:=s.store.ListPlans(r.Context(),false)
	if err!=nil { s.internalError(w,r,err); return }
	writeJSON(w,http.StatusOK,map[string]any{"plans":plans})
}

func (s *Server) handleCreatePlan(w http.ResponseWriter,r *http.Request) {
	var req struct {
		Code string `json:"code"`
		Name string `json:"name"`
		PriceMinor int64 `json:"price_minor"`
		Currency string `json:"currency"`
		BillingPeriodDays int `json:"billing_period_days"`
		DeviceLimit int `json:"device_limit"`
		TrialDays int `json:"trial_days"`
		GraceDays int `json:"grace_days"`
	}
	if err:=decodeJSON(w,r,&req); err!=nil { return }
	plan,err:=s.store.CreatePlanVersion(r.Context(),store.CreatePlanInput{
		Code:req.Code,Name:req.Name,PriceMinor:req.PriceMinor,Currency:req.Currency,
		BillingPeriodDays:req.BillingPeriodDays,DeviceLimit:req.DeviceLimit,
		TrialDays:req.TrialDays,GraceDays:req.GraceDays,
	})
	if err!=nil {
		if strings.Contains(err.Error(),"invalid") {
			writeJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid_plan","detail":err.Error()})
			return
		}
		s.internalError(w,r,err)
		return
	}
	admin,_:=adminFromContext(r.Context())
	_ = s.store.WriteAudit(r.Context(),"admin",admin.ID,"billing.plan.create","plan",plan.ID,
		requestIDFromContext(r.Context()),ipString(clientIP(r)),"success")
	writeJSON(w,http.StatusCreated,plan)
}


func (s *Server) handleFinanceSummary(w http.ResponseWriter,r *http.Request){
	days:=30
	if raw:=r.URL.Query().Get("days");raw!=""{if v,err:=strconv.Atoi(raw);err==nil{days=v}}
	summary,err:=s.store.FinanceSummary(r.Context(),days)
	if err!=nil{s.internalError(w,r,err);return}
	writeJSON(w,http.StatusOK,summary)
}
