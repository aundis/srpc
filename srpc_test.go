package srpc

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gmap"
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/gclient"
	"github.com/gogf/gf/v2/os/gctx"
	"github.com/gorilla/websocket"
)

var server = NewServer("main")
var clients = gmap.NewStrAnyMap(true)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var wg = sync.WaitGroup{}

func init() {
	wg.Add(5)

	ctx := gctx.New()

	// start server
	go func() {
		http.HandleFunc("/login", handler)
		http.ListenAndServe(":8080", nil)
	}()

	startClient(ctx, "client1", map[string]ControllerHandle{})
	startClient(ctx, "client2", map[string]ControllerHandle{})
	startClient(ctx, "client3", map[string]ControllerHandle{})
	startClient(ctx, "client4", map[string]ControllerHandle{})
	startClient(ctx, "client5", map[string]ControllerHandle{})
	wg.Wait()
}

func startClient(ctx context.Context, name string, controller map[string]ControllerHandle) {
	go func() {
		socket := gclient.NewWebSocket()
		conn, _, err := socket.Dial("ws://localhost:8080/login?name="+name, http.Header{})
		if err != nil {
			panic(err)
		}

		client := NewClient(name, conn)

		clients.Set(name, client)
		defer clients.Remove(name)

		client.SetController(controller)
		wg.Done()
		client.Start(ctx)
	}()
}

func request(ctx context.Context, name string, req RequestData) (*gjson.Json, error) {
	res, err := clients.Get(name).(*Client).Request(ctx, req)
	if err != nil {
		return nil, err
	}

	jsn, err := gjson.DecodeToJson(res)
	if err != nil {
		return nil, err
	}
	return jsn, nil
}

func handler(w http.ResponseWriter, r *http.Request) {

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	v, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		log.Println(err)
		return
	}

	name := v.Get("name")
	client := NewClient(name, conn)

	listens := v.Get("listens")
	if len(listens) > 0 {
		client.SetListens(strings.Split(listens, ","))
	}

	// conn.WriteMessage(websocket.TextMessage, []byte("hello ok!"))
	server.AddClient(client)
}

func getClinet(name string) *Client {
	return clients.Get(name).(*Client)
}

func TestCallSelf(t *testing.T) {
	ctx := gctx.New()

	getClinet("client1").SetController(map[string]ControllerHandle{})
	getClinet("client2").SetController(map[string]ControllerHandle{})

	getClinet("client1").SetController(map[string]ControllerHandle{
		"sub": func(ctx context.Context, data []byte) (interface{}, error) {
			jsn, err := gjson.DecodeToJson(data)
			if err != nil {
				return nil, err
			}

			a := jsn.Get("a").Int()
			b := jsn.Get("b").Int()
			return BasicValue(a - b), nil
		},
	})

	jsn, err := request(ctx, "client1", RequestData{
		Mark:   CallMark,
		Target: "client1",
		Action: "sub",
		Data:   []byte(`{"a": 10, "b": 20}`),
	})
	if err != nil {
		t.Error(err)
		return
	}
	v := jsn.Get("value").Int()
	if v != -10 {
		t.Errorf("10 - 20 except -10 but got %d", v)
	}
}

func TestCallOther(t *testing.T) {
	ctx := gctx.New()

	getClinet("client1").SetController(map[string]ControllerHandle{})
	getClinet("client2").SetController(map[string]ControllerHandle{})

	getClinet("client2").SetController(map[string]ControllerHandle{
		"sub": func(ctx context.Context, data []byte) (interface{}, error) {
			jsn, err := gjson.DecodeToJson(data)
			if err != nil {
				return nil, err
			}

			a := jsn.Get("a").Int()
			b := jsn.Get("b").Int()
			return BasicValue(a - b), nil
		},
	})

	jsn, err := request(ctx, "client1", RequestData{
		Mark:   CallMark,
		Target: "client2",
		Action: "sub",
		Data:   []byte(`{"a": 10, "b": 20}`),
	})
	if err != nil {
		t.Error(err)
		return
	}
	v := jsn.Get("value").Int()
	if v != -10 {
		t.Errorf("10 - 20 except -10 but got %d", v)
	}
}

func TestCallReturnErr(t *testing.T) {
	ctx := gctx.New()

	getClinet("client1").SetController(map[string]ControllerHandle{})
	getClinet("client2").SetController(map[string]ControllerHandle{})

	getClinet("client2").SetController(map[string]ControllerHandle{
		"hello": func(ctx context.Context, data []byte) (interface{}, error) {
			return nil, errors.New("error3")
		},
	})

	_, err := request(ctx, "client1", RequestData{
		Mark:   CallMark,
		Target: "client2",
		Action: "hello",
		Data:   []byte(`{}`),
	})
	if err == nil {
		t.Errorf("except error but go nil")
		return
	}
	if err.Error() != "error3" {
		t.Errorf("except error3 bug got %s", err.Error())
		return
	}
}

func TestCallServer(t *testing.T) {
	ctx := gctx.New()
	server.SetController(map[string]ControllerHandle{
		"getValue": func(ctx context.Context, data []byte) (interface{}, error) {
			return BasicValue(1000), nil
		},
	})

	jsn, err := request(ctx, "client1", RequestData{
		Mark:   CallMark,
		Target: "main",
		Action: "getValue",
		Data:   []byte(`{}`),
	})
	if err != nil {
		t.Error(err)
		return
	}
	v := jsn.Get("value").Int()
	if v != 1000 {
		t.Errorf("except 1000 but got %d", v)
	}
}

func TestClientEmit(t *testing.T) {
	ctx := gctx.New()

	counter := 0

	server.GetClient("client1").SetListens([]string{"client3@walk"})
	getClinet("client1").SetController(map[string]ControllerHandle{
		"walk": func(ctx context.Context, data []byte) (interface{}, error) {
			counter++
			return BasicValue(nil), nil
		},
	})

	server.GetClient("client2").SetListens([]string{"client3@walk"})
	getClinet("client2").SetController(map[string]ControllerHandle{
		"walk": func(ctx context.Context, data []byte) (interface{}, error) {
			counter++
			return BasicValue(nil), nil
		},
	})

	_, err := getClinet("client3").Request(ctx, RequestData{
		Mark:   EmitMark,
		Target: "main",
		Action: "walk",
		Data:   []byte(`{"a": 20}`),
	})
	if err != nil {
		t.Error(err)
	}
	if counter != 2 {
		t.Errorf("except counter 2 but got %d", counter)
	}
}

func TestServerEmit(t *testing.T) {
	ctx := gctx.New()

	counter := 0

	server.GetClient("client1").SetListens([]string{"client3@walk"})
	getClinet("client1").SetController(map[string]ControllerHandle{
		"walk": func(ctx context.Context, data []byte) (interface{}, error) {
			counter++
			return BasicValue(nil), nil
		},
	})

	server.GetClient("client2").SetListens([]string{"client3@walk"})
	getClinet("client2").SetController(map[string]ControllerHandle{
		"walk": func(ctx context.Context, data []byte) (interface{}, error) {
			counter++
			return BasicValue(nil), nil
		},
	})

	_, err := getClinet("client3").Request(ctx, RequestData{
		Mark:   EmitMark,
		Target: "main",
		Action: "walk",
		Data:   []byte(`{"a": 20}`),
	})
	if err != nil {
		t.Error(err)
	}
	if counter != 2 {
		t.Errorf("except counter 2 but got %d", counter)
	}
}

func TestQps(t *testing.T) {
	// 注释掉这条代码以开启测试
	return

	ctx := gctx.New()

	getClinet("client3").SetController(map[string]ControllerHandle{})
	getClinet("client4").SetController(map[string]ControllerHandle{})

	getClinet("client3").SetController(map[string]ControllerHandle{
		"sub": func(ctx context.Context, data []byte) (interface{}, error) {
			jsn, err := gjson.DecodeToJson(data)
			if err != nil {
				return nil, err
			}

			a := jsn.Get("a").Int()
			b := jsn.Get("b").Int()
			return g.Map{
				"value": a - b,
			}, nil
		},
	})

	getClinet("client4").SetController(map[string]ControllerHandle{
		"add": func(ctx context.Context, data []byte) (interface{}, error) {
			jsn, err := gjson.DecodeToJson(data)
			if err != nil {
				return nil, err
			}

			a := jsn.Get("a").Int()
			b := jsn.Get("b").Int()
			return g.Map{
				"value": a + b,
			}, nil
		},
	})

	counter := 0
	go func() {
		for {
			<-time.After(time.Second)
			t.Log(counter)
			counter = 0
		}
	}()

	_, err := request(ctx, "client3", RequestData{
		Mark:   CallMark,
		Target: "client4",
		Action: "add",
		Data:   []byte(`{"a": 10, "b": 20}`),
	})
	if err != nil {
		t.Error(err)
		return
	}

	for i := 0; i < 10; i++ {
		go func() {
			for {
				_, err := request(ctx, "client3", RequestData{
					Mark:   CallMark,
					Target: "client4",
					Action: "add",
					Data:   []byte(`{"a": 10, "b": 20}`),
				})
				if err != nil {
					t.Error(err)
					return
				}
				counter++
			}
		}()
	}

	<-time.After(10 * time.Second)
}
