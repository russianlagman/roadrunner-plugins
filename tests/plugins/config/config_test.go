package config

import (
	"os"
	"os/signal"
	"testing"
	"time"

	endure "github.com/spiral/endure/pkg/container"
	"github.com/spiral/roadrunner-plugins/v2/config"
	"github.com/spiral/roadrunner-plugins/v2/logger"
	"github.com/spiral/roadrunner-plugins/v2/rpc"
	"github.com/stretchr/testify/assert"
)

func TestViperProvider_Init(t *testing.T) {
	container, err := endure.NewContainer(nil, endure.RetryOnFail(true), endure.SetLogLevel(endure.ErrorLevel))
	if err != nil {
		t.Fatal(err)
	}
	vp := &config.Viper{}
	vp.Path = "configs/.rr.yaml"
	vp.Prefix = "rr"
	vp.Flags = nil

	err = container.Register(vp)
	if err != nil {
		t.Fatal(err)
	}

	err = container.Register(&Foo{})
	if err != nil {
		t.Fatal(err)
	}

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

	tt := time.NewTicker(time.Second * 2)
	defer tt.Stop()

	for {
		select {
		case e := <-errCh:
			assert.NoError(t, e.Error)
			assert.NoError(t, container.Stop())
			return
		case <-c:
			er := container.Stop()
			assert.NoError(t, er)
			return
		case <-tt.C:
			assert.NoError(t, container.Stop())
			return
		}
	}
}

func TestConfigOverwriteFail(t *testing.T) {
	container, err := endure.NewContainer(nil, endure.RetryOnFail(false), endure.SetLogLevel(endure.ErrorLevel))
	if err != nil {
		t.Fatal(err)
	}
	vp := &config.Viper{}
	vp.Path = "configs/.rr.yaml"
	vp.Prefix = "rr"
	vp.Flags = []string{"rpc.listen=tcp//not_exist"}

	err = container.RegisterAll(
		&logger.ZapLogger{},
		&rpc.Plugin{},
		vp,
		&Foo2{},
	)
	assert.NoError(t, err)

	err = container.Init()
	assert.Error(t, err)
}

func TestConfigOverwriteFail_2(t *testing.T) {
	container, err := endure.NewContainer(nil, endure.RetryOnFail(false), endure.SetLogLevel(endure.ErrorLevel))
	if err != nil {
		t.Fatal(err)
	}
	vp := &config.Viper{}
	vp.Path = "configs/.rr.yaml"
	vp.Prefix = "rr"
	vp.Flags = []string{"rpc.listen="}

	err = container.RegisterAll(
		&logger.ZapLogger{},
		&rpc.Plugin{},
		vp,
		&Foo2{},
	)
	assert.NoError(t, err)

	err = container.Init()
	assert.Error(t, err)
}

func TestConfigOverwriteFail_3(t *testing.T) {
	container, err := endure.NewContainer(nil, endure.RetryOnFail(false), endure.SetLogLevel(endure.ErrorLevel))
	if err != nil {
		t.Fatal(err)
	}
	vp := &config.Viper{}
	vp.Path = "configs/.rr.yaml"
	vp.Prefix = "rr"
	vp.Flags = []string{"="}

	err = container.RegisterAll(
		&logger.ZapLogger{},
		&rpc.Plugin{},
		vp,
		&Foo2{},
	)
	assert.NoError(t, err)

	err = container.Init()
	assert.Error(t, err)
}

func TestConfigOverwriteValid(t *testing.T) {
	container, err := endure.NewContainer(nil, endure.RetryOnFail(false), endure.SetLogLevel(endure.ErrorLevel))
	if err != nil {
		t.Fatal(err)
	}
	vp := &config.Viper{}
	vp.Path = "configs/.rr.yaml"
	vp.Prefix = "rr"
	vp.Flags = []string{"rpc.listen=tcp://127.0.0.1:36643"}

	err = container.RegisterAll(
		&logger.ZapLogger{},
		&rpc.Plugin{},
		vp,
		&Foo2{},
	)
	assert.NoError(t, err)

	err = container.Init()
	assert.NoError(t, err)

	errCh, err := container.Serve()
	assert.NoError(t, err)

	// stop by CTRL+C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	tt := time.NewTicker(time.Second * 3)
	defer tt.Stop()

	for {
		select {
		case e := <-errCh:
			assert.NoError(t, e.Error)
			assert.NoError(t, container.Stop())
			return
		case <-c:
			er := container.Stop()
			assert.NoError(t, er)
			return
		case <-tt.C:
			assert.NoError(t, container.Stop())
			return
		}
	}
}

func TestConfigEnvVariables(t *testing.T) {
	container, err := endure.NewContainer(nil, endure.RetryOnFail(false), endure.SetLogLevel(endure.ErrorLevel))
	if err != nil {
		t.Fatal(err)
	}

	err = os.Setenv("SUPER_RPC_ENV", "tcp://127.0.0.1:36643")
	assert.NoError(t, err)

	vp := &config.Viper{}
	vp.Path = "configs/.rr-env.yaml"
	vp.Prefix = "rr"

	err = container.RegisterAll(
		&logger.ZapLogger{},
		&rpc.Plugin{},
		vp,
		&Foo2{},
	)
	assert.NoError(t, err)

	err = container.Init()
	assert.NoError(t, err)

	errCh, err := container.Serve()
	assert.NoError(t, err)

	// stop by CTRL+C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	tt := time.NewTicker(time.Second * 3)
	defer tt.Stop()

	for {
		select {
		case e := <-errCh:
			assert.NoError(t, e.Error)
			assert.NoError(t, container.Stop())
			return
		case <-c:
			er := container.Stop()
			assert.NoError(t, er)
			return
		case <-tt.C:
			assert.NoError(t, container.Stop())
			return
		}
	}
}

func TestConfigEnvVariablesFail(t *testing.T) {
	container, err := endure.NewContainer(nil, endure.RetryOnFail(false), endure.SetLogLevel(endure.ErrorLevel))
	if err != nil {
		t.Fatal(err)
	}

	err = os.Setenv("SUPER_RPC_ENV", "tcp://127.0.0.1:6065")
	assert.NoError(t, err)

	vp := &config.Viper{}
	vp.Path = "configs/.rr-env.yaml"
	vp.Prefix = "rr"

	err = container.RegisterAll(
		&logger.ZapLogger{},
		&rpc.Plugin{},
		vp,
		&Foo2{},
	)
	assert.NoError(t, err)

	err = container.Init()
	assert.NoError(t, err)

	_, err = container.Serve()
	assert.Error(t, err)
}

func TestConfigProvider_GeneralSection(t *testing.T) {
	container, err := endure.NewContainer(nil, endure.RetryOnFail(true), endure.SetLogLevel(endure.ErrorLevel))
	if err != nil {
		t.Fatal(err)
	}
	vp := &config.Viper{}
	vp.Path = "configs/.rr.yaml"
	vp.Prefix = "rr"
	vp.Flags = nil
	vp.CommonConfig = &config.General{GracefulTimeout: time.Second * 10}

	err = container.Register(vp)
	if err != nil {
		t.Fatal(err)
	}

	err = container.Register(&Foo3{})
	if err != nil {
		t.Fatal(err)
	}

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

	tt := time.NewTicker(time.Second * 2)
	defer tt.Stop()

	for {
		select {
		case e := <-errCh:
			assert.NoError(t, e.Error)
			assert.NoError(t, container.Stop())
			return
		case <-c:
			er := container.Stop()
			assert.NoError(t, er)
			return
		case <-tt.C:
			assert.NoError(t, container.Stop())
			return
		}
	}
}
