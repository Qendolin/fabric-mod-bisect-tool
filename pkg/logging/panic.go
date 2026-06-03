package logging

import (
	"runtime/debug"
)

type PanicPayload struct {
	Value interface{}
	Stack []byte
}

var PanicChannel chan PanicPayload

func init() {
	PanicChannel = make(chan PanicPayload, 1)
}

func HandlePanic() {
	if r := recover(); r != nil {
		Errorf("Main: Panic recovered: %v\n%s", r, string(debug.Stack()))
		PanicChannel <- PanicPayload{Value: r, Stack: debug.Stack()}
	}
}
