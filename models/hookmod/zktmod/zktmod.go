// models/hookmod/zktmod/zktmod.go
package zktmod

type HookResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg,omitempty"`
}

type ZlmOnPlayReq struct {
	MediaServerId string `json:"mediaServerId"`
	App           string `json:"app"`
	Id            string `json:"id"`
	Ip            string `json:"ip"`
	Params        string `json:"params"`
	Port          int    `json:"port"`
	Schema        string `json:"schema"`
	Protocol      string `json:"protocol"`
	Stream        string `json:"stream"`
	Vhost         string `json:"vhost"`
}

type ZlmOnPublishReq struct {
	MediaServerId string `json:"mediaServerId"`
	App           string `json:"app"`
	Id            string `json:"id"`
	Ip            string `json:"ip"`
	Params        string `json:"params"`
	Port          int    `json:"port"`
	Schema        string `json:"schema"`
	Vhost         string `json:"vhost"`
	Stream        string `json:"stream"`
	// อื่น ๆ ที่ ZLM ส่งมาได้ เช่น "regist" เป็นต้น (ถ้ามี)
}

type ZlmOnStreamNoneReaderReq struct {
	MediaServerId string `json:"mediaServerId"`
	Schema        string `json:"schema"`
	Vhost         string `json:"vhost"`
	App           string `json:"app"`
	Stream        string `json:"stream"`
}

// ZLM on_stream_none_reader payload (field หลักที่ใช้จริง)
type OnStreamNotFoundReq struct {
	Schema        string `json:"schema"`
	Vhost         string `json:"vhost"`
	App           string `json:"app"`
	Stream        string `json:"stream"`
	MediaServerId string `json:"mediaServerId"`
}
