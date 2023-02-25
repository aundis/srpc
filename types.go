package srpc

import (
	"context"
	"time"
)

const DefaultTimeout = 60 * time.Second

const CallMark = "C"
const EmitMark = "E"
const ResponseMark = "R"
const ResponseErrMark = "RE"

type Message struct {
	Mark   string `json:"mark"`
	Name   string `json:"name"`
	Id     int64  `json:"id"`
	Action string `json:"action"`
	Data   []byte `json:"data"`
	Source []byte `json:"source"`
}

type RequestData struct {
	Mark    string        `json:"mark"`
	Target  string        `json:"target"`
	Id      int64         `json:"id"`
	Action  string        `json:"action"`
	Data    []byte        `json:"data"`
	Timeout time.Duration `json:"timeout"`
}

type ResponseData struct {
	Mark   string `json:"mark"`
	Source string `json:"source"`
	Id     int64  `json:"id"`
	Action string `json:"action"`
	Data   []byte `json:"data"`
}

type ErrorResponse struct {
	Value string `json:"value"`
}

type MessageHandle func(ctx context.Context, msg *Message) error
type ControllerHandle func(ctx context.Context, data []byte) (interface{}, error)
type SocketRequest func(ctx context.Context, data interface{}, timeoutOmit ...time.Duration) (interface{}, error)
