package ac_test

import (
	"context"
	"testing"
	"time"

	"github.com/sorinyang/midea-ble-go/internal/ac"
	"github.com/sorinyang/midea-ble-go/internal/ac/sessiontest"
)

func newDeviceConnected(t *testing.T) ac.IDevice {
	t.Helper()
	sess, err := ac.NewSession(sessiontest.Dial(), sessiontest.Adv, sessiontest.OpenID, sessiontest.Opts())
	if err != nil {
		t.Fatal(err)
	}
	dev := ac.NewDevice(sess)
	if err := dev.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	return dev
}

func TestTemperatureSetGet(t *testing.T) {
	dev := newDeviceConnected(t)
	defer dev.Close()
	v, err := dev.Temperature().Set(context.Background(), 26)
	if err != nil || v != 26 {
		t.Fatalf("set temp: v=%v err=%v", v, err)
	}
	g, err := dev.Temperature().Get(context.Background())
	if err != nil || g != 26 {
		t.Fatalf("get temp: g=%v err=%v", g, err)
	}
}

func TestModeFanSet(t *testing.T) {
	dev := newDeviceConnected(t)
	defer dev.Close()
	if v, err := dev.Mode().Set(context.Background(), "cool"); err != nil || v != "cool" {
		t.Fatalf("mode: v=%s err=%v", v, err)
	}
	if _, err := dev.Mode().Set(context.Background(), "bogus"); err == nil {
		t.Fatal("非法模式应报错")
	}
}

func TestPowerWatch(t *testing.T) {
	dev := newDeviceConnected(t)
	defer dev.Close()
	ch := dev.Power().Watch()
	if _, err := dev.Power().Set(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	select {
	case v := <-ch:
		if !v {
			t.Fatalf("watch 应收到开机=true，得 %v", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watch 超时")
	}
}

func TestSensorReadOnly(t *testing.T) {
	dev := newDeviceConnected(t)
	defer dev.Close()
	a, err := dev.Sensor().Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if a.In != 26 || a.Out != 30 {
		t.Fatalf("传感器期望 26/30，得 %v/%v", a.In, a.Out)
	}
	var _ ac.ISensor = dev.Sensor()
	_ = ac.ModeCool
}
