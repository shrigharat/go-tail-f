package main

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
)

var websocketUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		fmt.Println("Request coming from:", r.Host)
		return true
	},
}

func reverse(r []rune) string {
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	reversedString := string(r)
	return reversedString
}

func checkFileForUpdates(fileName string) {
	// 1. check for updates to the file every 1 second
	// 2. let another goroutine know about the updates, maybe through a channel
}

func main() {
	fileServer := http.FileServer(http.Dir("static"))
	// frontend template to view logs
	http.Handle("/", fileServer)

	// websocket handler for realtime logs streaming
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Spawning a goroutine for websocket request")

		// upgrade normal http connection to websocket connection
		conn, err := websocketUpgrader.Upgrade(w, r, nil)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Connection upgrade error: Websocket is not supported on the requesting client"))
			return
		}
		defer conn.Close()

		// open the log file
		file, err := os.Open("tail.log")
		if err != nil {
			conn.WriteMessage(websocket.TextMessage, []byte("Error: Could not open file contents"))
		}
		defer file.Close()

		// start reading the log file from the end
		currentOffset := -1
		_, err = file.Seek(int64(currentOffset), io.SeekEnd)
		if err != nil {
			fmt.Println("Error1:", err)
			conn.WriteMessage(websocket.TextMessage, []byte("Error1: Could not read file contents"))
			return
		}

		// read the log file one byte at a time
		currentLine := ""
		currentLineCount := 0
		buffer := make([]byte, 1)
		bytesRead, err := file.Read(buffer)
		if err != nil {
			fmt.Println("Error2:", err)
			conn.WriteMessage(websocket.TextMessage, []byte("Error2: Could not read file contents"))
			return
		}
		for bytesRead != 0 && currentLineCount < 10 {
			letter := string(buffer)
			if letter != "\n" {
				currentLine += string(buffer)
			} else {
				currentLineRunes := []rune(currentLine)
				reversedLine := reverse(currentLineRunes)
				if reversedLine == "" {
					conn.WriteMessage(websocket.TextMessage, []byte("<empty line>"))
				} else {
					conn.WriteMessage(websocket.TextMessage, []byte(reversedLine))
				}
				currentLine = ""
				currentLineCount += 1
			}
			currentOffset -= 1
			_, err = file.Seek(int64(currentOffset), io.SeekEnd)
			if err != nil {
				fmt.Println("Reached end of file")
				currentLineRunes := []rune(currentLine)
				reversedLine := reverse(currentLineRunes)
				conn.WriteMessage(websocket.TextMessage, []byte(reversedLine))
				break
			}
			bytesRead, err = file.Read(buffer)
			if err != nil {
				fmt.Println("Error1:", err)
				conn.WriteMessage(websocket.TextMessage, []byte("Error2: Could not read file contents"))
				return
			}
		}
	})

	// endpoint to handle one time retrieval of last n logs
	http.HandleFunc("/logs", func(w http.ResponseWriter, r *http.Request) {
		tempResponse := []byte("Log line")
		w.Write(tempResponse)
	})

	http.ListenAndServe("127.0.0.1:8000", nil)
}
