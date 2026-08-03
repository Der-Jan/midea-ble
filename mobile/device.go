package mobile

import (
	"context"
	"crypto/rand"
	"errors"
	"sync"

	"github.com/sorinyang/midea-ble-go/internal/ac"
)

// Device is the small, Java-friendly facade over internal/ac.IDevice.
type Device struct {
	inner ac.IDevice

	mu     sync.Mutex
	cancel context.CancelFunc
}

// NewDevice creates a device backed by a platform-provided Dialer.
func NewDevice(dialer Dialer, advertisData []byte, openID6 []byte) (*Device, error) {
	if dialer == nil {
		return nil, errors.New("dialer is nil")
	}
	acDialer := func(context.Context) (ac.Transport, error) {
		transport, err := dialer.Open()
		if err != nil {
			return nil, err
		}
		if transport == nil {
			return nil, errors.New("dialer returned nil transport")
		}
		return &transportAdapter{transport: transport}, nil
	}
	if len(openID6) != 6 {
		openID6 = make([]byte, 6)
		_, _ = rand.Read(openID6)
	}
	inner, err := ac.NewSession(acDialer, advertisData, openID6, ac.DefaultOptions())
	if err != nil {
		return nil, err
	}
	return &Device{inner: ac.NewDevice(inner)}, nil
}

func (d *Device) Connect() error {
	return d.inner.Connect(context.Background())
}

func (d *Device) Close() error {
	d.stopWatching()
	return d.inner.Close()
}

func (d *Device) GetPower() (bool, error) {
	return d.inner.Power().Get(context.Background())
}

func (d *Device) SetPower(on bool) (bool, error) {
	return d.inner.Power().Set(context.Background(), on)
}

func (d *Device) GetMode() (string, error) {
	return d.inner.Mode().Get(context.Background())
}

func (d *Device) SetMode(mode string) (string, error) {
	return d.inner.Mode().Set(context.Background(), mode)
}

func (d *Device) GetTemperature() (float64, error) {
	return d.inner.Temperature().Get(context.Background())
}

func (d *Device) SetTemperature(celsius float64) (float64, error) {
	return d.inner.Temperature().Set(context.Background(), celsius)
}

func (d *Device) GetFan() (string, error) {
	return d.inner.Fan().Get(context.Background())
}

func (d *Device) SetFan(fan string) (string, error) {
	return d.inner.Fan().Set(context.Background(), fan)
}

func (d *Device) GetSwing() (*Swing, error) {
	value, err := d.inner.Swing().Get(context.Background())
	if err != nil {
		return nil, err
	}
	return &Swing{UpDown: value.UD, LeftRight: value.LR}, nil
}

func (d *Device) SetSwing(upDown bool, leftRight bool) (*Swing, error) {
	value, err := d.inner.Swing().Set(context.Background(), upDown, leftRight)
	if err != nil {
		return nil, err
	}
	return &Swing{UpDown: value.UD, LeftRight: value.LR}, nil
}

func (d *Device) GetEco() (bool, error) {
	return d.inner.Eco().Get(context.Background())
}

func (d *Device) SetEco(on bool) (bool, error) {
	return d.inner.Eco().Set(context.Background(), on)
}

func (d *Device) GetStrong() (bool, error) {
	return d.inner.Strong().Get(context.Background())
}

func (d *Device) SetStrong(on bool) (bool, error) {
	return d.inner.Strong().Set(context.Background(), on)
}

func (d *Device) GetSensor() (*Ambient, error) {
	value, err := d.inner.Sensor().Get(context.Background())
	if err != nil {
		return nil, err
	}
	return &Ambient{Indoor: value.In, Outdoor: value.Out}, nil
}

func (d *Device) SetBeep(on bool) {
	d.inner.SetBeep(on)
}

// SetStateListener starts callback watchers. Calling it again replaces the previous listener.
func (d *Device) SetStateListener(listener StateListener) {
	d.stopWatching()
	if listener == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.mu.Lock()
	d.cancel = cancel
	d.mu.Unlock()

	go watchBool(ctx, d.inner.Power().Watch(), listener.OnPowerChanged)
	go watchString(ctx, d.inner.Mode().Watch(), listener.OnModeChanged)
	go watchFloat(ctx, d.inner.Temperature().Watch(), listener.OnTemperatureChanged)
	go watchString(ctx, d.inner.Fan().Watch(), listener.OnFanChanged)
	go watchSwing(ctx, d.inner.Swing().Watch(), listener.OnSwingChanged)
	go watchBool(ctx, d.inner.Eco().Watch(), listener.OnEcoChanged)
	go watchBool(ctx, d.inner.Strong().Watch(), listener.OnStrongChanged)
	go watchAmbient(ctx, d.inner.Sensor().Watch(), listener.OnAmbientChanged)
}

func (d *Device) stopWatching() {
	d.mu.Lock()
	cancel := d.cancel
	d.cancel = nil
	d.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

type Swing struct {
	UpDown    bool
	LeftRight bool
}

type Ambient struct {
	Indoor  float64
	Outdoor float64
}

type transportAdapter struct{ transport Transport }

var _ ac.Transport = (*transportAdapter)(nil)

func (a *transportAdapter) Write(data []byte) error { return a.transport.Write(data) }
func (a *transportAdapter) Close() error            { return a.transport.Close() }
func (a *transportAdapter) SetNotify(callback func([]byte)) {
	// notifyFunc contains a Go function and is therefore not comparable as a
	// value. gomobile stores interface values in a reference map, so pass a
	// pointer to keep the dynamic interface value comparable.
	a.transport.SetNotify(&notifyFunc{callback: callback})
}

type notifyFunc struct{ callback func([]byte) }

var _ Notify = (*notifyFunc)(nil)

func (n notifyFunc) OnNotify(data []byte) {
	if n.callback != nil {
		n.callback(data)
	}
}

func watchBool(ctx context.Context, ch <-chan bool, callback func(bool)) {
	for {
		select {
		case value, ok := <-ch:
			if !ok {
				return
			}
			callback(value)
		case <-ctx.Done():
			return
		}
	}
}

func watchString(ctx context.Context, ch <-chan string, callback func(string)) {
	for {
		select {
		case value, ok := <-ch:
			if !ok {
				return
			}
			callback(value)
		case <-ctx.Done():
			return
		}
	}
}

func watchFloat(ctx context.Context, ch <-chan float64, callback func(float64)) {
	for {
		select {
		case value, ok := <-ch:
			if !ok {
				return
			}
			callback(value)
		case <-ctx.Done():
			return
		}
	}
}

func watchSwing(ctx context.Context, ch <-chan ac.SwingState, callback func(bool, bool)) {
	for {
		select {
		case value, ok := <-ch:
			if !ok {
				return
			}
			callback(value.UD, value.LR)
		case <-ctx.Done():
			return
		}
	}
}

func watchAmbient(ctx context.Context, ch <-chan ac.Ambient, callback func(float64, float64)) {
	for {
		select {
		case value, ok := <-ch:
			if !ok {
				return
			}
			callback(value.In, value.Out)
		case <-ctx.Done():
			return
		}
	}
}
