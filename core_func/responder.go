package core_func

import (
	"errors"
	"github.com/go-kratos/kratos/v2/log"

	"github.com/yrzs/openimsdktools/errs"
)

type RespMessage struct {
	respMessagesChan chan *EventData
}

// NewRespMessage 创建一个新的RespMessage对象
//
// respMessagesChan: 用于接收事件数据的通道
// *RespMessage: 指向新创建的RespMessage对象的指针
func NewRespMessage(respMessagesChan chan *EventData) *RespMessage {
	return &RespMessage{respMessagesChan: respMessagesChan}
}

// sendOnSuccessResp 在操作成功时发送响应消息
//
//	operationID: 操作ID，用于标识请求的唯一性
//	event: 事件类型
//	data: 响应数据
func (r *RespMessage) sendOnSuccessResp(operationID, event string, data string) {
	r.respMessagesChan <- &EventData{
		Event:       event,
		OperationID: operationID,
		Data:        data,
	}
}

// sendOnErrorResp 在操作失败时发送错误响应消息
// 将错误信息封装在EventData结构体中，并通过respMessagesChan通道发送出去。
// 如果err实现了errs.CodeError接口，则将错误码和错误消息填充到EventData中。
//
//	operationID: 操作ID，用于标识请求的唯一性
//	event: 事件类型
//	err: 发生的错误
func (r *RespMessage) sendOnErrorResp(operationID, event string, err error) {
	log.Errorf("SendOnErrorResp operationID: %sevent: %serr: %v", operationID, event, err)
	resp := &EventData{
		Event:       event,
		OperationID: operationID,
	}
	var code errs.CodeError
	if errors.As(err, &code) {
		resp.ErrCode = int32(code.Code())
		resp.ErrMsg = code.Error()
	}
	r.respMessagesChan <- resp
}

// sendEventFailedRespNoErr 在事件处理失败但没有具体错误信息时发送响应消息
// 创建一个EventData对象，仅包含事件类型，并通过respMessagesChan通道发送出去，表示事件处理失败但没有具体的错误信息。
//
//	event: 事件类型
func (r *RespMessage) sendEventFailedRespNoErr(event string) {
	r.respMessagesChan <- &EventData{
		Event: event,
	}
}

// sendEventSuccessRespWithData 在事件处理成功时发送带有数据的响应消息
// 创建一个EventData对象，包含事件类型和数据，并通过respMessagesChan通道发送出去，表示事件处理成功并携带了相关数据。
//
//	event: 事件类型
//	data: 与事件相关的数据
func (r *RespMessage) sendEventSuccessRespWithData(event string, data string) {
	r.respMessagesChan <- &EventData{
		Event: event,
		Data:  data,
	}
}

// sendEventSuccessRespNoData 在事件处理成功但无需返回数据时发送响应消息
// 创建一个EventData对象，仅包含事件类型，并通过respMessagesChan通道发送出去，表示事件处理成功但没有数据返回。
//
//	event: 事件类型
func (r *RespMessage) sendEventSuccessRespNoData(event string) {
	r.respMessagesChan <- &EventData{
		Event: event,
	}
}

// sendEventFailedRespNoData 在事件处理失败但无需返回数据时发送响应消息
// 创建一个EventData对象，包含事件类型、错误码和错误信息，并通过respMessagesChan通道发送出去，表示事件处理失败但没有数据返回。
//
//	event: 事件类型
//	errCode: 错误码
//	errMsg: 错误信息
func (r *RespMessage) sendEventFailedRespNoData(event string, errCode int32, errMsg string) {
	r.respMessagesChan <- &EventData{
		Event:   event,
		ErrCode: errCode,
		ErrMsg:  errMsg,
	}
}
