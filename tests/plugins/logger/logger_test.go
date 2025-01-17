package logger

import (
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	endure "github.com/spiral/endure/pkg/container"
	"github.com/spiral/roadrunner-plugins/v2/config"
	httpPlugin "github.com/spiral/roadrunner-plugins/v2/http"
	"github.com/spiral/roadrunner-plugins/v2/logger"
	"github.com/spiral/roadrunner-plugins/v2/rpc"
	"github.com/spiral/roadrunner-plugins/v2/server"
	"github.com/spiral/roadrunner-plugins/v2/tests/mocks"
	"github.com/stretchr/testify/assert"
)

func TestLogger(t *testing.T) {
	container, err := endure.NewContainer(nil, endure.RetryOnFail(false), endure.SetLogLevel(endure.ErrorLevel))
	if err != nil {
		t.Fatal(err)
	}
	// config plugin
	vp := &config.Viper{}
	vp.Path = "configs/.rr.yaml"
	vp.Prefix = "rr"

	err = container.RegisterAll(
		vp,
		&Plugin{},
		&logger.ZapLogger{},
	)
	assert.NoError(t, err)

	err = container.Init()
	if err != nil {
		t.Fatal(err)
	}

	errCh, err := container.Serve()
	if err != nil {
		t.Fatal(err)
	}

	// stop by CTRL+C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	stopCh := make(chan struct{}, 1)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-errCh:
				assert.NoError(t, e.Error)
				assert.NoError(t, container.Stop())
				return
			case <-c:
				err = container.Stop()
				assert.NoError(t, err)
				return
			case <-stopCh:
				assert.NoError(t, container.Stop())
				return
			}
		}
	}()

	stopCh <- struct{}{}
	wg.Wait()
}

func TestLoggerRawErr(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	// config plugin
	cfg := &config.Viper{}
	cfg.Path = "configs/.rr-raw-mode.yaml"
	cfg.Prefix = "rr"

	controller := gomock.NewController(t)
	mockLogger := mocks.NewMockLogger(controller)

	mockLogger.EXPECT().Info("event", "type", "EventWorkerConstruct", "message", gomock.Any(), "plugin", "pool").AnyTimes()
	mockLogger.EXPECT().Debug("http server is running", "address", gomock.Any()).AnyTimes()

	err = cont.RegisterAll(
		cfg,
		mockLogger,
		&server.Plugin{},
		&httpPlugin.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

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

func TestLoggerNoConfig(t *testing.T) {
	container, err := endure.NewContainer(nil, endure.RetryOnFail(true), endure.SetLogLevel(endure.ErrorLevel))
	if err != nil {
		t.Fatal(err)
	}
	// config plugin
	vp := &config.Viper{}
	vp.Path = "configs/.rr-no-logger.yaml"
	vp.Prefix = "rr"

	err = container.RegisterAll(
		vp,
		&Plugin{},
		&logger.ZapLogger{},
	)
	assert.NoError(t, err)

	err = container.Init()
	if err != nil {
		t.Fatal(err)
	}

	errCh, err := container.Serve()
	if err != nil {
		t.Fatal(err)
	}

	// stop by CTRL+C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	stopCh := make(chan struct{}, 1)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-errCh:
				assert.NoError(t, e.Error)
				assert.NoError(t, container.Stop())
				return
			case <-c:
				err = container.Stop()
				assert.NoError(t, err)
				return
			case <-stopCh:
				assert.NoError(t, container.Stop())
				return
			}
		}
	}()

	stopCh <- struct{}{}
	wg.Wait()
}

// Should no panic
func TestLoggerNoConfig2(t *testing.T) {
	container, err := endure.NewContainer(nil, endure.RetryOnFail(true), endure.SetLogLevel(endure.ErrorLevel))
	if err != nil {
		t.Fatal(err)
	}
	// config plugin
	vp := &config.Viper{}
	vp.Path = "configs/.rr-no-logger2.yaml"
	vp.Prefix = "rr"

	err = container.RegisterAll(
		vp,
		&rpc.Plugin{},
		&logger.ZapLogger{},
		&httpPlugin.Plugin{},
		&server.Plugin{},
	)
	assert.NoError(t, err)

	err = container.Init()
	if err != nil {
		t.Fatal(err)
	}

	errCh, err := container.Serve()
	if err != nil {
		t.Fatal(err)
	}

	// stop by CTRL+C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	stopCh := make(chan struct{}, 1)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-errCh:
				assert.NoError(t, e.Error)
				assert.NoError(t, container.Stop())
				return
			case <-c:
				err = container.Stop()
				assert.NoError(t, err)
				return
			case <-stopCh:
				assert.NoError(t, container.Stop())
				return
			}
		}
	}()

	stopCh <- struct{}{}
	wg.Wait()
}

func TestFileLogger(t *testing.T) {
	container, err := endure.NewContainer(nil, endure.RetryOnFail(true), endure.SetLogLevel(endure.ErrorLevel))
	if err != nil {
		t.Fatal(err)
	}
	// config plugin
	vp := &config.Viper{}
	vp.Path = "configs/.rr-file-logger.yaml"
	vp.Prefix = "rr"

	err = container.RegisterAll(
		vp,
		&rpc.Plugin{},
		&logger.ZapLogger{},
		&httpPlugin.Plugin{},
		&server.Plugin{},
	)
	assert.NoError(t, err)

	err = container.Init()
	if err != nil {
		t.Fatal(err)
	}

	errCh, err := container.Serve()
	if err != nil {
		t.Fatal(err)
	}

	// stop by CTRL+C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	stopCh := make(chan struct{}, 1)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-errCh:
				assert.NoError(t, e.Error)
				assert.NoError(t, container.Stop())
				return
			case <-c:
				err = container.Stop()
				assert.NoError(t, err)
				return
			case <-stopCh:
				assert.NoError(t, container.Stop())
				return
			}
		}
	}()

	time.Sleep(time.Second * 2)
	t.Run("HTTPEchoReq", httpEcho)

	f, err := os.ReadFile("test.log")
	if err != nil {
		t.Fatal(err)
	}

	strings.Contains(string(f), "worker constructed")
	strings.Contains(string(f), "201 GET")

	_ = os.Remove("test.log")

	stopCh <- struct{}{}
	wg.Wait()
}

func httpEcho(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:54224?hello=world", nil)
	assert.NoError(t, err)

	r, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, r.StatusCode)

	err = r.Body.Close()
	assert.NoError(t, err)
}

func TestMarshalObjectLogging(t *testing.T) {
	container, err := endure.NewContainer(nil, endure.RetryOnFail(true), endure.SetLogLevel(endure.ErrorLevel))
	if err != nil {
		t.Fatal(err)
	}
	// config plugin
	vp := &config.Viper{}
	vp.Path = "configs/.rr-file-logger.yaml"
	vp.Prefix = "rr"

	err = container.RegisterAll(
		vp,
		&Plugin{},
		&logger.ZapLogger{},
	)
	assert.NoError(t, err)

	err = container.Init()
	if err != nil {
		t.Fatal(err)
	}

	errCh, err := container.Serve()
	if err != nil {
		t.Fatal(err)
	}

	// stop by CTRL+C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	stopCh := make(chan struct{}, 1)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-errCh:
				assert.NoError(t, e.Error)
				assert.NoError(t, container.Stop())
				return
			case <-c:
				err = container.Stop()
				assert.NoError(t, err)
				return
			case <-stopCh:
				assert.NoError(t, container.Stop())
				return
			}
		}
	}()

	time.Sleep(time.Second * 2)

	f, err := os.ReadFile("test.log")
	if err != nil {
		t.Fatal(err)
	}

	assert.Contains(t, string(f), "Example field error")
	assert.Equal(t, 4, strings.Count(string(f), "Example field error"))

	stopCh <- struct{}{}
	wg.Wait()

	_ = os.Remove("test.log")
}
