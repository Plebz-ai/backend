package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/gorilla/websocket"
)

const (
	baseURL     = "http://localhost:8081"
	wsURL       = "ws://localhost:8081/ws"
	userAPIKey  = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjozLCJlbWFpbCI6InNhdWhhcmQ3NGJlc3RAZ21haWwuY29tIiwiZXhwIjoxNzQ1NTQwNDA0LCJuYmYiOjE3NDU0NTQwMDQsImlhdCI6MTc0NTQ1NDAwNH0.udehAy86F8mSWFbbzwbcBEvdjoeQ8LyYXDTUgfqHSAI" // Replace with actual token
	mlAPIKey    = "ml-api-key-12345"
	clientID    = "audio-listener"
	sessionID   = "test-session-harvard"
	characterID = "1"
	audioPath   = "../frontend/public/harvard.wav"
	outputDir   = "./audio_samples"
)

type WSMessage struct {
	Type    string      `json:"type"`
	Content interface{} `json:"content"`
}

func main() {
	// Define command line flags
	testAudioFilePtr := flag.Bool("test-file", false, "Test with harvard.wav file")
	listenPtr := flag.Bool("listen", false, "Start audio listener for WebSocket")
	helpPtr := flag.Bool("help", false, "Show usage information")

	// Parse command line arguments
	flag.Parse()

	// Show help if requested or if no arguments provided
	if *helpPtr || (!*testAudioFilePtr && !*listenPtr) {
		fmt.Println("Audio Tools Usage:")
		fmt.Println("  -test-file    Test audio API with the harvard.wav file")
		fmt.Println("  -listen       Start WebSocket listener for audio from video calls")
		fmt.Println("  -help         Show this help message")
		os.Exit(0)
	}

	// Handle the test-file command
	if *testAudioFilePtr {
		fmt.Println("Testing with harvard.wav file...")
		testWithHarvardFile()
	}

	// Handle the listen command
	if *listenPtr {
		fmt.Println("Starting audio listener...")
		runAudioListener()
	}
}

// testWithHarvardFile runs the test audio functionality
func testWithHarvardFile() {
	// First upload the file
	uploadResp, err := uploadFile()
	if err != nil {
		fmt.Printf("Error uploading file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("File uploaded successfully: %+v\n", uploadResp)

	// Get the audio ID from the response
	audioID := uploadResp["id"].(string)

	// Retrieve the audio chunk using ML API
	chunk, err := getAudioChunk(audioID)
	if err != nil {
		fmt.Printf("Error retrieving audio chunk: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Audio chunk retrieved successfully:\n")
	fmt.Printf("ID: %s\n", chunk["id"])
	fmt.Printf("Format: %s\n", chunk["format"])
	fmt.Printf("Processing Status: %s\n", chunk["processingStatus"])
	fmt.Printf("Size of audio data: %d bytes\n", len(chunk["audioData"].([]byte)))

	// Update status to simulate processing
	err = updateStatus(audioID, "processing")
	if err != nil {
		fmt.Printf("Error updating status: %v\n", err)
	} else {
		fmt.Println("Updated status to 'processing'")
	}

	// Update to completed
	err = updateStatus(audioID, "completed")
	if err != nil {
		fmt.Printf("Error updating status: %v\n", err)
	} else {
		fmt.Println("Updated status to 'completed'")
	}

	fmt.Println("Test completed successfully!")
}

func uploadFile() (map[string]interface{}, error) {
	file, err := os.Open(audioPath)
	if err != nil {
		return nil, fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add file
	part, err := writer.CreateFormFile("audioFile", filepath.Base(audioPath))
	if err != nil {
		return nil, fmt.Errorf("error creating form file: %w", err)
	}
	_, err = io.Copy(part, file)
	if err != nil {
		return nil, fmt.Errorf("error copying file: %w", err)
	}

	// Add other form fields
	writer.WriteField("sessionId", sessionID)
	writer.WriteField("charId", characterID)
	writer.WriteField("format", "wav")
	writer.WriteField("sampleRate", "44100")
	writer.WriteField("channels", "1")
	writer.WriteField("ttl", "1h")

	err = writer.Close()
	if err != nil {
		return nil, fmt.Errorf("error closing writer: %w", err)
	}

	req, err := http.NewRequest("POST", baseURL+"/api/audio/upload", body)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+userAPIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error response: %s, status: %d", string(bodyBytes), resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return result, nil
}

func getAudioChunk(id string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", baseURL+"/api/ml/audio/chunk/"+id, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("X-ML-API-Key", mlAPIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error response: %s, status: %d", string(bodyBytes), resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return result, nil
}

func updateStatus(id string, status string) error {
	statusUpdate := map[string]string{
		"status": status,
	}

	jsonData, err := json.Marshal(statusUpdate)
	if err != nil {
		return fmt.Errorf("error marshaling status: %w", err)
	}

	req, err := http.NewRequest("PUT", baseURL+"/api/ml/audio/chunk/"+id+"/status", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ML-API-Key", mlAPIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error response: %s, status: %d", string(bodyBytes), resp.StatusCode)
	}

	return nil
}

func runAudioListener() {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Connect to WebSocket
	log.Println("Connecting to WebSocket...")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL+"?characterId="+characterID+"&clientId="+clientID+"&sessionId="+sessionID, nil)
	if err != nil {
		log.Fatalf("Error connecting to WebSocket: %v", err)
	}
	defer conn.Close()
	log.Println("Connected to WebSocket")

	// Set up signal handling for graceful shutdown
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	// Track audio chunks for this session
	audioChunks := make(map[string]time.Time)

	// Message handling loop
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("WebSocket read error: %v", err)
				return
			}

			var wsMessage WSMessage
			if err := json.Unmarshal(message, &wsMessage); err != nil {
				log.Printf("Error unmarshaling message: %v", err)
				continue
			}

			// Process different message types
			switch wsMessage.Type {
			case "audio":
				handleAudioMessage(wsMessage, audioChunks)
			case "chat":
				log.Printf("Received chat message: %+v", wsMessage.Content)
			case "speech_text":
				log.Printf("Received speech transcription: %+v", wsMessage.Content)
			}
		}
	}()

	// Send ping messages to keep connection alive
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Println("Audio listener is running. Press Ctrl+C to exit...")
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			// Send ping message
			err := conn.WriteJSON(WSMessage{
				Type: "ping",
			})
			if err != nil {
				log.Printf("Error writing ping: %v", err)
				return
			}
		case <-interrupt:
			log.Println("Interrupt received, shutting down...")

			// Close WebSocket connection gracefully
			err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Printf("Error during closing websocket: %v", err)
			}

			// Wait for server to close the connection
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
	}
}

func handleAudioMessage(message WSMessage, audioChunks map[string]time.Time) {
	// Extract content as map
	contentMap, ok := message.Content.(map[string]interface{})
	if !ok {
		log.Printf("Error converting audio content to map")
		return
	}

	// Extract audio data
	audioDataRaw, ok := contentMap["data"]
	if !ok {
		log.Printf("No audio data in message")
		return
	}

	// Extract message ID
	messageID, ok := contentMap["messageId"].(string)
	if !ok {
		log.Printf("No message ID in audio message")
		messageID = fmt.Sprintf("audio-%d", time.Now().UnixNano())
	}

	log.Printf("Received audio data for message ID: %s", messageID)

	// Convert audio data to bytes
	var audioData []byte
	switch data := audioDataRaw.(type) {
	case []interface{}:
		// Convert from array of numbers to bytes
		audioData = make([]byte, len(data))
		for i, v := range data {
			if num, ok := v.(float64); ok {
				audioData[i] = byte(num)
			}
		}
	case string:
		// Handle base64 encoded string
		var err error
		audioData, err = base64.StdEncoding.DecodeString(data)
		if err != nil {
			log.Printf("Error decoding base64 audio data: %v", err)
			return
		}
	default:
		log.Printf("Unexpected audio data format: %T", audioDataRaw)
		return
	}

	// Save audio data to file
	filename := fmt.Sprintf("%s/%s.webm", outputDir, messageID)
	if err := os.WriteFile(filename, audioData, 0644); err != nil {
		log.Printf("Error saving audio file: %v", err)
		return
	}
	log.Printf("Saved audio file: %s", filename)

	// Store audio chunk for ML processing
	if err := uploadAudioChunk(audioData, messageID); err != nil {
		log.Printf("Error uploading audio chunk: %v", err)
		return
	}

	// Track this chunk
	audioChunks[messageID] = time.Now()
}

func uploadAudioChunk(audioData []byte, messageID string) error {
	// Create multipart request
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add audio data
	part, err := writer.CreateFormFile("audioFile", messageID+".webm")
	if err != nil {
		return fmt.Errorf("error creating form file: %w", err)
	}
	if _, err := part.Write(audioData); err != nil {
		return fmt.Errorf("error writing audio data: %w", err)
	}

	// Add other form fields
	writer.WriteField("sessionId", sessionID)
	writer.WriteField("charId", characterID)
	writer.WriteField("format", "webm")
	writer.WriteField("sampleRate", "48000")
	writer.WriteField("channels", "1")
	writer.WriteField("ttl", "1h")
	writer.WriteField("metadata", fmt.Sprintf(`{"source":"video_call","messageId":"%s"}`, messageID))

	if err := writer.Close(); err != nil {
		return fmt.Errorf("error closing writer: %w", err)
	}

	// Create request
	req, err := http.NewRequest("POST", baseURL+"/api/audio/upload", body)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+userAPIKey)

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error response: %s, status: %d", string(bodyBytes), resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("error decoding response: %w", err)
	}

	log.Printf("Audio chunk uploaded with ID: %s, size: %v bytes", result["id"], len(audioData))
	return nil
}
