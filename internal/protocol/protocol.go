// Package protocol 定义 AP/1 自动化协议 (PLAN.md §5.3)。
//
// 传输: JSON Lines over stdio (主), 每行一个 JSON 对象。
// 接入模式: headless = 只接后端 (只有音乐); full = 前端 + 后端 (音乐 + 动画)。
// 规范全文见 docs/AP1.md。
package protocol

// Version AP 协议版本, 写在 hello 帧里, skill 据此兼容新旧 exe。
const Version = 1

// ---- AI → pianostage (命令) ----

// Command 一条自动化命令。
type Command struct {
	Op string `json:"op"` // ping|load|play|pause|stop|seek|tempo|note|program|cc|backend|query

	Source  string `json:"source,omitempty"`  // load: 文件路径或曲名
	Backend string `json:"backend,omitempty"` // backend: 后端名 (midiout)
	Device  string `json:"device,omitempty"`  // backend: 设备名/序号/"auto"
	Query   string `json:"query,omitempty"`   // query: devices|status|time
	Headless bool  `json:"headless,omitempty"`

	Position float64 `json:"position,omitempty"` // seek: 目标秒
	Tempo    float64 `json:"tempo,omitempty"`    // tempo: 播放速率倍率 (1.0 正常, 0.5 半速)

	On      bool `json:"on,omitempty"`      // note: true=按下 false=释放
	Ch      int  `json:"ch,omitempty"`      // 声道 0~15
	Note    int  `json:"note,omitempty"`    // 音符 0~127
	Vel     int  `json:"vel,omitempty"`     // 力度 0~127
	Prog    int  `json:"prog,omitempty"`    // GM 乐器编号
	Ctrl    int  `json:"ctrl,omitempty"`    // CC 控制器号
	CtrlVal int  `json:"ctrlval,omitempty"` // CC 值
}

// ---- pianostage → AI (回执 / 事件) ----

// Message 一条回执或事件。
type Message struct {
	Type   string `json:"type"`            // hello|ok|err|state|time|note
	Ap     int    `json:"ap,omitempty"`    // hello: 协议版本
	Op     string `json:"op,omitempty"`    // ok/err: 对应的命令 op
	Code   string `json:"code,omitempty"`  // err: 错误码 (见下)
	Detail string `json:"detail,omitempty"`
	Data   any    `json:"data,omitempty"`  // state/query 结果/note 详情
	T      float64 `json:"t,omitempty"`    // time/note: 曲内时间 (秒)
}

// 错误码。
const (
	ErrProto   = "E_PROTO"   // 非法 JSON / 未知字段形态
	ErrOp      = "E_OP"      // 未知 op
	ErrRange   = "E_RANGE"   // 字段越界
	ErrLoad    = "E_LOAD"    // 谱面加载失败
	ErrBackend = "E_BACKEND" // 后端不存在/未支持
	ErrDevice  = "E_DEVICE"  // 设备不存在/打开失败
	ErrState   = "E_STATE"   // 状态机非法迁移 (如未 load 就 play)
	ErrInternal = "E_INTERNAL"
)

// Hello 进程就绪的第一帧。
func Hello() Message { return Message{Type: "hello", Ap: Version} }

// OK 命令成功回执。
func OK(op string) Message { return Message{Type: "ok", Op: op} }

// Err 命令失败回执。
func Err(op, code, detail string) Message {
	return Message{Type: "err", Op: op, Code: code, Detail: detail}
}

// TimeTick 时钟广播 (调度器主时钟的曲内时间)。
func TimeTick(t float64) Message { return Message{Type: "time", T: t} }
