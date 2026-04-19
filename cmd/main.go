package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

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

const MAX_FILE_UPDATE_READ_RETRY = 5

func checkFileForUpdates(file *os.File, websocketWaitChannel chan bool, dataChannel chan<- string, initialSeekOffset int64) {
	fileUpdateReadRetryCount := 0
	for {
		fmt.Println("initial seek offset:", initialSeekOffset)
		if fileUpdateReadRetryCount >= MAX_FILE_UPDATE_READ_RETRY {
			fmt.Println("[File Update Checker Goroutine] Max file update read retry count reached, giving up")
			break
		}

		time.Sleep(5 * time.Second)
		// 1. check for updates to the file every 1 second
		latestFileSeekOffset, err := file.Seek(0, io.SeekEnd)
		if err != nil {
			fmt.Println("Error seeking to end of file:", err)
			break
		}
		offsetDiff := latestFileSeekOffset - initialSeekOffset
		if offsetDiff <= 0 {
			fmt.Println("No updates to the file")
			continue
		}
		_, err = file.Seek(0-offsetDiff, io.SeekCurrent)
		if err != nil {
			fmt.Println("Error seeking to diff start of file:", err)
			break
		}
		fmt.Println("File has been updated", offsetDiff, "bytes added")
		currentSeekOffset := int64(0)
		fileCharacterReadBuffer := make([]byte, 1)
		addedLines := make([]string, 0)
		currentLine := ""
		for {
			bytesRead, err := file.Read(fileCharacterReadBuffer)
			if err == io.EOF {
				fmt.Println("Reached end of file")
				addedLines = append(addedLines, currentLine)
				break
			}
			if err != nil {
				fmt.Println("[File Update Checker Goroutine] Error reading file:", err)
				fileUpdateReadRetryCount += 1
				break
			}
			if bytesRead == 0 {
				fmt.Println("Reached end of file")
				break
			}
			letter := string(fileCharacterReadBuffer)
			fmt.Println("Letter:", letter)
			if letter == "\n" {
				addedLines = append(addedLines, currentLine)
				fmt.Println("Added line:", currentLine)
				currentLine = ""
			} else {
				currentLine += letter
			}
			currentSeekOffset += 1
		}
		if len(addedLines) > 0 {
			fmt.Println("Added", len(addedLines), "lines to the data channel")
			fmt.Println("Lines:", addedLines)
			for _, line := range addedLines {
				dataChannel <- line
			}
		}
		// make the initial seek offset the current seek offset
		// so that the next time we check for updates, we start from the last (already) read position
		initialSeekOffset = initialSeekOffset + currentSeekOffset
	}
	websocketWaitChannel <- true
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
		initialSeekOffset, err := file.Seek(int64(currentOffset), io.SeekEnd)
		if err != nil {
			fmt.Println("Error1:", err)
			conn.WriteMessage(websocket.TextMessage, []byte("Error1: Could not read file contents"))
			return
		}

		// read the log file one byte at a time
		currentLine := ""
		currentLineCount := 0
		buffer := make([]byte, 1)

		for {
			if currentLineCount >= 10 {
				break
			}

			bytesRead, err := file.Read(buffer)
			if err != nil {
				fmt.Println("Error2:", err)
				conn.WriteMessage(websocket.TextMessage, []byte("Error2: Could not read file contents"))
				return
			}
			if bytesRead == 0 {
				fmt.Println("Nothing more to read from file")
				break
			}

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
		}
		websocketWaitChannel := make(chan bool)
		realtimeUpdateChannel := make(chan string)
		go checkFileForUpdates(file, websocketWaitChannel, realtimeUpdateChannel, initialSeekOffset+1)
		for {
			select {
			case newLine := <-realtimeUpdateChannel:
				conn.WriteMessage(websocket.TextMessage, []byte(newLine))
			case <-websocketWaitChannel:
				fmt.Println("Error with file reading operations. Closing websocket connection, exiting handler parent goroutine")
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
