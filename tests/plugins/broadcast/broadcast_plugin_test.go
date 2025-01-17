package broadcast

import (
	"context"
	"net"
	"net/rpc"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	goRedis "github.com/go-redis/redis/v8"
	"github.com/golang/mock/gomock"
	endure "github.com/spiral/endure/pkg/container"
	goridgeRpc "github.com/spiral/goridge/v3/pkg/rpc"
	websocketsv1 "github.com/spiral/roadrunner-plugins/v2/api/proto/websockets/v1beta"
	"github.com/spiral/roadrunner-plugins/v2/broadcast"
	"github.com/spiral/roadrunner-plugins/v2/config"
	httpPlugin "github.com/spiral/roadrunner-plugins/v2/http"
	"github.com/spiral/roadrunner-plugins/v2/logger"
	"github.com/spiral/roadrunner-plugins/v2/memory"
	"github.com/spiral/roadrunner-plugins/v2/redis"
	rpcPlugin "github.com/spiral/roadrunner-plugins/v2/rpc"
	"github.com/spiral/roadrunner-plugins/v2/server"
	"github.com/spiral/roadrunner-plugins/v2/tests/mocks"
	"github.com/spiral/roadrunner-plugins/v2/tests/plugins/broadcast/plugins"
	"github.com/spiral/roadrunner-plugins/v2/websockets"
	"github.com/stretchr/testify/assert"
)

func TestBroadcastInit(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "configs/.rr-broadcast-init.yaml",
		Prefix: "rr",
	}

	err = cont.RegisterAll(
		cfg,
		&broadcast.Plugin{},
		&rpcPlugin.Plugin{},
		&logger.ZapLogger{},
		&server.Plugin{},
		&redis.Plugin{},
		&websockets.Plugin{},
		&httpPlugin.Plugin{},
		&memory.Plugin{},
	)

	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	if err != nil {
		t.Fatal(err)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	stopCh <- struct{}{}

	wg.Wait()
}

func TestBroadcastConfigError(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "configs/.rr-broadcast-config-error.yaml",
		Prefix: "rr",
	}

	err = cont.RegisterAll(
		cfg,
		&broadcast.Plugin{},
		&rpcPlugin.Plugin{},
		&logger.ZapLogger{},
		&server.Plugin{},
		&redis.Plugin{},
		&websockets.Plugin{},
		&httpPlugin.Plugin{},
		&memory.Plugin{},

		&plugins.Plugin1{},
	)

	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	_, err = cont.Serve()
	assert.Error(t, err)
	_ = cont.Stop()
}

func TestBroadcastNoConfig(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "configs/.rr-broadcast-no-config.yaml",
		Prefix: "rr",
	}

	controller := gomock.NewController(t)
	mockLogger := mocks.NewMockLogger(controller)

	mockLogger.EXPECT().Debug("worker destructed", "pid", gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug("worker constructed", "pid", gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug("RPC plugin started", "address", "tcp://127.0.0.1:6001", "plugins", []string{}).MinTimes(1)
	mockLogger.EXPECT().Debug("http server is running", "address", gomock.Any()).AnyTimes()

	err = cont.RegisterAll(
		cfg,
		&broadcast.Plugin{},
		&rpcPlugin.Plugin{},
		mockLogger,
		&server.Plugin{},
		&redis.Plugin{},
		&websockets.Plugin{},
		&httpPlugin.Plugin{},
		&memory.Plugin{},
	)

	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	// should be just disabled
	_, err = cont.Serve()
	assert.NoError(t, err)
	_ = cont.Stop()
}

func TestBroadcastSameSubscriber(t *testing.T) {
	t.Run("RedisFlush", redisFlushAll("127.0.0.1:6379"))
	t.Run("RedisFlush", redisFlushAll("127.0.0.1:6378"))

	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel), endure.GracefulShutdownTimeout(time.Second))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "configs/.rr-broadcast-same-section.yaml",
		Prefix: "rr",
	}

	controller := gomock.NewController(t)
	mockLogger := mocks.NewMockLogger(controller)

	mockLogger.EXPECT().Info("event", "type", "EventWorkerConstruct", "message", gomock.Any(), "plugin", "pool").AnyTimes()
	mockLogger.EXPECT().Debug("RPC plugin started", "address", "tcp://127.0.0.1:6002", "plugins", gomock.Any()).Times(1)

	mockLogger.EXPECT().Debug("message published", "msg", gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug("http server is running", "address", gomock.Any()).AnyTimes()

	mockLogger.EXPECT().Info(`plugin1: {foo hello}`).Times(3)
	mockLogger.EXPECT().Info(`plugin1: {foo2 hello}`).Times(2)
	mockLogger.EXPECT().Info(`plugin1: {foo3 hello}`).Times(3)
	mockLogger.EXPECT().Info(`plugin2: {foo hello}`).Times(3)
	mockLogger.EXPECT().Info(`plugin3: {foo hello}`).Times(3)
	mockLogger.EXPECT().Info(`plugin4: {foo hello}`).Times(3)
	mockLogger.EXPECT().Info(`plugin5: {foo hello}`).Times(3)
	mockLogger.EXPECT().Info(`plugin6: {foo hello}`).Times(3)

	err = cont.RegisterAll(
		cfg,
		&broadcast.Plugin{},
		&rpcPlugin.Plugin{},
		mockLogger,
		&server.Plugin{},
		&redis.Plugin{},
		&websockets.Plugin{},
		&httpPlugin.Plugin{},
		&memory.Plugin{},

		// test - redis
		// test2 - redis (port 6378)
		// test3 - memory
		// test4 - memory
		&plugins.Plugin1{}, // foo, foo2, foo3 test
		&plugins.Plugin2{}, // foo, test
		&plugins.Plugin3{}, // foo, test2
		&plugins.Plugin4{}, // foo, test3
		&plugins.Plugin5{}, // foo, test4
		&plugins.Plugin6{}, // foo, test3
	)

	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	if err != nil {
		t.Fatal(err)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second * 2)

	t.Run("PublishHelloFooFoo2Foo3", BroadcastPublishFooFoo2Foo3("6002"))
	time.Sleep(time.Second)
	t.Run("PublishHelloFoo2", BroadcastPublishFoo2("6002"))
	time.Sleep(time.Second)
	t.Run("PublishHelloFoo3", BroadcastPublishFoo3("6002"))
	time.Sleep(time.Second)
	t.Run("PublishAsyncHelloFooFoo2Foo3", BroadcastPublishAsyncFooFoo2Foo3("6002"))

	time.Sleep(time.Second * 5)

	stopCh <- struct{}{}

	wg.Wait()

	t.Run("RedisFlush", redisFlushAll("127.0.0.1:6379"))
	t.Run("RedisFlush", redisFlushAll("127.0.0.1:6378"))

	time.Sleep(time.Second * 5)
}

func TestBroadcastSameSubscriberGlobal(t *testing.T) {
	t.Run("RedisFlush", redisFlushAll("127.0.0.1:6379"))
	t.Run("RedisFlush", redisFlushAll("127.0.0.1:6378"))

	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel), endure.GracefulShutdownTimeout(time.Second))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "configs/.rr-broadcast-global.yaml",
		Prefix: "rr",
	}

	controller := gomock.NewController(t)
	mockLogger := mocks.NewMockLogger(controller)

	mockLogger.EXPECT().Info("event", "type", "EventWorkerConstruct", "message", gomock.Any(), "plugin", "pool").AnyTimes()
	mockLogger.EXPECT().Debug("RPC plugin started", "address", "tcp://127.0.0.1:6003", "plugins", gomock.Any()).Times(1)

	mockLogger.EXPECT().Debug("message published", "msg", gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug("http server is running", "address", gomock.Any()).AnyTimes()

	mockLogger.EXPECT().Info(`plugin1: {foo hello}`).Times(3)
	mockLogger.EXPECT().Info(`plugin1: {foo2 hello}`).Times(2)
	mockLogger.EXPECT().Info(`plugin1: {foo3 hello}`).Times(3)
	mockLogger.EXPECT().Info(`plugin2: {foo hello}`).Times(3)
	mockLogger.EXPECT().Info(`plugin3: {foo hello}`).Times(3)
	mockLogger.EXPECT().Info(`plugin4: {foo hello}`).Times(3)
	mockLogger.EXPECT().Info(`plugin5: {foo hello}`).Times(3)
	mockLogger.EXPECT().Info(`plugin6: {foo hello}`).Times(3)

	err = cont.RegisterAll(
		cfg,
		&broadcast.Plugin{},
		&rpcPlugin.Plugin{},
		mockLogger,
		&server.Plugin{},
		&redis.Plugin{},
		&websockets.Plugin{},
		&httpPlugin.Plugin{},
		&memory.Plugin{},

		// test - redis
		// test2 - redis (port 6378)
		// test3 - memory
		// test4 - memory
		&plugins.Plugin1{}, // foo, foo2, foo3 test
		&plugins.Plugin2{}, // foo, test
		&plugins.Plugin3{}, // foo, test2
		&plugins.Plugin4{}, // foo, test3
		&plugins.Plugin5{}, // foo, test4
		&plugins.Plugin6{}, // foo, test3
	)

	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	if err != nil {
		t.Fatal(err)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second * 2)

	t.Run("PublishHelloFooFoo2Foo3", BroadcastPublishFooFoo2Foo3("6003"))
	time.Sleep(time.Second)
	t.Run("PublishHelloFoo2", BroadcastPublishFoo2("6003"))
	time.Sleep(time.Second)
	t.Run("PublishHelloFoo3", BroadcastPublishFoo3("6003"))
	time.Sleep(time.Second)
	t.Run("PublishAsyncHelloFooFoo2Foo3", BroadcastPublishAsyncFooFoo2Foo3("6003"))

	time.Sleep(time.Second * 4)

	stopCh <- struct{}{}

	wg.Wait()

	time.Sleep(time.Second * 5)

	t.Run("RedisFlush", redisFlushAll("127.0.0.1:6379"))
	t.Run("RedisFlush", redisFlushAll("127.0.0.1:6378"))
}

func BroadcastPublishFooFoo2Foo3(port string) func(t *testing.T) {
	return func(t *testing.T) {
		conn, err := net.Dial("tcp", "127.0.0.1:"+port)
		if err != nil {
			t.Fatal(err)
		}

		client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))

		ret := &websocketsv1.Response{}
		err = client.Call("broadcast.Publish", makeMessage([]byte("hello"), "foo", "foo2", "foo3"), ret)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func BroadcastPublishFoo2(port string) func(t *testing.T) {
	return func(t *testing.T) {
		conn, err := net.Dial("tcp", "127.0.0.1:"+port)
		if err != nil {
			t.Fatal(err)
		}

		client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))

		ret := &websocketsv1.Response{}
		err = client.Call("broadcast.Publish", makeMessage([]byte("hello"), "foo"), ret)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func BroadcastPublishFoo3(port string) func(t *testing.T) {
	return func(t *testing.T) {
		conn, err := net.Dial("tcp", "127.0.0.1:"+port)
		if err != nil {
			t.Fatal(err)
		}

		client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))

		ret := &websocketsv1.Response{}
		err = client.Call("broadcast.Publish", makeMessage([]byte("hello"), "foo3"), ret)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func BroadcastPublishAsyncFooFoo2Foo3(port string) func(t *testing.T) {
	return func(t *testing.T) {
		conn, err := net.Dial("tcp", "127.0.0.1:"+port)
		if err != nil {
			t.Fatal(err)
		}

		client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))

		ret := &websocketsv1.Response{}
		err = client.Call("broadcast.PublishAsync", makeMessage([]byte("hello"), "foo", "foo2", "foo3"), ret)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func makeMessage(payload []byte, topics ...string) *websocketsv1.Request {
	m := &websocketsv1.Request{
		Messages: []*websocketsv1.Message{
			{
				Topics:  topics,
				Payload: payload,
			},
		},
	}

	return m
}

func redisFlushAll(addr string) func(t *testing.T) {
	return func(t *testing.T) {
		rdb := goRedis.NewClient(&goRedis.Options{
			Addr:     addr,
			Password: "", // no password set
			DB:       0,  // use default DB
		})

		rdb.FlushAll(context.Background())
		_ = rdb.Close()
	}
}
