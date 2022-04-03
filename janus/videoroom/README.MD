## Janus VideoRoom

Janus *videoroom* plugin implement `Publish/Subscribe` pattern

### Publisher ([ref](https://janus.conf.meetecho.com/docs/videoroom.html#vroompub))
Publisher is participants that are able to publish media stream (audio/video) to other peers in the room (although may 
choose not to)

### Subscriber ([ref](https://janus.conf.meetecho.com/docs/videoroom.html#vroomsub))
Subscribers are not participant, this only used is to receive media stream 
from one or more participant (publisher) in the room.
Other participant will be notified about new participant, 
as the result they will create subscriber session to receive media stream
from this new participant, when a participant goes away, 
other participants will be notified so they can remove subscription session accordingly


## Video Conference
at least 2 PeerConnection needed for this use case, one PeerConnection as publisher and will publish media to janus,
and one for subscriber which will subscribe to one or all publisher (with multistream)

with non multistream, there will be one PeerConnection per subscription for publishers media 

## TODO
- [ ] Destroy room
- [ ] List room
- [x] Broadcast Events channel

Ref:
- https://janus.conf.meetecho.com/docs/videoroom.html
- https://github.com/meetecho/janus-gateway/blob/master/src/plugins/janus_videoroom.c