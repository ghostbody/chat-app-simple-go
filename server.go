package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	uuid "github.com/satori/go.uuid"
	"net/http"
)

type Message struct {
	Sender   string `json:"sender,omitempty"`
	Receiver string `json:"receiver,omitempty"`
	Content  string `json:"content,omitempty"`
}

type Client struct {
	Id     string
	Socket *websocket.Conn
	Send   chan []byte
}

type ClientManager struct {
	Clients    map[*Client]bool
	Broadcast  chan []byte
	Register   chan *Client
	Unregister chan *Client
}

var manager = ClientManager{
	Clients:    make(map[*Client]bool),
	Broadcast:  make(chan []byte),
	Register:   make(chan *Client),
	Unregister: make(chan *Client),
}

func (c *Client) Read() {
	defer func() {
		manager.Unregister <- c
		c.Socket.Close()
	}()
	for {
		_, message, err := c.Socket.ReadMessage()
		if err != nil {
			break
		}
		jsonMessage, _ := json.Marshal(&Message{Sender: c.Id, Content: string(message)})
		manager.Broadcast <- jsonMessage
	}
}

func (c *Client) Write() {
	defer func() {
		c.Socket.Close()
	}()
	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				c.Socket.WriteMessage(websocket.CloseMessage, []byte{})
				break
			}
			c.Socket.WriteMessage(websocket.TextMessage, message)
		}
	}
}

func (manager *ClientManager) Start() {
	for {
		select {
		case conn := <-manager.Register:
			manager.Clients[conn] = true
			jsonMessage, _ := json.Marshal(&Message{Content: "A new connection has been established"})
			manager.Send(jsonMessage, conn)
		case conn := <-manager.Unregister:
			if _, ok := manager.Clients[conn]; ok {
				// close the channel
				close(conn.Send)
				delete(manager.Clients, conn)
				jsonMessage, _ := json.Marshal(&Message{Content: fmt.Sprintf("connection %s disconnected", conn.Id)})
				manager.Send(jsonMessage, conn)
			}
		case message := <-manager.Broadcast:
			for conn := range manager.Clients {
				select {
				case conn.Send <- message:
				default:
					close(conn.Send)
					delete(manager.Clients, conn)
				}
			}
		}
	}
}

func (manager *ClientManager) Send(message []byte, sourceClient *Client) {
	for conn := range manager.Clients {
		if conn != sourceClient {
			conn.Send <- message
		}
	}
}

func wsPage(res http.ResponseWriter, req *http.Request) {
	conn, err := (&websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}).Upgrade(res, req, nil)
	if err != nil {
		http.NotFound(res, req)
		return
	}
	client := &Client{Id: uuid.NewV4().String(), Socket: conn, Send: make(chan []byte)}

	manager.Register <- client

	go client.Read()
	go client.Write()
}

func main() {
	fmt.Println("start application...")
	go manager.Start()
	http.HandleFunc("/ws", wsPage)
	http.ListenAndServe(":12345", nil)
}
