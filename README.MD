# go janus client

> support only for janus multistream (latest)

Janus WebRTC server client write in Go language. implemented using Websocket transport protocol to communicate with Janus API.

Janus plugin could process request synchronously or asynchronously, read the docs for each plugin to know more.
Therefore, use `handle.Request()` for synchronous operation and `handle.Message()` for asynchronous operation.

> note: Closing websocket connection will also tear down all session it created.