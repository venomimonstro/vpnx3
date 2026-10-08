package httpapi

import (
	"net/http"
	"strconv"
)

func (s *Server) handleAdminReferrals(w http.ResponseWriter,r *http.Request){
	limit:=100
	if raw:=r.URL.Query().Get("limit");raw!=""{
		if value,err:=strconv.Atoi(raw);err==nil&&value>0&&value<=500{limit=value}
	}
	summary,rows,err:=s.store.ReferralAdminAnalytics(r.Context(),limit)
	if err!=nil{s.internalError(w,r,err);return}
	writeJSON(w,http.StatusOK,map[string]any{"summary":summary,"redemptions":rows})
}
