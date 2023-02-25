package srpc

import (
	"bytes"
	"context"
	"errors"
	"strings"

	"github.com/gogf/gf/v2/container/gmap"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gmutex"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/gorilla/websocket"
)

type Client struct {
	name       string
	addr       string
	conn       *Conn
	counter    int64
	genMutex   *gmutex.Mutex
	response   *gmap.IntAnyMap
	controller *gmap.StrAnyMap
	listens    *gmap.StrIntMap
}

func NewClient(name string, addr string) *Client {
	r := &Client{
		name:       name,
		addr:       addr,
		conn:       nil,
		counter:    0,
		genMutex:   gmutex.New(),
		response:   gmap.NewIntAnyMap(true),
		controller: gmap.NewStrAnyMap(true),
		listens:    gmap.NewStrIntMap(true),
	}
	return r
}

func (c *Client) generateId() int64 {
	c.genMutex.Lock()
	defer c.genMutex.Unlock()
	c.counter++
	return c.counter
}

func (c *Client) SetController(controller map[string]ControllerHandle) {
	for k, v := range controller {
		c.controller.Set(k, v)
	}
}

func (c *Client) SetListens(names []string) {
	c.listens.Clear()
	for _, v := range names {
		c.listens.Set(v, 1)
	}
}

func (c *Client) IsListenTo(name string) bool {
	return c.listens.Get(name) == 1
}

// Request send request and wait response
func (c *Client) Request(ctx context.Context, req RequestData) ([]byte, error) {
	if req.Timeout == 0 {
		req.Timeout = DefaultTimeout
	}
	if req.Id == 0 {
		req.Id = c.generateId()
	}

	ctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	ch := make(chan ResponseData)
	c.response.Set(int(req.Id), ch)
	defer c.response.Remove(int(req.Id))

	err := c.conn.WriteBinMessage(req.Mark, req.Target, req.Id, req.Action, req.Data)
	if err != nil {
		return nil, err
	}

	select {
	case rsp := <-ch:
		if rsp.Mark == "RE" {
			var ep *ErrorResponse
			err = gconv.Struct(rsp.Data, &ep)
			if err != nil {
				return nil, err
			}
			return nil, errors.New(ep.Value)
		}

		return rsp.Data, nil
	case <-ctx.Done():
		return nil, gerror.New("request timeout")
	}
}

func (c *Client) Start(ctx context.Context, hook ...MessageHandle) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var handle MessageHandle
	if len(hook) > 0 {
		handle = hook[0]
	} else {
		handle = c.handleMessage
	}
	for {
		tpe, data, err := c.conn.ReadMessage()
		if err != nil {
			return err
		}
		if tpe != websocket.TextMessage {
			continue
		}
		if len(data) == 0 {
			continue
		}
		msg, err := c.parseMessage(data)
		if err != nil {
			continue
		}

		go handle(ctx, msg)
	}
}

func (c *Client) parseMessage(msg []byte) (*Message, error) {
	index := bytes.IndexByte(msg, '{')
	if index < 0 {
		return nil, errors.New("message format error")
	}

	headStr := string(msg[0:index])
	infos := strings.Split(headStr, ",")
	if len(infos) != 4 {
		return nil, errors.New("head info length error")
	}

	mark := infos[0]
	if len(mark) == 0 {
		return nil, errors.New("mark is empty")
	}

	return &Message{
		Mark:   mark,
		Name:   infos[1],
		Id:     gconv.Int64(infos[2]),
		Action: infos[3],
		Data:   msg[index:],
		Source: msg,
	}, nil
}

func (c *Client) handleMessage(ctx context.Context, msg *Message) (err error) {
	defer func() {
		if err != nil {
			c.conn.WriteJsonMessage(ResponseErrMark, msg.Name, msg.Id, msg.Action, ErrorResponse{
				Value: err.Error(),
			})
		}
	}()

	switch msg.Mark {
	case CallMark:
		err = c.handleCall(ctx, msg)
	case ResponseMark, ResponseErrMark:
		err = c.handleResponse(ctx, msg)
	default:
		err = errors.New("not support mark")
	}

	return
}

func (c *Client) handleCall(ctx context.Context, msg *Message) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	handle := c.controller.Get(msg.Action)
	if handle == nil {
		return errors.New("not found call " + msg.Action)
	}

	data, err := handle.(ControllerHandle)(ctx, msg.Data)
	if err != nil {
		// ignore this error result
		c.conn.WriteJsonMessage(ResponseErrMark, msg.Name, msg.Id, msg.Action, ErrorResponse{
			Value: err.Error(),
		})
	}
	// success call handle
	err = c.conn.WriteJsonMessage(ResponseMark, msg.Name, msg.Id, msg.Action, data)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) handleResponse(ctx context.Context, msg *Message) error {
	ch := c.response.Remove(int(msg.Id))
	if ch != nil {
		ch.(chan ResponseData) <- ResponseData{
			Mark:   msg.Mark,
			Source: msg.Name,
			Id:     msg.Id,
			Action: msg.Action,
			Data:   msg.Data,
		}
	}

	return nil
}
