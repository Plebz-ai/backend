package ws

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func StartWebSocketServer(port string) {
	http.HandleFunc("/ws/chat", chatHandler)
	log.Printf("WebSocket server listening on %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("failed to serve websocket: %v", err)
	}
}

func chatHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()
	// TODO: Implement chat streaming logic
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("WebSocket read error: %v", err)
			break
		}
		log.Printf("Received message: %s", msg)
		// Echo back for now
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			log.Printf("WebSocket write error: %v", err)
			break
		}
	}
} 