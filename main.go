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
    Type        string `json:"type"`
    Text        string `json:"text"`
    Sender      string `json:"sender"`
    Timestamp   string `json:"timestamp"`
}

// Function to handle WebSocket connections
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Println("Upgrade error:", err)
        return
    }

    // Set a pong handler to handle ping-pong keepalive
    conn.SetPongHandler(func(appData string) error {
        log.Println("Pong received from client")
        return nil
    })

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

        // Ignore ping messages
        if message.Type == "ping" {
            // Optionally send a Pong message back to the client if needed
            if err := conn.WriteMessage(websocket.PongMessage, nil); err != nil {
                log.Println("Error sending pong message:", err)
            }
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
        port = "8090" // Default to 8080 for local testing
        fmt.Println("PORT environment variable not set, using default port 8090")
    }

    fmt.Printf("Server started on port %s\n", port) // Corrected log message

    http.HandleFunc("/ws", handleWebSocket)

    // Run the message broadcasting handler
    go handleMessages()

    log.Fatal(http.ListenAndServe(":"+port, nil)) // Correctly bind to the port
}
