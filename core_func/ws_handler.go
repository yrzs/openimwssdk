package core_func

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/yrzs/openimsdkcore/open_im_sdk_callback"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/yrzs/openimsdkcore/open_im_sdk"
	"github.com/yrzs/openimsdkcore/pkg/ccontext"
	"github.com/yrzs/openimsdkcore/pkg/sdkerrs"
	"github.com/yrzs/openimsdkcore/pkg/utils"
)

const (
	Success = "OnSuccess"
	Failed  = "OnError"
)

type EventData struct {
	Event       string `json:"event"`
	ErrCode     int32  `json:"errCode"`
	ErrMsg      string `json:"errMsg"`
	Data        string `json:"data"`
	OperationID string `json:"operationID"`
}

type FuncRouter struct {
	userForSDK  *open_im_sdk.LoginMgr
	respMessage *RespMessage
	sessionId   string
}

// NewFuncRouter 创建并返回一个FuncRouter实例
//
//	respMessagesChan: 用于接收事件数据的通道
//	sessionId: 会话ID
//	*FuncRouter: 指向新创建的FuncRouter实例的指针
func NewFuncRouter(respMessagesChan chan *EventData, sessionId string) *FuncRouter {
	return &FuncRouter{respMessage: NewRespMessage(respMessagesChan),
		userForSDK: new(open_im_sdk.LoginMgr), sessionId: sessionId}
}

// call 函数用于异步调用指定的函数，并处理调用结果
//
// 使用go关键字启动一个新的goroutine来异步调用指定的函数fn，并处理调用结果。
// 首先，通过reflect包获取函数指针，并使用runtime包获取函数名。
// 然后，对函数名进行分割处理，获取实际的函数名称。
// 如果处理后的函数名为空，则通过respMessage发送错误响应。
// 否则，调用call_函数执行实际的函数调用，并处理调用结果。
// 如果调用成功，则将结果转换为JSON字符串，并通过respMessage发送成功响应。
// 如果调用失败或结果转换失败，则通过respMessage发送错误响应。
//
//     operationID: 操作ID，用于标识请求的唯一性
//     fn: 要调用的函数
//     args: 传递给函数的参数列表

func (f *FuncRouter) call(operationID string, fn any, args ...any) {
	go func() {
		funcPtr := reflect.ValueOf(fn).Pointer()
		funcName := runtime.FuncForPC(funcPtr).Name()
		parts := strings.Split(funcName, ".")
		var trimFuncName string
		if trimFuncNameList := strings.Split(parts[len(parts)-1], "-"); len(trimFuncNameList) == 0 {
			f.respMessage.sendOnErrorResp(operationID, "FuncError", errors.New("call function trimFuncNameList is empty"))
			return
		} else {
			trimFuncName = trimFuncNameList[0]
		}
		res, err := f.call_(operationID, fn, funcName, args...)
		if err != nil {
			f.respMessage.sendOnErrorResp(operationID, trimFuncName, err)
			return
		}
		data, err := json.Marshal(res)
		if err != nil {
			f.respMessage.sendOnErrorResp(operationID, trimFuncName, err)
			return
		} else {
			f.respMessage.sendOnSuccessResp(operationID, trimFuncName, string(data))
		}
	}()
}

// CheckResourceLoad checks the SDK is resource load status.
func CheckResourceLoad(uSDK *open_im_sdk.LoginMgr, funcName string) error {
	if uSDK == nil {
		return utils.Wrap(errors.New("CheckResourceLoad failed uSDK == nil "), "")
	}
	if funcName == "" {
		return nil
	}
	parts := strings.Split(funcName, ".")
	if parts[len(parts)-1] == "Login-fm" {
		return nil
	}
	if uSDK.Friend() == nil || uSDK.User() == nil || uSDK.Group() == nil || uSDK.Conversation() == nil ||
		uSDK.Full() == nil {
		return utils.Wrap(errors.New("CheckResourceLoad failed, resource nil "), "")
	}
	return nil
}

// call_ is the internal function that actually invokes the SDK functions.
func (f *FuncRouter) call_(operationID string, fn any, funcName string, args ...any) (res any, err error) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("panic: %+v\n%s", r, debug.Stack())
			err = fmt.Errorf("call panic: %+v", r)
		}
	}()
	if operationID == "" {
		return nil, sdkerrs.ErrArgs.Wrap("call function operationID is empty")
	}
	if err := CheckResourceLoad(f.userForSDK, funcName); err != nil {
		return nil, sdkerrs.ErrResourceLoad.Wrap("not load resource")
	}

	ctx := ccontext.WithOperationID(f.userForSDK.BaseCtx(), operationID)

	fnv := reflect.ValueOf(fn)
	if fnv.Kind() != reflect.Func {
		return nil, sdkerrs.ErrSdkInternal.Wrap(fmt.Sprintf("call function fn is not function, is %T", fn))
	}
	fnt := fnv.Type()
	nin := fnt.NumIn()
	if len(args)+1 != nin {
		return nil, sdkerrs.ErrSdkInternal.Wrap(fmt.Sprintf("go code error: fn in args num is not match"))
	}
	t := time.Now()
	log.Info(ctx, "input req", "function name", funcName, "args", args)
	ins := make([]reflect.Value, 0, nin)
	ins = append(ins, reflect.ValueOf(ctx))
	for i := 0; i < len(args); i++ {
		inFnField := fnt.In(i + 1)
		arg := reflect.TypeOf(args[i])
		if arg.String() == inFnField.String() || inFnField.Kind() == reflect.Interface {
			ins = append(ins, reflect.ValueOf(args[i]))
			continue
		}
		//convert float64 to int when javascript call with number,because javascript only have double
		//precision floating-point format
		if arg.String() == "float64" && isInteger(inFnField) {
			ins = append(ins, reflect.ValueOf(convert(args[i].(float64), inFnField)))
			continue
		}
		if arg.Kind() == reflect.String { // json
			var ptr int
			for inFnField.Kind() == reflect.Ptr {
				inFnField = inFnField.Elem()
				ptr++
			}
			switch inFnField.Kind() {
			case reflect.Struct, reflect.Slice, reflect.Array, reflect.Map:
				v := reflect.New(inFnField)
				if err := json.Unmarshal([]byte(args[i].(string)), v.Interface()); err != nil {
					return nil, sdkerrs.ErrSdkInternal.Wrap(fmt.Sprintf("go call json.Unmarshal error: %s",
						err))
				}
				if ptr == 0 {
					v = v.Elem()
				} else if ptr != 1 {
					for i := ptr - 1; i > 0; i-- {
						temp := reflect.New(v.Type())
						temp.Elem().Set(v)
						v = temp
					}
				}
				ins = append(ins, v)
				continue
			}
		}
		return nil, sdkerrs.ErrSdkInternal.Wrap(fmt.Sprintf("go code error: fn in args type is not match"))
	}
	outs := fnv.Call(ins)
	if len(outs) == 0 {
		return "", nil
	}
	if fnt.Out(len(outs) - 1).Implements(reflect.ValueOf(new(error)).Elem().Type()) {
		if errValueOf := outs[len(outs)-1]; !errValueOf.IsNil() {
			log.Error(ctx, "fn call error", errValueOf.Interface().(error), "function name",
				funcName, "cost time", time.Since(t))
			return nil, errValueOf.Interface().(error)
		}
		if len(outs) == 1 {
			return "", nil
		}
		outs = outs[:len(outs)-1]
	}
	for i := 0; i < len(outs); i++ {
		out := outs[i]
		switch out.Kind() {
		case reflect.Map:
			if out.IsNil() {
				outs[i] = reflect.MakeMap(out.Type())
			}
		case reflect.Slice:
			if out.IsNil() {
				outs[i] = reflect.MakeSlice(out.Type(), 0, 0)
			}
		}
	}
	if len(outs) == 1 {
		log.Info(ctx, "output resp", "function name", funcName, "resp", outs[0].Interface(),
			"cost time", time.Since(t))
		return outs[0].Interface(), nil
	}
	val := make([]any, 0, len(outs))
	for i := range outs {
		val = append(val, outs[i].Interface())
	}
	log.Info(ctx, "output resp", "function name", funcName, "resp", val, "cost time", time.Since(t))
	return val, nil
}
func isInteger(arg reflect.Type) bool {
	switch arg.Kind() {
	case reflect.Int:
		fallthrough
	case reflect.Int8:
		fallthrough
	case reflect.Int16:
		fallthrough
	case reflect.Int32:
		fallthrough
	case reflect.Int64:
		return true
	default:
		return false

	}
}
func convert(arg float64, p reflect.Type) any {
	switch p.Kind() {
	case reflect.Int:
		return int(arg)
	case reflect.Int8:
		return int8(arg)
	case reflect.Int16:
		return int16(arg)
	case reflect.Int32:
		return int32(arg)
	case reflect.Int64:
		return int64(arg)
	default:
		return arg

	}
}
func (f *FuncRouter) messageCall(operationID string, fn any, args ...any) {
	go func() {
		funcPtr := reflect.ValueOf(fn).Pointer()
		funcName := runtime.FuncForPC(funcPtr).Name()
		parts := strings.Split(funcName, ".")
		var trimFuncName string
		if trimFuncNameList := strings.Split(parts[len(parts)-1], "-"); len(trimFuncNameList) == 0 {
			f.respMessage.sendOnErrorResp(operationID, "FuncError",
				errors.New("call function trimFuncNameList is empty"))
			return
		} else {
			trimFuncName = trimFuncNameList[0]
		}
		sendMessageCallback := NewSendMessageCallback(trimFuncName, f.respMessage)
		res, err := f.messageCall_(sendMessageCallback, operationID, fn, funcName, args...)
		if err != nil {
			f.respMessage.sendOnErrorResp(operationID, trimFuncName, err)
			return
		}
		data, err := json.Marshal(res)
		if err != nil {
			f.respMessage.sendOnErrorResp(operationID, trimFuncName, err)
			return
		} else {
			f.respMessage.sendOnSuccessResp(operationID, trimFuncName, string(data))
		}
	}()
}
func (f *FuncRouter) messageCall_(callback open_im_sdk_callback.SendMsgCallBack, operationID string,
	fn any, funcName string, args ...any) (res any, err error) {

	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("panic: %+v\n%s", r, debug.Stack())
			err = fmt.Errorf("call panic: %+v", r)
		}
	}()
	if operationID == "" {
		return nil, sdkerrs.ErrArgs.Wrap("call function operationID is empty")
	}
	if err := CheckResourceLoad(f.userForSDK, ""); err != nil {
		return nil, sdkerrs.ErrResourceLoad.Wrap("not load resource")
	}

	ctx := ccontext.WithOperationID(f.userForSDK.BaseCtx(), operationID)
	ctx = ccontext.WithSendMessageCallback(ctx, callback)

	fnv := reflect.ValueOf(fn)
	if fnv.Kind() != reflect.Func {
		return nil, sdkerrs.ErrSdkInternal.Wrap(fmt.Sprintf("call function fn is not function, is %T", fn))
	}
	log.Info(ctx, "input req", "function name", funcName, "args", args)
	fnt := fnv.Type()
	nin := fnt.NumIn()
	if len(args)+1 != nin {
		return nil, sdkerrs.ErrSdkInternal.Wrap(fmt.Sprintf("go code error: fn in args num is not match"))
	}
	t := time.Now()
	ins := make([]reflect.Value, 0, nin)
	ins = append(ins, reflect.ValueOf(ctx))
	for i := 0; i < len(args); i++ {
		inFnField := fnt.In(i + 1)
		arg := reflect.TypeOf(args[i])
		if arg.String() == inFnField.String() || inFnField.Kind() == reflect.Interface {
			ins = append(ins, reflect.ValueOf(args[i]))
			continue
		}
		if arg.String() == "float64" && isInteger(inFnField) {
			ins = append(ins, reflect.ValueOf(convert(args[i].(float64), inFnField)))
			continue
		}
		if arg.Kind() == reflect.String { // json
			var ptr int
			for inFnField.Kind() == reflect.Ptr {
				inFnField = inFnField.Elem()
				ptr++
			}
			switch inFnField.Kind() {
			case reflect.Struct, reflect.Slice, reflect.Array, reflect.Map:
				v := reflect.New(inFnField)
				if err := json.Unmarshal([]byte(args[i].(string)), v.Interface()); err != nil {
					return nil, sdkerrs.ErrSdkInternal.Wrap(fmt.Sprintf("go call json.Unmarshal error: %s",
						err))
				}
				if ptr == 0 {
					v = v.Elem()
				} else if ptr != 1 {
					for i := ptr - 1; i > 0; i-- {
						temp := reflect.New(v.Type())
						temp.Elem().Set(v)
						v = temp
					}
				}
				ins = append(ins, v)
				continue
			}
		}
		return nil, sdkerrs.ErrSdkInternal.Wrap(fmt.Sprintf("go code error: fn in args type is not match"))
	}
	outs := fnv.Call(ins)
	if len(outs) == 0 {
		return "", nil
	}
	if fnt.Out(len(outs) - 1).Implements(reflect.ValueOf(new(error)).Elem().Type()) {
		if errValueOf := outs[len(outs)-1]; !errValueOf.IsNil() {
			log.Error(ctx, "fn call error", errValueOf.Interface().(error), "function name",
				funcName, "cost time", time.Since(t))
			return nil, errValueOf.Interface().(error)
		}
		if len(outs) == 1 {
			return "", nil
		}
		outs = outs[:len(outs)-1]
	}
	for i := 0; i < len(outs); i++ {
		out := outs[i]
		switch out.Kind() {
		case reflect.Map:
			if out.IsNil() {
				outs[i] = reflect.MakeMap(out.Type())
			}
		case reflect.Slice:
			if out.IsNil() {
				outs[i] = reflect.MakeSlice(out.Type(), 0, 0)
			}
		default:
			log.Errorf("unhandled default case out.Kind():%v", out.Kind())
		}
	}
	if len(outs) == 1 {
		log.Info(ctx, "output resp", "function name", funcName, "resp", outs[0].Interface(),
			"cost time", time.Since(t))
		return outs[0].Interface(), nil
	}
	val := make([]any, 0, len(outs))
	for i := range outs {
		val = append(val, outs[i].Interface())
	}
	log.Info(ctx, "output resp", "function name", funcName, "resp", val, "cost time", time.Since(t))
	return val, nil
}
