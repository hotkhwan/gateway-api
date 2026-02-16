// internal/iwown/iwownParser/alarm.go
package iwownParser

import (
	"fmt"

	"github.com/hotkhwan/gateway-api/internal/iwown/iwownConv"
	pb "github.com/hotkhwan/gateway-api/internal/iwown/protobuf"

	"google.golang.org/protobuf/proto"
)

type AlarmParsed struct {
	Kind       string           `json:"kind"`
	Opt        int              `json:"opt"`
	PayloadLen int              `json:"payloadLen"`
	Alarms     []AlarmEvent     `json:"alarms,omitempty"`
	DeviceInfo *AlarmDeviceInfo `json:"deviceInfo,omitempty"`
}

type AlarmEvent struct {
	Type      string `json:"type"`
	Time      string `json:"time,omitempty"`
	Value1    int32  `json:"value1,omitempty"`
	Value2    int32  `json:"value2,omitempty"`
	ExtraText string `json:"extraText,omitempty"`
}

type AlarmDeviceInfo struct {
	Time            string `json:"time,omitempty"`
	LowPowerPercent *int32 `json:"lowpowerPercentage,omitempty"`
	PowerOffPercent *int32 `json:"poweroffPercentage,omitempty"`
	NotWear         bool   `json:"notWear,omitempty"`
	InterceptNumber string `json:"interceptNumber,omitempty"`
	SleepState      bool   `json:"sleepState,omitempty"`
}

func ParseAlarmFrame(payload []byte, opt int) (*AlarmParsed, error) {
	var msg pb.AlarmInfokConfirm
	if err := proto.Unmarshal(payload, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal AlarmInfokConfirm: %w", err)
	}

	out := &AlarmParsed{
		Kind:       "alarm",
		Opt:        opt,
		PayloadLen: len(payload),
	}

	// alarm list
	if a := msg.GetAlarm(); a != nil {
		for _, x := range a.GetAlarmHr() {
			sec := int64(x.GetTimeStamp().GetDateTime().GetSeconds())
			out.Alarms = append(out.Alarms, AlarmEvent{
				Type:   "hr",
				Time:   iwownConv.ParsePbDateTime(sec),
				Value1: int32(x.GetHr()),
			})
		}

		for _, x := range a.GetAlarmSpo2() {
			sec := int64(x.GetTimeStamp().GetDateTime().GetSeconds())
			out.Alarms = append(out.Alarms, AlarmEvent{
				Type:   "spo2",
				Time:   iwownConv.ParsePbDateTime(sec),
				Value1: int32(x.GetSpo2()),
			})
		}

		for _, x := range a.GetAlarm_Thrombus() {
			sec := int64(x.GetTimeStamp().GetDateTime().GetSeconds())
			out.Alarms = append(out.Alarms, AlarmEvent{
				Type: "thrombus",
				Time: iwownConv.ParsePbDateTime(sec),
			})
		}

		for _, x := range a.GetAlarmFall() {
			sec := int64(x.GetTimeStamp().GetDateTime().GetSeconds())
			out.Alarms = append(out.Alarms, AlarmEvent{
				Type: "fall",
				Time: iwownConv.ParsePbDateTime(sec),
			})
		}

		for _, x := range a.GetAlarm_Temperature() {
			sec := int64(x.GetTimeStamp().GetDateTime().GetSeconds())
			out.Alarms = append(out.Alarms, AlarmEvent{
				Type:   "temperature",
				Time:   iwownConv.ParsePbDateTime(sec),
				Value1: int32(x.GetTemperature()),
			})
		}

		for _, x := range a.GetAlarm_Bp() {
			sec := int64(x.GetTimeStamp().GetDateTime().GetSeconds())
			out.Alarms = append(out.Alarms, AlarmEvent{
				Type:   "bp",
				Time:   iwownConv.ParsePbDateTime(sec),
				Value1: int32(x.GetSbp()),
				Value2: int32(x.GetDbp()),
			})
		}

		for _, x := range a.GetAlarm_Sedentary() {
			sec := int64(x.GetTimeStamp().GetDateTime().GetSeconds())
			out.Alarms = append(out.Alarms, AlarmEvent{
				Type: "sedentary",
				Time: iwownConv.ParsePbDateTime(sec),
			})
		}

		// SOS_Notification_time (single)
		if t := a.GetSOS_NotificationTime(); t != nil {
			sec := int64(t.GetDateTime().GetSeconds())
			out.Alarms = append(out.Alarms, AlarmEvent{
				Type: "sos",
				Time: iwownConv.ParsePbDateTime(sec),
			})
		}

		for _, x := range a.GetAlarm_BloodPotassium() {
			sec := int64(x.GetTimeStamp().GetDateTime().GetSeconds())
			out.Alarms = append(out.Alarms, AlarmEvent{
				Type:   "blood_potassium",
				Time:   iwownConv.ParsePbDateTime(sec),
				Value1: int32(x.GetBloodPotassium()),
			})
		}

		for _, x := range a.GetAlarm_BloodSugar() {
			sec := int64(x.GetTimeStamp().GetDateTime().GetSeconds())
			out.Alarms = append(out.Alarms, AlarmEvent{
				Type:   "blood_sugar",
				Time:   iwownConv.ParsePbDateTime(sec),
				Value1: int32(x.GetBloodSugar()),
			})
		}
	}

	// device flags (alarminfo)
	if info := msg.GetAlarminfo(); info != nil {
		sec := int64(info.GetTimeStamp().GetDateTime().GetSeconds())
		dev := &AlarmDeviceInfo{Time: iwownConv.ParsePbDateTime(sec)}

		if info.LowpowerPercentage != nil {
			v := int32(info.GetLowpowerPercentage())
			dev.LowPowerPercent = &v
		}
		if info.PoweroffPercentage != nil {
			v := int32(info.GetPoweroffPercentage())
			dev.PowerOffPercent = &v
		}
		if info.Wearstate != nil {
			dev.NotWear = true
		}
		if info.InterceptNumber != nil {
			dev.InterceptNumber = info.GetInterceptNumber()
		}
		if info.Sleepstate != nil {
			dev.SleepState = true
		}

		out.DeviceInfo = dev
	}

	return out, nil
}
