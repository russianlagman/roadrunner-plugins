package natsjobs

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	json "github.com/json-iterator/go"
	"github.com/nats-io/nats.go"
	"github.com/spiral/errors"
	jobState "github.com/spiral/roadrunner-plugins/v2/api/jobs"
	cfgPlugin "github.com/spiral/roadrunner-plugins/v2/config"
	"github.com/spiral/roadrunner-plugins/v2/jobs/job"
	"github.com/spiral/roadrunner-plugins/v2/jobs/pipeline"
	"github.com/spiral/roadrunner-plugins/v2/logger"
	priorityqueue "github.com/spiral/roadrunner/v2/priority_queue"
)

const (
	pluginName      string = "nats"
	reconnectBuffer int    = 20 * 1024 * 1024
)

type consumer struct {
	// system
	sync.RWMutex
	log       logger.Logger
	queue     priorityqueue.Queue
	listeners uint32
	pipeline  atomic.Value
	stopCh    chan struct{}

	// nats
	conn  *nats.Conn
	sub   *nats.Subscription
	msgCh chan *nats.Msg
	js    nats.JetStreamContext

	// config
	subject            string
	stream             string
	prefetch           int
	rateLimit          uint64
	deleteAfterAck     bool
	deliverNew         bool
	deleteStreamOnStop bool
}

func FromConfig(configKey string, log logger.Logger, cfg cfgPlugin.Configurer, queue priorityqueue.Queue) (*consumer, error) {
	const op = errors.Op("new_nats_consumer")

	if !cfg.Has(configKey) {
		return nil, errors.E(op, errors.Errorf("no configuration by provided key: %s", configKey))
	}

	// if no global section
	if !cfg.Has(pluginName) {
		return nil, errors.E(op, errors.Str("no global nats configuration, global configuration should contain NATS URL"))
	}

	var conf *config
	err := cfg.UnmarshalKey(configKey, &conf)
	if err != nil {
		return nil, errors.E(op, err)
	}

	err = cfg.UnmarshalKey(pluginName, &conf)
	if err != nil {
		return nil, errors.E(op, err)
	}

	conf.InitDefaults()

	conn, err := nats.Connect(conf.Addr,
		nats.NoEcho(),
		nats.Timeout(time.Minute),
		nats.MaxReconnects(-1),
		nats.PingInterval(time.Second*10),
		nats.ReconnectWait(time.Second),
		nats.ReconnectBufSize(reconnectBuffer),
		nats.ReconnectHandler(reconnectHandler(log)),
		nats.DisconnectErrHandler(disconnectHandler(log)),
	)
	if err != nil {
		return nil, errors.E(op, err)
	}

	js, err := conn.JetStream()
	if err != nil {
		return nil, errors.E(op, err)
	}

	si, err := js.StreamInfo(conf.Stream)
	if err != nil {
		if err.Error() == "nats: stream not found" {
			// skip
		} else {
			return nil, errors.E(op, err)
		}
	}

	if si == nil {
		_, err = js.AddStream(&nats.StreamConfig{
			Name:     conf.Stream,
			Subjects: []string{conf.Subject},
		})
		if err != nil {
			return nil, errors.E(op, err)
		}
	}

	cs := &consumer{
		log:    log,
		stopCh: make(chan struct{}),
		queue:  queue,

		conn:               conn,
		js:                 js,
		subject:            conf.Subject,
		stream:             conf.Stream,
		deleteAfterAck:     conf.DeleteAfterAck,
		deleteStreamOnStop: conf.DeleteStreamOnStop,
		prefetch:           conf.Prefetch,
		deliverNew:         conf.DeliverNew,
		rateLimit:          conf.RateLimit,
		msgCh:              make(chan *nats.Msg, conf.Prefetch),
	}

	return cs, nil
}

func FromPipeline(pipe *pipeline.Pipeline, log logger.Logger, cfg cfgPlugin.Configurer, queue priorityqueue.Queue) (*consumer, error) {
	const op = errors.Op("new_nats_consumer")

	// if no global section -- error
	if !cfg.Has(pluginName) {
		return nil, errors.E(op, errors.Str("no global nats configuration, global configuration should contain NATS URL"))
	}

	var conf *config
	err := cfg.UnmarshalKey(pluginName, &conf)
	if err != nil {
		return nil, errors.E(op, err)
	}

	conf.InitDefaults()

	conn, err := nats.Connect(conf.Addr,
		nats.NoEcho(),
		nats.Timeout(time.Minute),
		nats.MaxReconnects(-1),
		nats.PingInterval(time.Second*10),
		nats.ReconnectWait(time.Second),
		nats.ReconnectBufSize(reconnectBuffer),
		nats.ReconnectHandler(reconnectHandler(log)),
		nats.DisconnectErrHandler(disconnectHandler(log)),
	)
	if err != nil {
		return nil, errors.E(op, err)
	}

	js, err := conn.JetStream()
	if err != nil {
		return nil, errors.E(op, err)
	}

	si, err := js.StreamInfo(pipe.String(pipeStream, "default-stream"))
	if err != nil {
		if err.Error() == "nats: stream not found" {
			// skip
		} else {
			return nil, errors.E(op, err)
		}
	}

	if si == nil {
		_, err = js.AddStream(&nats.StreamConfig{
			Name:     pipe.String(pipeStream, "default-stream"),
			Subjects: []string{pipe.String(pipeSubject, "default")},
		})
		if err != nil {
			return nil, errors.E(op, err)
		}
	}

	cs := &consumer{
		log:    log,
		queue:  queue,
		stopCh: make(chan struct{}),

		conn:               conn,
		js:                 js,
		subject:            pipe.String(pipeSubject, "default"),
		stream:             pipe.String(pipeStream, "default-stream"),
		prefetch:           pipe.Int(pipePrefetch, 100),
		deleteAfterAck:     pipe.Bool(pipeDeleteAfterAck, false),
		deliverNew:         pipe.Bool(pipeDeliverNew, false),
		deleteStreamOnStop: pipe.Bool(pipeDeleteStreamOnStop, false),
		rateLimit:          uint64(pipe.Int(pipeRateLimit, 1000)),
		msgCh:              make(chan *nats.Msg, pipe.Int(pipePrefetch, 100)),
	}

	return cs, nil
}

func (c *consumer) Push(_ context.Context, job *job.Job) error {
	const op = errors.Op("nats_consumer_push")
	if job.Options.Delay > 0 {
		return errors.E(op, errors.Str("nats doesn't support delayed messages, see: https://github.com/nats-io/nats-streaming-server/issues/324"))
	}

	data, err := json.Marshal(job)
	if err != nil {
		return errors.E(op, err)
	}

	_, err = c.js.Publish(c.subject, data)
	if err != nil {
		return errors.E(op, err)
	}

	job = nil
	return nil
}

func (c *consumer) Register(_ context.Context, pipeline *pipeline.Pipeline) error {
	c.pipeline.Store(pipeline)
	return nil
}

func (c *consumer) Run(_ context.Context, p *pipeline.Pipeline) error {
	start := time.Now()
	const op = errors.Op("nats_run")

	pipe := c.pipeline.Load().(*pipeline.Pipeline)
	if pipe.Name() != p.Name() {
		return errors.E(op, errors.Errorf("no such pipeline registered: %s", pipe.Name()))
	}

	err := c.listenerInit()
	if err != nil {
		return errors.E(op, err)
	}

	atomic.AddUint32(&c.listeners, 1)

	go c.listenerStart()

	c.log.Debug("pipeline started", "driver", pipe.Driver(), "pipeline", pipe.Name(), "start", start, "elapsed", time.Since(start))
	return nil
}

func (c *consumer) Pause(_ context.Context, p string) {
	start := time.Now()

	pipe := c.pipeline.Load().(*pipeline.Pipeline)
	if pipe.Name() != p {
		c.log.Error("no such pipeline", "requested pause on: ", p)
	}

	l := atomic.LoadUint32(&c.listeners)
	// no active listeners
	if l == 0 {
		c.log.Warn("no active listeners, nothing to pause")
		return
	}

	// remove listener
	atomic.AddUint32(&c.listeners, ^uint32(0))

	if c.sub != nil {
		err := c.sub.Drain()
		if err != nil {
			c.log.Error("drain error", "error", err)
		}
	}

	c.stopCh <- struct{}{}
	c.sub = nil

	c.log.Debug("pipeline paused", "driver", pipe.Driver(), "pipeline", pipe.Name(), "start", start, "elapsed", time.Since(start))
}

func (c *consumer) Resume(_ context.Context, p string) {
	start := time.Now()
	pipe := c.pipeline.Load().(*pipeline.Pipeline)
	if pipe.Name() != p {
		c.log.Error("no such pipeline", "requested resume on: ", p)
	}

	l := atomic.LoadUint32(&c.listeners)
	// no active listeners
	if l == 1 {
		c.log.Warn("nats listener already in the active state")
		return
	}

	err := c.listenerInit()
	if err != nil {
		c.log.Error("failed to resume NATS pipeline", "error", err, "pipeline", pipe.Name())
		return
	}

	go c.listenerStart()

	atomic.AddUint32(&c.listeners, 1)

	c.log.Debug("pipeline resumed", "driver", pipe.Driver(), "pipeline", pipe.Name(), "start", start, "elapsed", time.Since(start))
}

func (c *consumer) State(_ context.Context) (*jobState.State, error) {
	pipe := c.pipeline.Load().(*pipeline.Pipeline)

	st := &jobState.State{
		Pipeline: pipe.Name(),
		Driver:   pipe.Driver(),
		Queue:    c.subject,
		Ready:    ready(atomic.LoadUint32(&c.listeners)),
	}

	if c.sub != nil {
		ci, err := c.sub.ConsumerInfo()
		if err != nil {
			return nil, err
		}

		if ci != nil {
			st.Active = int64(ci.NumAckPending)
			st.Reserved = int64(ci.NumWaiting)
			st.Delayed = 0
		}
	}

	return st, nil
}

func (c *consumer) Stop(_ context.Context) error {
	start := time.Now()

	if atomic.LoadUint32(&c.listeners) > 0 {
		if c.sub != nil {
			err := c.sub.Drain()
			if err != nil {
				c.log.Error("drain error", "error", err)
			}
		}

		c.stopCh <- struct{}{}
	}

	if c.deleteStreamOnStop {
		err := c.js.DeleteStream(c.stream)
		if err != nil {
			return err
		}
	}

	pipe := c.pipeline.Load().(*pipeline.Pipeline)
	err := c.conn.Drain()
	if err != nil {
		return err
	}

	c.conn.Close()
	c.msgCh = nil
	c.log.Debug("pipeline stopped", "driver", pipe.Driver(), "pipeline", pipe.Name(), "start", start, "elapsed", time.Since(start))

	return nil
}

// private

func (c *consumer) requeue(item *Item) error {
	const op = errors.Op("nats_requeue")
	if item.Options.Delay > 0 {
		return errors.E(op, errors.Str("nats doesn't support delayed messages, see: https://github.com/nats-io/nats-streaming-server/issues/324"))
	}

	data, err := json.Marshal(item)
	if err != nil {
		return errors.E(op, err)
	}

	_, err = c.js.Publish(c.subject, data)
	if err != nil {
		return errors.E(op, err)
	}

	// delete the old message
	_ = c.js.DeleteMsg(c.stream, item.Options.seq)

	item = nil
	return nil
}

func (c *consumer) respond(data []byte, subject string) error {
	const op = errors.Op("nats_respond")
	err := c.conn.Publish(subject, data)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func reconnectHandler(log logger.Logger) func(*nats.Conn) {
	return func(conn *nats.Conn) {
		log.Warn("connection lost, reconnecting", "url", conn.ConnectedUrl())
	}
}

func disconnectHandler(log logger.Logger) func(*nats.Conn, error) {
	return func(_ *nats.Conn, err error) {
		if err != nil {
			log.Error("nast disconnected", "error", err)
			return
		}

		log.Warn("nast disconnected")
	}
}

func ready(r uint32) bool {
	return r > 0
}
