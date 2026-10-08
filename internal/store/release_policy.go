package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var releaseVersionPattern=regexp.MustCompile(`^[0-9]+(?:\.[0-9]+){0,3}$`)

type ReleasePolicy struct {
	Target string `json:"target"`
	MinimumSupportedVersion string `json:"minimum_supported_version"`
	RecommendedVersion string `json:"recommended_version"`
	RolloutPercent int `json:"rollout_percent"`
	BlockedVersions []string `json:"blocked_versions"`
	Message string `json:"message"`
	UpdatedAt time.Time `json:"updated_at"`
}

func validateReleasePolicyVersion(value string) error {
	value=strings.TrimSpace(value)
	if value==""{return nil}
	if !releaseVersionPattern.MatchString(value){return fmt.Errorf("invalid version %q",value)}
	return nil
}

func validReleaseTarget(target string) bool {
	switch target{
	case "android_apk","android_aab","chrome_zip","firefox_zip","ios_ipa":
		return true
	default:
		return false
	}
}

func normalizeBlockedVersions(values []string)([]string,error){
	seen:=map[string]struct{}{}
	out:=make([]string,0,len(values))
	for _,value:=range values{
		value=strings.TrimSpace(value)
		if value==""{continue}
		if err:=validateReleasePolicyVersion(value);err!=nil{return nil,err}
		if _,ok:=seen[value];ok{continue}
		seen[value]=struct{}{}
		out=append(out,value)
	}
	sort.Strings(out)
	if len(out)>100{return nil,fmt.Errorf("too many blocked versions")}
	return out,nil
}

func (s *Store) ReleasePolicy(ctx context.Context,target string)(ReleasePolicy,error){
	if !validReleaseTarget(target){return ReleasePolicy{},fmt.Errorf("invalid release target")}
	var p ReleasePolicy
	err:=s.DB.QueryRow(ctx,`
		SELECT target,minimum_supported_version,recommended_version,
		       rollout_percent,blocked_versions,message,updated_at
		FROM release_policies WHERE target=$1
	`,target).Scan(
		&p.Target,&p.MinimumSupportedVersion,&p.RecommendedVersion,
		&p.RolloutPercent,&p.BlockedVersions,&p.Message,&p.UpdatedAt,
	)
	if errors.Is(err,pgx.ErrNoRows){
		return ReleasePolicy{
			Target:target,RolloutPercent:100,BlockedVersions:[]string{},
			UpdatedAt:time.Unix(0,0).UTC(),
		},nil
	}
	if err!=nil{return ReleasePolicy{},err}
	return p,nil
}

func (s *Store) SetReleasePolicy(
	ctx context.Context,target,minVersion,recommended string,rollout int,
	blocked []string,message,adminID string,
)(ReleasePolicy,error){
	if !validReleaseTarget(target){return ReleasePolicy{},fmt.Errorf("invalid release target")}
	minVersion=strings.TrimSpace(minVersion)
	recommended=strings.TrimSpace(recommended)
	message=strings.TrimSpace(message)
	if err:=validateReleasePolicyVersion(minVersion);err!=nil{return ReleasePolicy{},err}
	if err:=validateReleasePolicyVersion(recommended);err!=nil{return ReleasePolicy{},err}
	if rollout<0||rollout>100{return ReleasePolicy{},fmt.Errorf("rollout percent must be 0..100")}
	blocked,err:=normalizeBlockedVersions(blocked);if err!=nil{return ReleasePolicy{},err}
	if len(message)>500{return ReleasePolicy{},fmt.Errorf("message too long")}

	tx,err:=s.DB.Begin(ctx);if err!=nil{return ReleasePolicy{},err};defer tx.Rollback(ctx)
	if _,err:=tx.Exec(ctx,`
		INSERT INTO release_policies(
		  target,minimum_supported_version,recommended_version,rollout_percent,
		  blocked_versions,message,updated_at,updated_by
		) VALUES($1,$2,$3,$4,$5,$6,now(),NULLIF($7,'')::uuid)
		ON CONFLICT(target) DO UPDATE SET
		  minimum_supported_version=EXCLUDED.minimum_supported_version,
		  recommended_version=EXCLUDED.recommended_version,
		  rollout_percent=EXCLUDED.rollout_percent,
		  blocked_versions=EXCLUDED.blocked_versions,
		  message=EXCLUDED.message,
		  updated_at=now(),
		  updated_by=EXCLUDED.updated_by
	`,target,minVersion,recommended,rollout,blocked,message,adminID);err!=nil{
		return ReleasePolicy{},err
	}
	if _,err:=tx.Exec(ctx,`
		INSERT INTO release_policy_events(
		  target,minimum_supported_version,recommended_version,rollout_percent,
		  blocked_versions,message,admin_user_id
		) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,'')::uuid)
	`,target,minVersion,recommended,rollout,blocked,message,adminID);err!=nil{
		return ReleasePolicy{},err
	}
	if err:=tx.Commit(ctx);err!=nil{return ReleasePolicy{},err}
	return s.ReleasePolicy(ctx,target)
}
