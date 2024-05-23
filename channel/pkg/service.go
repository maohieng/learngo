package pkg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

type Response struct {
	Partial string `json:"partial"`
}

type Result struct {
	EndTime     float64
	NumbMicDrop int
	AvgDelay    float64
	Err         string
}

const (
	// channelBufSize is crucial for how asynchronous speech of the user!!!!!
	channelBufSize = 100
	readBufSize    = 1024
)

type Service interface {
	HandleSend(ctx context.Context, url string, count int) ([]Result, error)
}

func NewService(audioFileName string, writeResultFile bool) Service {
	return &service{
		writeResultFile: writeResultFile,
		audioFileName:   audioFileName,
	}
}

type service struct {
	writeResultFile bool
	audioFileName   string
}

func streamWAVFile(ctx context.Context, conn *websocket.Conn, dataCh <-chan []byte, writef *os.File) (Result, error) {
	start := time.Now()

	var lastPartialL, numbDrop int
	var micDroppedTime, lastResTime, totalDelay float64

	for {
		select {
		case <-ctx.Done():
			// The context has been cancelled
			avg := totalDelay / float64(numbDrop)

			if writef != nil {
				writef.WriteString(fmt.Sprintf("%s, %f\n", "[--user cancelled--]", time.Since(start).Seconds()))
				writef.WriteString("numb_mic_drop, resp_time, avg_delay\n")
				writef.WriteString(fmt.Sprintf("%d, %f, %f\n", numbDrop, lastResTime, avg))
			}

			return Result{
				EndTime:     lastResTime,
				NumbMicDrop: numbDrop,
				AvgDelay:    avg,
			}, ctx.Err()
		case data, ok := <-dataCh:
			if !ok {
				// The channel has been closed
				avg := totalDelay / float64(numbDrop)

				if writef != nil {
					writef.WriteString(fmt.Sprintf("%s, %f\n", "[--Done streaming--]", time.Since(start).Seconds()))
					writef.WriteString("numb_mic_drop, resp_time, avg_delay\n")
					writef.WriteString(fmt.Sprintf("%d, %f, %f\n", numbDrop, lastResTime, avg))
				}

				return Result{
					EndTime:     lastResTime,
					NumbMicDrop: numbDrop,
					AvgDelay:    avg,
				}, nil
			}

			if micDroppedTime == 0 {
				micDroppedTime = time.Since(start).Seconds()
				if writef != nil {
					writef.WriteString(fmt.Sprintf("%s, %f\n", "[--drop the mic--]", micDroppedTime))
				}
			}

			err := conn.WriteMessage(websocket.BinaryMessage, data)
			if err != nil {
				return Result{}, err
			}

			// Read the response
			_, message, err := conn.ReadMessage()
			if err != nil {
				return Result{}, err
			}

			var response Response
			err = json.Unmarshal(message, &response)
			if err != nil {
				return Result{}, err
			}

			if len := len(response.Partial); response.Partial != "" && len != lastPartialL {
				lastPartialL = len
				lastResTime = time.Since(start).Seconds()
				delay := lastResTime - micDroppedTime

				if writef != nil {
					writef.WriteString(fmt.Sprintf("%s, %f, %f\n", response.Partial, lastResTime, delay))
				}

				micDroppedTime = 0
				numbDrop++
				totalDelay += delay
			}
		}
	}
}

func (s *service) runWebSocketClient(ctx context.Context, conn *websocket.Conn, dataCh <-chan []byte, index int) (Result, error) {
	fileIndex := index + 1
	log.Printf("Stream started for %d", fileIndex)
	// Open file to write Results and errors
	var writef *os.File
	var errorF string
	if s.writeResultFile {
		ResultF := fmt.Sprintf("./Results/response_%d.txt", fileIndex)
		errorF = fmt.Sprintf("./errors/error_%d.txt", fileIndex)

		writef, _ = os.OpenFile(ResultF, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	}

	defer func() {
		if writef != nil {
			writef.Close()
		}
	}()

	res, err := streamWAVFile(ctx, conn, dataCh, writef)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Println("Error streaming WAV file", fileIndex, "err", err)
		if errorF != "" {
			// write error to file
			os.WriteFile(errorF, []byte(err.Error()), 0644)
		}
	}

	log.Printf("Streaming complete for %d", fileIndex)

	return res, err
}

func (s *service) HandleSend(ctx context.Context, url string, count int) ([]Result, error) {
	if s.writeResultFile {
		// Remove all files in the Results and errors directory
		os.RemoveAll("./Results")
		if _, err := os.Stat("./errors"); os.IsNotExist(err) {
			os.Mkdir("./errors", 0755)
		}

		os.RemoveAll("./errors")
		if _, err := os.Stat("./Results"); os.IsNotExist(err) {
			os.Mkdir("./Results", 0755)
		}
	}

	// Open audio file
	file, err := os.Open(fmt.Sprintf("./%s", s.audioFileName))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Create array of channels
	handshakeChans := make([]chan *websocket.Conn, count)
	for i := 0; i < count; i++ {
		handshakeChans[i] = make(chan *websocket.Conn, 4)
		go func(i int, url string) {
			conn, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				log.Print("WebSocket connection error", err)
				handshakeChans[i] <- nil
				return
			}
			handshakeChans[i] <- conn
		}(i, url)
	}

	// Wait for all connections to be ready
	var hsCons []*websocket.Conn
	for i := 0; i < count; i++ {
		if conn := <-handshakeChans[i]; conn != nil {
			hsCons = append(hsCons, conn)
		}
	}

	numbHandshake := len(hsCons)

	if numbHandshake == 0 {
		return nil, errors.New("no connection is ready to stream")
	}

	log.Printf("Connections %d are ready to stream.", numbHandshake)

	// Create channels for writing data and receive Results from workers
	dataChans := make([]chan []byte, numbHandshake)
	ResultChans := make([]chan Result, numbHandshake)

	for i := 0; i < numbHandshake; i++ {
		dataChans[i] = make(chan []byte, channelBufSize)
		ResultChans[i] = make(chan Result, 1)
	}

	// Start all workers
	for i := 0; i < numbHandshake; i++ {
		conn := hsCons[i]
		go func(i int) {
			res, err := s.runWebSocketClient(ctx, conn, dataChans[i], i)
			if err != nil {
				ResultChans[i] <- Result{Err: err.Error()}
			} else {
				ResultChans[i] <- res
			}
		}(i)
	}

	go func() {
		defer func() {
			// Close the channel to signal that no more data will be sent
			for _, ch := range dataChans {
				close(ch)
			}
		}()

		buf := make([]byte, readBufSize)
	outerLoop:
		for {
			select {
			case <-ctx.Done():
				log.Println("Context is done. Stop writing data.")
				break outerLoop
			default:
				n, err := file.Read(buf)
				if err != nil {
					if err != io.EOF {
						log.Printf("Failed to read file: %v", err)
					}

					break outerLoop
				}
				if n == 0 {
					break
				}

				// Create a copy of the buffer slice with the correct length
				data := make([]byte, n)
				copy(data, buf[:n])

				// Send data to all worker channels
				for _, ch := range dataChans {
					select {
					case ch <- data:
					case <-ctx.Done():
						log.Println("Context is done. Stop writing data.")
						break outerLoop
					}
				}
			}
		}
	}()

	// Write data to all worker channels
	log.Printf("Read audio data and send to all worker's channels.")

	// Read Result data from all workers
	log.Printf("Waiting for %d workers to finish.", numbHandshake)
	Results := make([]Result, numbHandshake)
	for i, ch := range ResultChans {
		Results[i] = <-ch
		close(ch)
	}

	return Results, nil
}
