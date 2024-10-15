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

type Client struct {
    conn *websocket.Conn
}

var clients = make(map[*Client]bool)
var broadcast = make(chan *Message)
var mutex = sync.Mutex{}

type Message struct {
    Text      string `json:"text"`
    Sender    string `json:"sender"`
    Timestamp string `json:"timestamp"`
}

// Function to handle WebSocket connections
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Println("Upgrade error:", err)
        return
    }

    client := &Client{conn: conn}
    mutex.Lock()
    clients[client] = true
    mutex.Unlock()

    defer func() {
        mutex.Lock()
        delete(clients, client)
        mutex.Unlock()
        log.Println("Closing connection")
        conn.Close()
    }()

    for {
        _, messageBytes, err := conn.ReadMessage()
        if err != nil {
            log.Println("Read error:", err)
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

// Function to broadcast messages to all clients
func handleMessages() {
    for {
        msg := <-broadcast
        messageJSON, err := json.Marshal(msg)
        if err != nil {
            log.Printf("Error marshalling message: %v", err)
            continue
        }

        mutex.Lock()
        for client := range clients {
            err := client.conn.WriteMessage(websocket.TextMessage, messageJSON)
            if err != nil {
                log.Printf("Write error: %v", err)
                client.conn.Close()
                delete(clients, client)
            }
        }
        mutex.Unlock()
    }
}

func main() {
    // Get the port from the environment (required by Heroku)
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080" // Default to 8080 if not running on Heroku
        fmt.Println("PORT environment variable not set, using default port 8080")
    }

    http.HandleFunc("/ws", handleWebSocket)

    // Run the message broadcasting handler
    go handleMessages()

    fmt.Println("Server started at :8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}
