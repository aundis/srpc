package srpc

import (
	"context"
	"errors"
	"sync"

	"github.com/gogf/gf/v2/container/garray"
	"github.com/gogf/gf/v2/container/gmap"
	"github.com/gogf/gf/v2/os/gctx"
	"github.com/gorilla/websocket"
)

func NewServer(name string) *Server {
	return &Server{
		name:       name,
		clients:    gmap.NewStrAnyMap(true),
		controller: gmap.NewStrAnyMap(true),
	}
}

type Server struct {
	name       string
	clients    *gmap.StrAnyMap
	controller *gmap.StrAnyMap
}

func (s *Server) SetController(controller map[string]ControllerHandle) {
	for k, v := range controller {
		s.controller.Set(k, v)
	}
}

func (s *Server) handleMessage(ctx context.Context, sender *Client, msg *Message) (err error) {
	defer func() {
		if err != nil {
			sender.conn.WriteJsonMessage(ResponseErrMark, sender.name, msg.Id, msg.Action, ErrorResponse{
				Value: err.Error(),
			})
		}
	}()

	switch msg.Mark {
	case CallMark:
		err = s.handleCall(ctx, sender, msg)
	case EmitMark:
		err = s.handleEmit(ctx, sender, msg)
	case ResponseMark, ResponseErrMark:
		err = s.handleResponse(ctx, sender, msg)
	}

	return
}

func (s *Server) handleCall(ctx context.Context, sender *Client, msg *Message) error {
	if msg.Name == s.name {
		// client call server slot
		return s.handleCallContoller(ctx, sender, msg)
	}

	// call to other client
	client := s.GetClient(msg.Name)
	if client == nil {
		return errors.New("not found client " + msg.Name)
	}
	// change source client name
	err := client.conn.WriteBinMessage(msg.Mark, sender.name, msg.Id, msg.Action, msg.Data)
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) handleCallContoller(ctx context.Context, sender *Client, msg *Message) error {
	handle := s.controller.Get(msg.Action)
	if handle == nil {
		return errors.New("not found action " + msg.Action)
	}
	r, err := handle.(ControllerHandle)(ctx, msg.Data)
	if err != nil {
		return err
	}
	err = sender.conn.WriteJsonMessage(ResponseMark, s.name, msg.Id, msg.Action, r)
	if err != nil {
		return err
	}

	return nil
}

func (s *Server) handleEmit(ctx context.Context, sender *Client, msg *Message) error {
	var monitors []*Client
	action := sender.name + "@" + msg.Action
	s.clients.RLockFunc(func(m map[string]interface{}) {
		for _, v := range m {
			c := v.(*Client)
			if c.IsListenTo(action) {
				monitors = append(monitors, c)
			}
		}
	})

	errs := garray.New(true)
	wg := sync.WaitGroup{}
	for _, v := range monitors {
		wg.Add(1)
		go func(client *Client) {
			defer wg.Done()
			_, err := client.Request(ctx, RequestData{
				Mark:   CallMark,
				Target: sender.name,
				Action: action,
				Data:   msg.Data,
			})
			if err != nil {
				errs.Append(err)
			}
		}(v)
	}
	wg.Wait()

	if errs.Len() > 0 {
		return errs.At(0).(error)
	}

	// response to sender
	err := sender.conn.WriteBinMessage(ResponseMark, s.name, msg.Id, msg.Action, []byte("{}"))
	if err != nil {
		return err
	}

	return nil
}

func (s *Server) handleResponse(ctx context.Context, sender *Client, msg *Message) error {
	if msg.Name == s.name {
		// must call client response handle, because maybe has request wait this response
		// if you want to call client request, must set message target is server name
		return sender.handleResponse(ctx, msg)
	}

	// response to other client
	target := s.GetClient(msg.Name)
	if target == nil {
		return errors.New("not found client " + msg.Name)
	}

	err := target.conn.WriteData(websocket.TextMessage, msg.Source)
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) GetClient(name string) *Client {
	client := s.clients.Get(name)
	if client != nil {
		return client.(*Client)
	}
	return nil
}

func (s *Server) AddClient(client *Client) {
	go func() {
		s.clients.Set(client.name, client)
		defer s.clients.Remove(client.name)

		wrapper := func(ctx context.Context, msg *Message) error {
			return s.handleMessage(ctx, client, msg)
		}
		client.Start(gctx.New(), wrapper)
	}()
}
