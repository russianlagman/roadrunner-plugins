package http

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	endure "github.com/spiral/endure/pkg/container"
	"github.com/spiral/roadrunner-plugins/v2/config"
	httpPlugin "github.com/spiral/roadrunner-plugins/v2/http"
	"github.com/spiral/roadrunner-plugins/v2/logger"
	rpcPlugin "github.com/spiral/roadrunner-plugins/v2/rpc"
	"github.com/spiral/roadrunner-plugins/v2/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPPost(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "configs/.rr-post-test.yaml",
		Prefix: "rr",
	}

	err = cont.RegisterAll(
		cfg,
		&rpcPlugin.Plugin{},
		&logger.ZapLogger{},
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

	time.Sleep(time.Second * 1)
	t.Run("BombardWithPosts", echoHTTPPost)

	stopCh <- struct{}{}

	wg.Wait()
}

func echoHTTPPost(t *testing.T) {
	body := struct {
		Name  string `json:"name"`
		Index int    `json:"index"`
	}{
		Name:  "foo",
		Index: 111,
	}

	bd, err := json.Marshal(body)
	require.NoError(t, err)

	rdr := bytes.NewReader(bd)

	resp, err := http.Post("http://127.0.0.1:10084/", "", rdr)
	assert.NoError(t, err)

	b, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	require.True(t, bytes.Equal(bd, b))

	_ = resp.Body.Close()

	for i := 0; i < 20; i++ {
		rdr = bytes.NewReader(bd)
		resp, err = http.Post("http://127.0.0.1:10084/", "application/json", rdr)
		assert.NoError(t, err)

		b, err = ioutil.ReadAll(resp.Body)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		require.True(t, bytes.Equal(bd, b))

		_ = resp.Body.Close()
	}
}