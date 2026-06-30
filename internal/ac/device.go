package ac

import (
	"context"
)

// bleDevice 在一个 Session 上聚合全部功能模块，实现 IDevice。
type bleDevice struct {
	s      *Session
	power  *powerModule
	mode   *modeModule
	temp   *temperatureModule
	fan    *fanModule
	swing  *swingModule
	eco    *ecoModule
	strong *strongModule
	sensor *sensorModule
}

var _ IDevice = (*bleDevice)(nil)

// NewDevice 在给定 Session 上构造一台空调设备的能力聚合。
func NewDevice(s *Session) IDevice {
	return &bleDevice{
		s:      s,
		power:  newPowerModule(s),
		mode:   newModeModule(s),
		temp:   newTemperatureModule(s),
		fan:    newFanModule(s),
		swing:  newSwingModule(s),
		eco:    newEcoModule(s),
		strong: newStrongModule(s),
		sensor: newSensorModule(s),
	}
}

func (d *bleDevice) Connect(ctx context.Context) error { return d.s.Handshake(ctx) }
func (d *bleDevice) Close() error                      { return d.s.Close() }

func (d *bleDevice) Power() IPower             { return d.power }
func (d *bleDevice) Mode() IMode               { return d.mode }
func (d *bleDevice) Temperature() ITemperature { return d.temp }
func (d *bleDevice) Fan() IFan                 { return d.fan }
func (d *bleDevice) Swing() ISwing             { return d.swing }
func (d *bleDevice) Eco() IEco                 { return d.eco }
func (d *bleDevice) Strong() IStrong           { return d.strong }
func (d *bleDevice) Sensor() ISensor           { return d.sensor }

func (d *bleDevice) Snapshot(ctx context.Context) (*Status, error) { return d.s.Query(ctx) }
func (d *bleDevice) SetBeep(on bool)                               { d.s.SetBeep(on) }
