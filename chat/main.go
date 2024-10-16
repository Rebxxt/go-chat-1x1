package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

const (
	port      = "9999"
	dbAddress = "postgresql://postgres:password@localhost:9980/postgres"
)

type ChatHandlers struct {
	pool *pgxpool.Pool
}

func NewChatHandlers(pool *pgxpool.Pool) *ChatHandlers {
	return &ChatHandlers{pool: pool}
}

type User struct {
	Username string `json:"username"`
}

func (c *ChatHandlers) GetUser(w http.ResponseWriter, r *http.Request) {
	fmt.Println()
	username := r.Header.Get("username")
	if username == "" {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	query, err := c.pool.Query(r.Context(), `select username from users where username = $1`, username)
	if err != nil {
		fmt.Println("query error")
		return
	}
	defer query.Close()

	ok := query.Next()
	if !ok {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	var user User
	err = query.Scan(&user.Username)
	if err != nil {
		fmt.Printf("error scan query result: %s\n", err)
		return
	}

	response, err := json.Marshal(user)
	if err != nil {
		return
	}
	_, err = w.Write(response)
	if err != nil {
		fmt.Printf("error write response: %s", err)
		return
	}
}

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
)

type ConnectionStore struct {
	store map[string]*websocket.Conn
	mu    sync.RWMutex
}

func NewConnectionStore() *ConnectionStore {
	return &ConnectionStore{
		store: make(map[string]*websocket.Conn),
	}
}

func (c *ConnectionStore) Set(username string, conn *websocket.Conn) {
	c.mu.Lock()
	c.store[username] = conn
	c.mu.Unlock()
}

func (c *ConnectionStore) Delete(username string) {
	c.mu.Lock()
	delete(c.store, username)
	c.mu.Unlock()
}

func (c *ConnectionStore) Get(username string) *websocket.Conn {
	c.mu.RLock()
	v := c.store[username]
	c.mu.RUnlock()
	return v
}

type ToWSMessage struct {
	To   string `json:"to"`
	Text string `json:"text"`
}

type FromWSMessage struct {
	From string `json:"from"`
	Text string `json:"text"`
}

const (
	ErrAlreadyConnection = "user already connected"
	ErrUpgrade           = "upgrade error"
)

func main() {
	fmt.Printf("starting server on %s\n", port)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbAddress)
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	handlers := NewChatHandlers(pool)

	connections := NewConnectionStore()

	mux.HandleFunc("GET /user", handlers.GetUser)
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		username := r.Header.Get("username")

		connInStore := connections.Get(username)
		if connInStore != nil {
			// what to do if many connections from one account
			log.Printf("user \"%s\" already has connection: %s\n", username, err)
			http.Error(w, ErrAlreadyConnection, http.StatusConflict)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("upgrade error: %s\n", err)
			w.Write([]byte(err.Error()))
			return
		}
		connections.Set(username, conn)

		go func() {
			defer connections.Delete(username)
			for {
				messageType, message, err := conn.ReadMessage()
				if err != nil && messageType == websocket.CloseMessage {
					fmt.Printf("close connection\n")
					return
				}
				if err != nil {
					fmt.Printf("error ReadMessage: %s\n", err)
					return
				}

				var fromMsg ToWSMessage
				err = json.Unmarshal(message, &fromMsg)
				if err != nil {
					fmt.Printf("error Unmarshal \"%s\": %s\n", message, err)
					continue
				}

				receiverConn := connections.Get(fromMsg.To)
				if receiverConn == nil {
					fmt.Printf("user \"%s\" is offline\n", fromMsg.To)
					continue
				}

				toMsg := &FromWSMessage{
					From: username,
					Text: fromMsg.Text,
				}
				msg, err := json.Marshal(toMsg)
				if err != nil {
					fmt.Printf("error Marshal: %s\n", err)
					continue
				}
				err = receiverConn.WriteMessage(1, msg)
				if err != nil {
					fmt.Printf("error on send message from \"%s\" to \"%s\": %s", username, fromMsg.To, err)
					continue
				}

				fmt.Printf("from \"%s\" to \"%s\": %s\n", username, fromMsg.To, fromMsg.Text)
			}
		}()
	})

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, os.Interrupt)

	go func() {
		err := http.ListenAndServe(fmt.Sprintf("localhost:%s", port), mux)
		if err != nil {
			panic(err)
		}
	}()

	for s := range sig {
		fmt.Println("SIG", s)
		pool.Close()
		return
	}
}
