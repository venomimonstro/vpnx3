package trustbundle

import (
	"context"
	"fmt"
	"time"
)

func Bootstrap(
	ctx context.Context,
	sources []string,
	rootPublicKey string,
	statePath string,
	now time.Time,
)(*Client,Payload,error){
	client,err:=NewClient(sources,rootPublicKey,statePath)
	if err!=nil{return nil,Payload{},err}

	var stored Payload
	haveStored:=false
	if p,ok,loadErr:=client.LoadStored(now);loadErr==nil&&ok{
		stored=p
		haveStored=true
	}

	fresh,fetchErr:=client.Fetch(ctx,now)
	if fetchErr==nil{return client,fresh,nil}
	if haveStored{return client,stored,nil}
	return nil,Payload{},fmt.Errorf("no valid trust bundle available: %w",fetchErr)
}

func Poll(
	ctx context.Context,
	client *Client,
	interval time.Duration,
	onUpdate func(Payload) error,
	onError func(error),
){
	if client==nil||onUpdate==nil{return}
	if interval<time.Minute{interval=time.Minute}
	ticker:=time.NewTicker(interval)
	defer ticker.Stop()
	for{
		select{
		case <-ctx.Done():
			return
		case now:=<-ticker.C:
			fetchCtx,cancel:=context.WithTimeout(ctx,10*time.Second)
			payload,err:=client.Fetch(fetchCtx,now.UTC())
			cancel()
			if err!=nil{
				if onError!=nil{onError(err)}
				continue
			}
			if err:=onUpdate(payload);err!=nil&&onError!=nil{onError(err)}
		}
	}
}
