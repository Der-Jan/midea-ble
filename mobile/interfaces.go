package mobile

// Notify receives raw FFA2 notification fragments from a platform transport.
// It is an interface (rather than a Go func) so gomobile can bind it to Java.
type Notify interface {
	OnNotify(data []byte)
}

// Transport is the gomobile-friendly equivalent of internal/ac.Transport.
// Implementations are supplied by the host platform (Android BluetoothGatt,
// for example).
type Transport interface {
	Write(data []byte) error
	SetNotify(notify Notify)
	Close() error
}

// Dialer creates a fresh Transport for the initial connection and reconnects.
// It intentionally has no context.Context so it can be implemented by Java.
type Dialer interface {
	Open() (Transport, error)
}

// StateListener receives de-duplicated state changes from the core modules.
type StateListener interface {
	OnPowerChanged(on bool)
	OnModeChanged(mode string)
	OnTemperatureChanged(celsius float64)
	OnFanChanged(fan string)
	OnSwingChanged(upDown bool, leftRight bool)
	OnEcoChanged(on bool)
	OnStrongChanged(on bool)
	OnAmbientChanged(indoor float64, outdoor float64)
}
