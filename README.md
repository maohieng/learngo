# Learngo


## Goroutine and Channels
See [Makefile](Makefile) and `channel/main.go` of how the program run:

- Program will start http server on port 8001
- API: `POST /start` of request
body `{ "url":WEBSOCKET_URL, "count":NUMBER_OF_REQUEST }`.
- `WEBSOCKET_URL` is a URL to an ASR (Automatic Speech Regconition) service.
- `NUMBER_OF_REQUEST` number to start handshaking with ASR service concurrently.
- Request will read a given audio file and stream to the number of websocket connections `count`
- Request also read the transcribed response back while streaming (bidirectional)
- Result returns as a time consume, response time per speech,... See `Results` folder that program write result to. (Make sure to run program with `-w true` to allow write result file).