package module

import (
	"encoding/json"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/yrzs/openimwssdk/common"
	"github.com/yrzs/openimwssdk/gate"
)

var ProgressStartTime int64

func NewStatusAgent(a gate.Agent) {
	aUerData := a.UserData().(*common.TAgentUserData)
	log.Info("one status ws connect", "sessionId", aUerData.SessionID)
	actor, err := NewStatusActor(a, aUerData.SessionID, nil)
	if err != nil {
		log.Error("NewStatusActor error", "err", err, "sessionId", aUerData.SessionID)
		res := &ResponseSt{Type: RESP_OP_TYPE, Success: false, ErrMsg: "NewMQActor error"}
		resb, _ := json.Marshal(res)
		resSend := &common.TWSData{MsgType: common.MessageText, Msg: resb}
		a.WriteMsg(resSend)
		a.Close()
		return
	}
	aUerData.ProxyBody = actor
	a.SetUserData(aUerData)
	log.Info("one status linked", "sessionId", aUerData.SessionID)

}
func CloseStatusAgent(a gate.Agent) {
	aUerData := a.UserData().(*common.TAgentUserData)
	if aUerData.ProxyBody != nil {
		aUerData.ProxyBody.(MActor).Destroy()
		aUerData.ProxyBody = nil
	}
	log.Info("one  status dislinkder", "sessionId", a.UserData().(*common.TAgentUserData).SessionID)
}
func DataRecvStatus(data interface{}, a gate.Agent) {
	aUerData := a.UserData().(*common.TAgentUserData)
	if aUerData.ProxyBody != nil {
		err := aUerData.ProxyBody.(MActor).ProcessRecvMsg(data)
		if err != nil {
			log.Error("溢出错误", "sessionId", aUerData.SessionID)
			a.Destroy()
		}
	}
}
