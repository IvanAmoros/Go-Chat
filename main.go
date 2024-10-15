package main

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "sync"
    "time"

    "github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        return true
    },
}

// Client represents a connected WebSocket client
type Client struct {
    conn *websocket.Conn
    send chan *Message
}

var (
    clients   = make(map[*Client]bool)
    broadcast = make(chan *Message)
    mutex     = sync.Mutex{}
)

// Message represents the structure of messages exchanged
type Message struct {
    Type      string `json:"type"`
    Text      string `json:"text"`
    Sender    string `json:"sender"`
    Timestamp string `json:"timestamp"`
}

// Timeouts and limits
const (
    writeWait      = 10 * time.Second
    pongWait       = 60 * time.Second
    pingPeriod     = (pongWait * 9) / 10 // Send pings at 90% of the pongWait
    maxMessageSize = 512
)

// handleWebSocket handles incoming WebSocket connections
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Println("Upgrade error:", err)
        return
    }

    client := &Client{
        conn: conn,
        send: make(chan *Message),
    }

    mutex.Lock()
    clients[client] = true
    mutex.Unlock()

    // Start goroutines for reading from and writing to the client
    go client.readPump()
    go client.writePump()
}

// readPump pumps messages from the WebSocket connection to the broadcast channel
func (c *Client) readPump() {
    defer func() {
        mutex.Lock()
        delete(clients, c)
        mutex.Unlock()
        c.conn.Close()
        log.Println("Connection closed for client")
    }()

    c.conn.SetReadLimit(maxMessageSize)
    c.conn.SetReadDeadline(time.Now().Add(pongWait))
    c.conn.SetPongHandler(func(string) error {
        c.conn.SetReadDeadline(time.Now().Add(pongWait))
        return nil
    })

    for {
        _, messageBytes, err := c.conn.ReadMessage()
        if err != nil {
            if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
                log.Printf("Read error: %v", err)
            }
            break
        }

        // Parse the message from the client
        var message Message
        err = json.Unmarshal(messageBytes, &message)
        if err != nil {
            log.Println("Error parsing message:", err)
            continue
        }

        // Add timestamp to the message
        message.Timestamp = time.Now().Format("2006-01-02 15:04:05")

        // Send the message to the broadcast channel
        broadcast <- &message
    }
}

// writePump pumps messages from the broadcast channel to the WebSocket connection
func (c *Client) writePump() {
    ticker := time.NewTicker(pingPeriod)
    defer func() {
        ticker.Stop()
        c.conn.Close()
    }()

    for {
        select {
        case message, ok := <-c.send:
            c.conn.SetWriteDeadline(time.Now().Add(writeWait))
            if !ok {
                // The channel was closed.
                c.conn.WriteMessage(websocket.CloseMessage, []byte{})
                return
            }

            // Send the message to the client
            if err := c.conn.WriteJSON(message); err != nil {
                log.Println("Write error:", err)
                return
            }

        case <-ticker.C:
            // Send a ping to the client
            c.conn.SetWriteDeadline(time.Now().Add(writeWait))
            if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
                log.Println("Ping error:", err)
                return
            }
        }
    }
}

// handleMessages broadcasts messages to all connected clients
func handleMessages() {
    for {
        msg := <-broadcast

        mutex.Lock()
        for client := range clients {
            select {
            case client.send <- msg:
            default:
                close(client.send)
                delete(clients, client)
            }
        }
        mutex.Unlock()
    }
}

func main() {
    port := os.Getenv("PORT")
    if port == "" {
        port = "8090"
        fmt.Println("PORT environment variable not set, using default port 8090")
    }

    fmt.Printf("Server started on port %s\n", port)

    http.HandleFunc("/ws/", handleWebSocket)

    go handleMessages()

    log.Fatal(http.ListenAndServe(":"+port, nil))
}
