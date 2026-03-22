# Video Player

Plays video files on an HTML canvas — frames are decoded in Go via `ffmpeg`, sent to the browser, and painted on a `<canvas>` element. No `<video>` tag involved.

## Requirements

- [ffmpeg](https://ffmpeg.org/) installed and in your `PATH`

## Usage

```bash
go run ./examples/video-player/ -video /path/to/video.mp4
```

With godom flags:

```bash
go run ./examples/video-player/ -video /path/to/video.mp4 --port 8099 --no-auth
```

## Controls

- **Play / Pause** — toggle playback
- **Stop** — reset to first frame
- **-5s / +5s** — skip backward or forward

## How it works

1. Go shells out to `ffmpeg` to decode the video into JPEG frames at 24fps
2. Frames are stored in memory and base64-encoded as plugin data
3. The JS plugin (20 lines) decodes each JPEG and draws it on canvas via `drawImage`
4. All playback logic (play/pause/stop/seek) lives entirely in Go
