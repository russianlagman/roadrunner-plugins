package plugins

import (
	"context"
	"fmt"

	"github.com/spiral/errors"
	"github.com/spiral/roadrunner-plugins/v2/api/pubsub"
	"github.com/spiral/roadrunner-plugins/v2/broadcast"
	"github.com/spiral/roadrunner-plugins/v2/logger"
)

const Plugin3Name = "plugin3"

type Plugin3 struct {
	log    logger.Logger
	b      broadcast.Broadcaster
	driver pubsub.SubReader
	ctx    context.Context
	cancel context.CancelFunc
}

func (p *Plugin3) Init(log logger.Logger, b broadcast.Broadcaster) error {
	p.log = log
	p.b = b
	p.ctx, p.cancel = context.WithCancel(context.Background())
	return nil
}

func (p *Plugin3) Serve() chan error {
	errCh := make(chan error, 1)

	var err error
	p.driver, err = p.b.GetDriver("test2")
	if err != nil {
		panic(err)
	}

	err = p.driver.Subscribe("3", "foo")
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			msg, err := p.driver.Next(p.ctx)
			if err != nil {
				if errors.Is(errors.TimeOut, err) {
					return
				}
				errCh <- err
				return
			}

			if msg == nil {
				continue
			}

			p.log.Info(fmt.Sprintf("%s: %s", Plugin3Name, *msg))
		}
	}()

	return errCh
}

func (p *Plugin3) Stop() error {
	_ = p.driver.Unsubscribe("3", "foo")
	p.cancel()
	return nil
}

func (p *Plugin3) Name() string {
	return Plugin3Name
}
