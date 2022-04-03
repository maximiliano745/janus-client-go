# Janus WebRTC Gateway API Client for Golang

> support only for janus multistream (latest)

Janus WebRTC server client write in Go language. implemented using Websocket transport protocol to communicate with Janus API.


## Usage

**Install**
```shell
go get github.com/abdularis/janus-client-go
```

note that if WebSocket connection to Janus server terminated all resources in janus such as 
sessions/handles will also be cleared.
Unless you set non-zero value to `reclaim_session_timeout` in `janus.jcfg` to give chance for session to be reclaimed.
this library will try to reclaim session if connection terminated abnormally.

**Connect to Janus Gateway**
```go
// connect to janus using ws transport
gateway, err := janus.Connect("ws://localhost:8188")
if err != nil {
    log.Fatal(err)
}

// get janus server info and print them
info, err := gateway.Info()
if err != nil {
    log.Fatal(err)
}
fmt.Printf("%+v\n", info)
```

**Create Janus Session & Handle**
```go
...

session, err := gateway.Create()
if err != nil {
    log.Fatal(err)
}

// create new plugin handle and attach it to video room plugin
handle, err := session.Attach(videoroom.PackageName)
if err != nil {
    log.Fatal(err)
}
```

**Keep Janus Session Alive**

Janus session will timeout after a while as configured, send keep alive message
to keep it stay
```go
...

go func() {
    t := time.NewTicker(time.Second * 50)
    for ; ; <-t.C {
        _ = session.KeepAlive()
    }
}()
```

## Janus Plugin

Janus plugin could process request synchronously or asynchronously, read the docs for each plugin to know more.
Therefore, use `handle.Request()` for synchronous operation and `handle.Message()` for asynchronous operation.

Implemented wrappers:
- VideoRoom (WIP)
- AudioBridge (WIP)