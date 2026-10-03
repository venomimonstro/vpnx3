package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type AdminListRow struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Status    string    `json:"status"`
	Roles     []string  `json:"roles"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Store) ListAdmins(ctx context.Context) ([]AdminListRow,error) {
	rows,err:=s.DB.Query(ctx,`
		SELECT u.id::text,u.email,u.status,
		       COALESCE(array_agg(r.code ORDER BY r.code) FILTER(WHERE r.code IS NOT NULL),'{}'),
		       u.created_at,u.updated_at
		FROM admin_users u
		LEFT JOIN admin_user_roles aur ON aur.admin_user_id=u.id
		LEFT JOIN roles r ON r.id=aur.role_id
		GROUP BY u.id
		ORDER BY u.created_at
	`)
	if err!=nil{return nil,err}
	defer rows.Close()
	out:=make([]AdminListRow,0)
	for rows.Next(){
		var a AdminListRow
		if err:=rows.Scan(&a.ID,&a.Email,&a.Status,&a.Roles,&a.CreatedAt,&a.UpdatedAt);err!=nil{return nil,err}
		out=append(out,a)
	}
	return out,rows.Err()
}

func (s *Store) CreateAdmin(ctx context.Context,email,passwordHash string,roles []string) (AdminListRow,error) {
	email=strings.ToLower(strings.TrimSpace(email))
	if email=="" || passwordHash=="" || len(roles)==0{return AdminListRow{},fmt.Errorf("invalid admin")}
	tx,err:=s.DB.Begin(ctx);if err!=nil{return AdminListRow{},err};defer tx.Rollback(ctx)
	roleIDs,err:=resolveRoleIDs(ctx,tx,roles);if err!=nil{return AdminListRow{},err}
	var id string
	if err:=tx.QueryRow(ctx,`
		INSERT INTO admin_users(email,password_hash,status)
		VALUES($1,$2,'active') RETURNING id::text
	`,email,passwordHash).Scan(&id);err!=nil{return AdminListRow{},fmt.Errorf("create admin: %w",err)}
	for _,roleID:=range roleIDs{
		if _,err:=tx.Exec(ctx,"INSERT INTO admin_user_roles(admin_user_id,role_id) VALUES($1,$2)",id,roleID);err!=nil{return AdminListRow{},err}
	}
	if err:=tx.Commit(ctx);err!=nil{return AdminListRow{},err}
	return s.adminListByID(ctx,id)
}

func (s *Store) SetAdminRoles(ctx context.Context,targetID,actingID string,roles []string) (AdminListRow,error) {
	if targetID==actingID{return AdminListRow{},fmt.Errorf("cannot change own roles")}
	if len(roles)==0{return AdminListRow{},fmt.Errorf("at least one role is required")}
	tx,err:=s.DB.Begin(ctx);if err!=nil{return AdminListRow{},err};defer tx.Rollback(ctx)

	var currentStatus string
	var isOwner bool
	err=tx.QueryRow(ctx,`
		SELECT u.status,EXISTS(
		  SELECT 1 FROM admin_user_roles aur JOIN roles r ON r.id=aur.role_id
		  WHERE aur.admin_user_id=u.id AND r.code='owner'
		)
		FROM admin_users u WHERE u.id=$1 FOR UPDATE
	`,targetID).Scan(&currentStatus,&isOwner)
	if errors.Is(err,pgx.ErrNoRows){return AdminListRow{},ErrNotFound}
	if err!=nil{return AdminListRow{},err}

	roleIDs,err:=resolveRoleIDs(ctx,tx,roles);if err!=nil{return AdminListRow{},err}
	newOwner:=containsRole(roles,"owner")
	if isOwner && !newOwner && currentStatus=="active" {
		var others int
		if err:=tx.QueryRow(ctx,`
			SELECT count(*)::int
			FROM admin_users u
			WHERE u.status='active' AND u.id<>$1
			  AND EXISTS(
			    SELECT 1 FROM admin_user_roles aur JOIN roles r ON r.id=aur.role_id
			    WHERE aur.admin_user_id=u.id AND r.code='owner'
			  )
		`,targetID).Scan(&others);err!=nil{return AdminListRow{},err}
		if others<1{return AdminListRow{},fmt.Errorf("cannot remove last active owner")}
	}

	if _,err:=tx.Exec(ctx,"DELETE FROM admin_user_roles WHERE admin_user_id=$1",targetID);err!=nil{return AdminListRow{},err}
	for _,roleID:=range roleIDs{
		if _,err:=tx.Exec(ctx,"INSERT INTO admin_user_roles(admin_user_id,role_id) VALUES($1,$2)",targetID,roleID);err!=nil{return AdminListRow{},err}
	}
	if _,err:=tx.Exec(ctx,"UPDATE admin_users SET updated_at=now() WHERE id=$1",targetID);err!=nil{return AdminListRow{},err}
	if err:=tx.Commit(ctx);err!=nil{return AdminListRow{},err}
	return s.adminListByID(ctx,targetID)
}

func (s *Store) SetAdminStatus(ctx context.Context,targetID,actingID,status string) (AdminListRow,error) {
	if targetID==actingID{return AdminListRow{},fmt.Errorf("cannot change own status")}
	if status!="active" && status!="disabled"{return AdminListRow{},fmt.Errorf("invalid admin status")}
	tx,err:=s.DB.Begin(ctx);if err!=nil{return AdminListRow{},err};defer tx.Rollback(ctx)
	var current string
	var owner bool
	err=tx.QueryRow(ctx,`
		SELECT u.status,EXISTS(
		  SELECT 1 FROM admin_user_roles aur JOIN roles r ON r.id=aur.role_id
		  WHERE aur.admin_user_id=u.id AND r.code='owner'
		)
		FROM admin_users u WHERE u.id=$1 FOR UPDATE
	`,targetID).Scan(&current,&owner)
	if errors.Is(err,pgx.ErrNoRows){return AdminListRow{},ErrNotFound}
	if err!=nil{return AdminListRow{},err}
	if current==status { return AdminListRow{},fmt.Errorf("admin already has requested status") }
	if owner && status=="disabled" {
		var others int
		if err:=tx.QueryRow(ctx,`
			SELECT count(*)::int FROM admin_users u
			WHERE u.status='active' AND u.id<>$1
			  AND EXISTS(
			    SELECT 1 FROM admin_user_roles aur JOIN roles r ON r.id=aur.role_id
			    WHERE aur.admin_user_id=u.id AND r.code='owner'
			  )
		`,targetID).Scan(&others);err!=nil{return AdminListRow{},err}
		if others<1{return AdminListRow{},fmt.Errorf("cannot disable last active owner")}
	}
	if _,err:=tx.Exec(ctx,"UPDATE admin_users SET status=$2,updated_at=now() WHERE id=$1",targetID,status);err!=nil{return AdminListRow{},err}
	if status=="disabled" {
		if _,err:=tx.Exec(ctx,"UPDATE admin_sessions SET revoked_at=now() WHERE admin_user_id=$1 AND revoked_at IS NULL",targetID);err!=nil{return AdminListRow{},err}
	}
	if err:=tx.Commit(ctx);err!=nil{return AdminListRow{},err}
	return s.adminListByID(ctx,targetID)
}

func (s *Store) adminListByID(ctx context.Context,id string)(AdminListRow,error){
	var a AdminListRow
	err:=s.DB.QueryRow(ctx,`
		SELECT u.id::text,u.email,u.status,
		       COALESCE(array_agg(r.code ORDER BY r.code) FILTER(WHERE r.code IS NOT NULL),'{}'),
		       u.created_at,u.updated_at
		FROM admin_users u
		LEFT JOIN admin_user_roles aur ON aur.admin_user_id=u.id
		LEFT JOIN roles r ON r.id=aur.role_id
		WHERE u.id=$1 GROUP BY u.id
	`,id).Scan(&a.ID,&a.Email,&a.Status,&a.Roles,&a.CreatedAt,&a.UpdatedAt)
	if errors.Is(err,pgx.ErrNoRows){return AdminListRow{},ErrNotFound}
	return a,err
}

type roleQuerier interface {
	Query(context.Context,string,...any)(pgx.Rows,error)
}
func resolveRoleIDs(ctx context.Context,q roleQuerier,roles []string)([]string,error){
	seen:=map[string]bool{}
	clean:=make([]string,0,len(roles))
	for _,r:=range roles{
		r=strings.TrimSpace(strings.ToLower(r))
		if r!=""&&!seen[r]{seen[r]=true;clean=append(clean,r)}
	}
	if len(clean)==0{return nil,fmt.Errorf("no valid roles")}
	rows,err:=q.Query(ctx,"SELECT id::text,code FROM roles WHERE code=ANY($1)",clean)
	if err!=nil{return nil,err};defer rows.Close()
	ids:=make([]string,0,len(clean));found:=map[string]bool{}
	for rows.Next(){var id,code string;if err:=rows.Scan(&id,&code);err!=nil{return nil,err};ids=append(ids,id);found[code]=true}
	if err:=rows.Err();err!=nil{return nil,err}
	if len(found)!=len(clean){return nil,fmt.Errorf("unknown admin role")}
	return ids,nil
}
func containsRole(roles []string,target string)bool{
	for _,r:=range roles{if strings.EqualFold(strings.TrimSpace(r),target){return true}}
	return false
}
