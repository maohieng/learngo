package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/maohieng/learngo/channel/pkg"
	"github.com/oklog/ulid"
)

var (
	fs              = flag.NewFlagSet("bench", flag.ExitOnError)
	audioFileName   = fs.String("f", "19s.wav", "Input audio file name")
	writeResultFile = fs.Bool("w", true, "Write results to file")
	port            = fs.String("p", "8001", "Port number")
)

// main function
// Example: go run main.go -w true -f ./../19s.wav
func main() {
	fs.Parse(os.Args[1:])

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	svc := pkg.NewService(*audioFileName, *writeResultFile)

	router.POST("/start", func(c *gin.Context) {
		var requestBody struct {
			Count int    `json:"count"`
			URL   string `json:"url"`
		}

		if err := c.ShouldBindJSON(&requestBody); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Call the function to run concurrent WebSocket clients here using requestBody.Count and requestBody.URL
		count := requestBody.Count
		if count <= 0 {
			count = 1
		}

		url := requestBody.URL
		if url == "" {
			url = "ws://localhost:8001/ws"
		}

		started := time.Now()

		results, err := svc.HandleSend(ctx, url, count)

		ended := time.Since(started).Seconds()

		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(), "time": ended,
			})
			return
		}

		err = writeCSV(c.Writer, count, results)
		if err != nil {
			log.Println("Error writing CSV", "err", err)
		}
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", *port),
		Handler: router.Handler(),
	}

	log.Printf("Server started at %s", srv.Addr)
	go func() {
		// service connections
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with
	// a timeout of 5 seconds.
	quit := make(chan os.Signal, 1)
	// kill (no param) default send syscall.SIGTERM
	// kill -2 is syscall.SIGINT
	// kill -9 is syscall. SIGKILL but can"t be catch, so don't need add it
	signal.Notify(quit, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutdown server. Please wait...")

	// Cancel the operation
	cancel()

	sdCtx, sdCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer sdCancel()

	if err := srv.Shutdown(sdCtx); err != nil {
		log.Fatal("Server Shutdown:", err)
	}

	// catching sdCtx.Done(). timeout of 5 seconds.
	<-sdCtx.Done()
	log.Println("Server exiting")

}

func writeCSV(w gin.ResponseWriter, count int, data []pkg.Result) error {
	// Create a file
	fileid := "bench_result"

	entropy := rand.New(rand.NewSource(time.Now().UnixNano()))
	ms := ulid.Timestamp(time.Now())
	id, err := ulid.New(ms, entropy)
	if err == nil {
		fileid = id.String()
	}

	filename := fmt.Sprintf("%s.csv", fileid)
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed creating file: %s", err)
	}
	defer file.Close()

	// Create a CSV writer for the file
	fileWriter := csv.NewWriter(file)

	// Create a CSV writer for the http.ResponseWriter
	respWriter := csv.NewWriter(w)

	// Write the CSV data to both writers
	respWriter.Write([]string{"total_req", "resp_time", "avg_delay", "err"})
	fileWriter.Write([]string{"total_req", "resp_time", "avg_delay", "err"})
	for _, res := range data {
		respWriter.Write([]string{
			fmt.Sprintf("%d", count),
			fmt.Sprintf("%f", res.EndTime),
			fmt.Sprintf("%f", res.AvgDelay),
			res.Err,
		})

		fileWriter.Write([]string{
			fmt.Sprintf("%d", count),
			fmt.Sprintf("%f", res.EndTime),
			fmt.Sprintf("%f", res.AvgDelay),
			res.Err,
		})
	}

	// Flush both writers to ensure all data is written
	respWriter.Flush()
	fileWriter.Flush()

	if err := fileWriter.Error(); err != nil {
		return fmt.Errorf("error with file writer: %s", err)
	}
	if err := respWriter.Error(); err != nil {
		return fmt.Errorf("error with gin.ResponseWriter writer: %s", err)
	}

	return nil
}
