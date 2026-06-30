package ac

import (
	"fmt"
	"math"
)

// 业务层：biz type 恒为 32；body 是美的 appliance 帧（0xAA 开头）。
const (
	BizTypeAC = 32
	devTypeAC = 0xAC
)

// 模式
const (
	ModeAuto     = 1
	ModeCool     = 2
	ModeDry      = 3
	ModeHeat     = 4
	ModeFan      = 5
	ModeSmartDry = 6
)

// 风速
const (
	WindLow   = 1
	WindMid   = 2
	WindHigh  = 3
	WindFull  = 4
	WindMute  = 5
	WindAuto  = 6
	WindFixed = 7
)

var ModeNames = map[int]string{1: "auto", 2: "cool", 3: "dry", 4: "heat", 5: "fan", 6: "smart_dry"}
var WindNames = map[int]string{1: "low", 2: "mid", 3: "high", 4: "full", 5: "mute", 6: "auto", 7: "fixed"}
var ModeByName = map[string]int{"auto": 1, "cool": 2, "dry": 3, "heat": 4, "fan": 5, "smart_dry": 6}
var WindByName = map[string]int{"low": 1, "mid": 2, "high": 3, "full": 4, "mute": 5, "auto": 6, "fixed": 7}

// crc8_854 查表（从 sub_out/modules/AC 抽出）。
var crc8854Table = [256]byte{
	0, 94, 188, 226, 97, 63, 221, 131, 194, 156, 126, 32, 163, 253, 31, 65,
	157, 195, 33, 127, 252, 162, 64, 30, 95, 1, 227, 189, 62, 96, 130, 220,
	35, 125, 159, 193, 66, 28, 254, 160, 225, 191, 93, 3, 128, 222, 60, 98,
	190, 224, 2, 92, 223, 129, 99, 61, 124, 34, 192, 158, 29, 67, 161, 255,
	70, 24, 250, 164, 39, 121, 155, 197, 132, 218, 56, 102, 229, 187, 89, 7,
	219, 133, 103, 57, 186, 228, 6, 88, 25, 71, 165, 251, 120, 38, 196, 154,
	101, 59, 217, 135, 4, 90, 184, 230, 167, 249, 27, 69, 198, 152, 122, 36,
	248, 166, 68, 26, 153, 199, 37, 123, 58, 100, 134, 216, 91, 5, 231, 185,
	140, 210, 48, 110, 237, 179, 81, 15, 78, 16, 242, 172, 47, 113, 147, 205,
	17, 79, 173, 243, 112, 46, 204, 146, 211, 141, 111, 49, 178, 236, 14, 80,
	175, 241, 19, 77, 206, 144, 114, 44, 109, 51, 209, 143, 12, 82, 176, 238,
	50, 108, 142, 208, 83, 13, 239, 177, 240, 174, 76, 18, 145, 207, 45, 115,
	202, 148, 118, 40, 171, 245, 23, 73, 8, 86, 180, 234, 105, 55, 213, 139,
	87, 9, 235, 181, 54, 104, 138, 212, 149, 203, 41, 119, 244, 170, 72, 22,
	233, 183, 85, 11, 136, 214, 52, 106, 43, 117, 151, 201, 74, 20, 246, 168,
	116, 42, 200, 150, 21, 75, 169, 247, 182, 232, 10, 84, 215, 137, 107, 53,
}

// makeSum(arr, n) = (255 - Σarr[0:n] + 1) & 0xFF
func makeSum(arr []byte, n int) byte {
	var s int
	for i := 0; i < n; i++ {
		s += int(arr[i])
	}
	return byte((255 - s + 1) & 0xFF)
}

// crc8_854(arr, n)：对数据段查表 CRC8。
func crc8854(arr []byte, n int) byte {
	var o byte
	for i := 0; i < n; i++ {
		o = crc8854Table[o^arr[i]]
	}
	return o
}

// ACState 控制帧编码所需的状态（默认值=全关）。
type ACState struct {
	RunStatus      int
	ControlSource  int
	IMode          int
	ChildSleepMode int
	TimingType     int
	QuickChkSts    int
	BtnSound       int

	Mode           int
	TempSet        float64
	TempModeSwitch int

	WindSpeed     int
	TimingIsValid int

	TimingOnSwitch  int
	TimingOffSwitch int
	TimingOnHour    int
	TimingOnMinute  int
	TimingOffHour   int
	TimingOffMinute int

	CosyWind           int
	LeftUpDownWind     int
	RightUpDownWind    int
	LeftLeftRightWind  int
	RightLeftRightWind int

	CosySleepMode int
	AlmSleep      int
	PowerSave     int
	FarceWind     int
	Strong        int
	EnergySave    int
	BodySense     int

	WisdomEye       int
	ChgOfAir        int
	DiyFunc         int
	ElecHeat        int
	ElecHeatForced  int
	CleanUpFunc     int
	ChgComfortSleep int
	EcoFunc         int

	SleepFuncState  int
	TubroFuncState  int
	AgainstCool     int
	NightLight      int
	Pmv             int
	DustFlow        int
	CleanFanRunTime int

	SleepTemps       [10]float64
	ComfortSleepTime int
	NaturalWind      int

	TempSet2         float64
	Humidity         int
	DownWind         int
	TempRangeUpLimit float64
	CSEco            int
	Order            int
}

// NewACState 返回带合法默认值的状态（睡眠温度默认 26℃，btnSound 开，制冷 26℃，自动风）。
func NewACState() *ACState {
	s := &ACState{
		ControlSource:    1,
		BtnSound:         1,
		Mode:             ModeCool,
		TempSet:          26,
		WindSpeed:        WindAuto,
		ComfortSleepTime: 10,
		TempSet2:         26,
		TempRangeUpLimit: 26,
		Order:            1,
	}
	for i := range s.SleepTemps {
		s.SleepTemps[i] = 26
	}
	return s
}

func checkTempVal(t float64) int {
	v := int(math.Round(t))
	if v < 17 {
		v = 17
	}
	if v > 30 {
		v = 30
	}
	return v
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// BuildControlFrame 构造控制 appliance 帧（opcode 0x40，37 字节）。
func BuildControlFrame(s *ACState) ([]byte, error) {
	a := make([]byte, 37)
	a[0] = 0xAA
	a[2] = devTypeAC
	a[8] = 2
	a[9] = 2
	a[10] = 64
	s.ControlSource = 1

	a[11] = byte((s.BtnSound&1)<<6 | (s.QuickChkSts&1)<<5 | (s.TimingType&1)<<4 |
		(s.ChildSleepMode&1)<<3 | (s.IMode&1)<<2 | (s.ControlSource&1)<<1 | (s.RunStatus & 1))

	a[12] = byte((s.Mode & 7) << 5)
	u := int(math.Round(10 * s.TempSet))
	if !(u < 160 || u > 300) {
		a[12] |= byte((u/10 - 16) & 15)
	}
	switch u % 10 {
	case 0:
	case 5:
		a[12] |= 16
	default:
		return nil, fmt.Errorf("温度只能是 0.5 的整数倍: %v", s.TempSet)
	}

	a[13] = byte((s.TimingIsValid&1)<<7 | (s.WindSpeed & 127))

	if s.TimingType == 1 {
		a[14] = byte(s.TimingOnSwitch<<7 | s.TimingOnHour<<2 | s.TimingOnMinute/15)
		a[15] = byte(s.TimingOffSwitch<<7 | s.TimingOffHour<<2 | (3 & s.TimingOffMinute))
		a[16] = byte((s.TimingOnMinute%15)<<4 | (s.TimingOffMinute % 15))
	} else {
		S := s.TimingOnMinute
		if s.TimingOnSwitch == 1 && S != 0 {
			a[14] = 128
			if S%15 != 0 {
				a[14] |= byte((S / 15) & 127)
				a[16] |= byte(((15 - S%15) & 15) << 4)
			} else {
				a[14] |= byte((S / 15) & 127)
				a[16] = 255
			}
		} else {
			a[14] = 127
			a[16] = 255
		}
		S = s.TimingOffMinute
		if s.TimingOffSwitch == 1 && S != 0 {
			a[15] = 128
			if S%15 != 0 {
				a[15] |= byte((S / 15) & 127)
				a[16] |= byte((15 - S%15) & 15)
			} else {
				a[15] |= byte((S / 15) & 127)
				a[16] = 255
			}
		} else {
			a[15] = 127
			a[16] = 255
		}
	}

	if s.CosyWind == 0 {
		a[17] = 48
		a[17] |= byte((s.RightLeftRightWind & 1) << 0)
		a[17] |= byte((s.LeftLeftRightWind & 1) << 1)
		a[17] |= byte((s.RightUpDownWind & 1) << 2)
		a[17] |= byte((s.LeftUpDownWind & 1) << 3)
	} else if s.CosyWind < 10 {
		a[17] = byte(s.CosyWind + 16)
	} else {
		a[17] = byte(s.CosyWind + 22)
	}

	a[18] = byte(3&s.CosySleepMode | s.AlmSleep<<2 | s.PowerSave<<3 |
		s.FarceWind<<4 | s.Strong<<5 | s.EnergySave<<6 | s.BodySense<<7)

	if s.CosySleepMode > 0 {
		s.ChgComfortSleep = 1
	} else {
		s.ChgComfortSleep = 0
	}
	a[19] = byte(s.WisdomEye<<0 | s.ChgOfAir<<1 | s.DiyFunc<<2 | s.ElecHeat<<3 |
		s.ElecHeatForced<<4 | s.CleanUpFunc<<5 | s.ChgComfortSleep<<6 | s.EcoFunc<<7)

	a[20] = byte(s.SleepFuncState<<0 | s.TubroFuncState<<1 | s.TempModeSwitch<<2 |
		s.AgainstCool<<3 | s.NightLight<<4 | s.Pmv<<5 | s.DustFlow<<6 | s.CleanFanRunTime<<7)

	a[21] = byte((checkTempVal(s.SleepTemps[0]) - 17) | (checkTempVal(s.SleepTemps[1])-17)<<4)
	a[22] = byte((checkTempVal(s.SleepTemps[2]) - 17) | (checkTempVal(s.SleepTemps[3])-17)<<4)
	a[23] = byte((checkTempVal(s.SleepTemps[4]) - 17) | (checkTempVal(s.SleepTemps[5])-17)<<4)
	a[24] = byte((checkTempVal(s.SleepTemps[6]) - 17) | (checkTempVal(s.SleepTemps[7])-17)<<4)
	a[25] = byte((checkTempVal(s.SleepTemps[8]) - 17) | (checkTempVal(s.SleepTemps[9])-17)<<4)

	half := func(t float64) bool { return int(math.Round(10*t))%10 == 5 }
	a[26] = 0
	bits26 := []int{1, 2, 4, 8, 16, 32, 64, 128}
	for i := 0; i < 8; i++ {
		if half(s.SleepTemps[i]) {
			a[26] |= byte(bits26[i])
		}
	}
	a[27] = 0
	if half(s.SleepTemps[8]) {
		a[27] |= 16
	}
	if half(s.SleepTemps[9]) {
		a[27] |= 32
	}
	a[27] |= byte(15 & s.ComfortSleepTime)
	a[27] |= byte((8 & s.Pmv) << 4)
	a[27] |= byte((1 & s.NaturalWind) << 6)

	a[28] = byte((7 & s.Pmv) << 5)
	c := int(math.Round(10*s.TempSet2 + 0.5))
	a[28] |= byte((c/10 - 12) & 31)

	a[29] = byte(127 & s.Humidity)
	a[29] = a[29] + 128

	a[30] = byte((1 & s.DownWind) << 7)
	a[31] = 0 // 源码计算后强制 0
	a[32] = byte(s.CSEco << 0)
	a[33] = 0
	a[34] = byte(s.Order & 0xFF)
	a[35] = crc8854(a[10:], 25)
	a[1] = 36
	a[36] = makeSum(a[1:], 35)
	return a, nil
}

// BuildQueryFrame 构造查询 appliance 帧（opcode 0x41，optCommand=3/queryStat=2）。
func BuildQueryFrame(order int, sound int) []byte {
	a := make([]byte, 24)
	a[0] = 0xAA
	a[1] = 23
	a[2] = devTypeAC
	a[8] = 0
	a[9] = 3
	a[10] = 65
	a[11] = byte((sound&1)<<6 | 33)
	a[12] = 0
	a[13] = 255
	a[14] = 3
	a[15] = 255
	a[16] = 0
	a[17] = 2
	a[18] = 0
	a[19] = 0
	a[20] = 0
	a[21] = byte(order & 0xFF)
	a[22] = crc8854(a[10:], 12)
	a[23] = makeSum(a[1:], 22)
	return a
}

// Status 解析后的设备状态。
type Status struct {
	RunStatus  int
	FaultFlag  int
	Mode       int
	ModeName   string
	TempSet    float64
	WindSpeed  int
	WindName   string
	SwingUD    int
	SwingLR    int
	TempIn     float64
	TempOut    float64
	TempSet2   float64
	EcoFunc    int
	Strong     int
	ElecHeat   int
	ScreenShow int
	TankFull   bool
	ErrCode    int
	Raw        []byte
}

// ParseStatusFrame 解析 0xC0 状态回包。
func ParseStatusFrame(frame []byte) (*Status, error) {
	t := frame
	if len(t) == 0 || t[0] != 0xAA {
		return nil, fmt.Errorf("not an appliance frame: %x", t)
	}
	n := int(t[1]) + 1
	if makeSum(t[1:], n-1) != 0 {
		return nil, fmt.Errorf("checksum fail: %x", t)
	}
	if t[10] != 0xC0 {
		return nil, fmt.Errorf("非 0xC0 状态帧, opcode=%#x", t[10])
	}
	S := t[10:]
	d := &Status{Raw: frame}
	d.FaultFlag = b2i(128&S[1] != 0)
	d.RunStatus = b2i(1&S[1] != 0)
	d.Mode = int((224 & S[2]) >> 5)
	d.ModeName = nameOr(ModeNames, d.Mode)
	d.TempSet = float64(16 + (15 & S[2]))
	if 16&S[2] != 0 {
		d.TempSet += 0.5
	}
	d.WindSpeed = int(S[3])
	d.WindName = nameOr(WindNames, d.WindSpeed)

	switch {
	case S[7] < 16:
	case S[7] < 32:
	case S[7] < 48:
	case (240 & S[7]) == 48:
		d.SwingLR = b2i(S[7]&0x03 != 0)
		d.SwingUD = b2i(S[7]&0x0C != 0)
	}

	d.Strong = b2i(32&S[8] != 0)
	d.ElecHeat = b2i(8&S[9] != 0)
	d.EcoFunc = b2i(16&S[9] != 0)

	tin := math.Trunc((float64(S[11]) - 50) / 2)
	if tin < 0 {
		tin -= 0.1 * float64(S[15]&15)
	} else {
		tin += 0.1 * float64(S[15]&15)
	}
	d.TempIn = round1(tin)
	tout := (float64(S[12]) - 50) / 2
	tout += 0.1 * float64((S[15]>>4)&15)
	d.TempOut = round1(tout)

	d.TankFull = S[16] == 38
	d.TempSet2 = float64(12 + (31 & S[13]))
	if 16&S[2] != 0 {
		d.TempSet2 += 0.5
	}
	d.ScreenShow = int((112 & S[14]) >> 4)
	if len(S) > 16 {
		d.ErrCode = int(S[16])
	}
	return d, nil
}

func nameOr(m map[int]string, k int) string {
	if v, ok := m[k]; ok {
		return v
	}
	return fmt.Sprintf("%d", k)
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

// ApplyStatus 把回读状态灌入 ACState（读-改-写）。
func (s *ACState) ApplyStatus(st *Status) {
	s.RunStatus = st.RunStatus
	s.Mode = st.Mode
	s.TempSet = st.TempSet
	s.WindSpeed = st.WindSpeed
	s.EcoFunc = st.EcoFunc
	s.Strong = st.Strong
	s.ElecHeat = st.ElecHeat
	s.TempModeSwitch = 0
	if st.TempSet2 > 0 {
		s.TempSet2 = st.TempSet2
	}
}
