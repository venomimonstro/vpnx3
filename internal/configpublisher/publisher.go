package configpublisher

import (
	"context"
	"log/slog"
	"time"

	"github.com/venomimonstro/vpnx3/internal/configservice"
)

type Publisher struct {
	service *configservice.Service
	logger *slog.Logger
	interval time.Duration
	trigger chan struct{}
}

func New(service *configservice.Service,logger *slog.Logger,interval time.Duration)*Publisher{
	if interval<time.Minute{interval=15*time.Minute}
	return &Publisher{
		service:service,
		logger:logger,
		interval:interval,
		trigger:make(chan struct{},1),
	}
}

func (p *Publisher) Trigger(){
	select{
	case p.trigger<-struct{}{}:
	default:
	}
}

func (p *Publisher) Run(ctx context.Context){
	// Publish once at startup so a fresh installation does not depend on a
	// manual admin action before clients can obtain a signed manifest.
	p.publish(ctx,"startup")

	t:=time.NewTicker(p.interval)
	defer t.Stop()

	for{
		select{
		case <-ctx.Done():
			return
		case <-t.C:
			p.publish(ctx,"periodic")
		case <-p.trigger:
			// Small debounce absorbs several node transitions that happen in one
			// operational action without delaying failover materially.
			timer:=time.NewTimer(2*time.Second)
			select{
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		drainLoop:
			for{
				select{
				case <-p.trigger:
					continue
				default:
					p.publish(ctx,"triggered")
					break drainLoop
				}
			}
		}
	}
}

func (p *Publisher) publish(parent context.Context,reason string){
	ctx,cancel:=context.WithTimeout(parent,10*time.Second)
	defer cancel()

	env,err:=p.service.Publish(ctx,"")
	if err!=nil{
		p.logger.Error("automatic configuration publish failed","reason",reason,"error",err)
		return
	}
	p.logger.Info(
		"automatic configuration published",
		"reason",reason,
		"key_id",env.KeyID,
	)
	if reason=="periodic" {
		if deleted,err:=p.service.CleanupHistory(ctx);err!=nil{
			p.logger.Warn("configuration history cleanup failed","error",err)
		}else if deleted>0{
			p.logger.Info("old configuration manifests cleaned","count",deleted)
		}
	}
}
