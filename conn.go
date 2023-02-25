package srpc

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"

	"github.com/gogf/gf/v2/os/gmutex"
	"github.com/gorilla/websocket"
)

type Socket interface {
	io.Closer
	ReadMessage() (messageType int, p []byte, err error)
	WriteJSON(v interface{}) error
	WriteMessage(messageType int, data []byte) error
}

type Conn struct {
	socket Socket
	mutex  *gmutex.Mutex
}

func NewConn(socket Socket) *Conn {
	return &Conn{
		socket: socket,
		mutex:  gmutex.New(),
	}
}

func (c *Conn) WriteBinMessage(mark string, source string, id int64, action string, data []byte) error {
	var buffer bytes.Buffer
	buffer.WriteString(mark)
	buffer.WriteByte(',')
	buffer.WriteString(source)
	buffer.WriteByte(',')
	buffer.WriteString(strconv.FormatInt(id, 10))
	buffer.WriteByte(',')
	buffer.WriteString(action)
	buffer.Write(data)

	return c.WriteData(websocket.TextMessage, buffer.Bytes())
}

func (c *Conn) WriteJsonMessage(mark string, source string, id int64, action string, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.WriteBinMessage(mark, source, id, action, data)
}

func (c *Conn) WriteData(tpe int, data []byte) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.socket.WriteMessage(tpe, data)
}

func (c *Conn) ReadMessage() (messageType int, p []byte, err error) {
	return c.socket.ReadMessage()
}
