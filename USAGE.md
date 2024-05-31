# ASR Benchmark Tool - INTERNAL USE ONLY!
This tool will start a server running on a port (`-p`),
to stream an audio file (`-f`) to an ASR server, then writes result to a folder `Results`.

## Usage (Windows64)
1. Run the tool
```bash
./channel.exe -f 19s.wav
```
This will run a service on port `8001`.

- To change default port, add `-p PORT`. 
- To change audio file `-f AUDIO_FILE`.

See `./channel.exe --help` for detail.

2. Start streaming by calling API `POST /start` with request body:
```json
{
  "count": {{numb_request}},
  "url": {{ws_url}}
}
```
- `url` to ASR's websocket URL 
- `count` number of calls in paralell. 

Example:
```
POST http://localhost:8001/start
Content-Type: application/json

{
  "count": {{numb_request}},
  "url": {{ws_url}}
}
```

- Detail will be written into files in directory name `Results`.
- Summarize benchmark data will be returned after all process are done.
- The duration of API call depends on lenght of audio file, number of `count` and the performance of ASR' server.