# Learngo

## Goroutine and Channels
See folder `channel` and `Makefile` of how to run the program.

- Program will start http server on port 8001
- API: `POST /start` of request
body `{ "url":WEBSOCKET_URL, "count":NUMBER_OF_REQUEST }`.
- Websocket is a URL to a ASR (Automatic Speech Regconition) service.
- Request will read a given audio file and stream to the number of websocket connections `count`
- Request also read the transcribed response back while streaming (bidirectional)
- Result returns as a time consume, response time per speech,... See `Results` folder that write file. (Make sure to run program with `-w true` to allow write result file).