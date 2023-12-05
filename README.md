# GoSmartKnob #

A Go port of the software for Scott Bezek's [SmartKnob](https://github.com/scottbez1/smartknob/tree/master/software/js).

## Dependencies ##

For generating the SmartKnob protocol implementation:
Protobuf
NanoPB

If the version in this repo is the version you're working with, there is no need to regenerate it.

On my Mac, these dependencies were installed as follows:

```sh
brew install protobuf
brew install nanopb-generator
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```

Also on my Mac, generating the protobuf code for the SmartKnob protocol was done with the following command line:

```sh
protoc -I=pb -I=/usr/local/Cellar/nanopb-generator/0.4.8/libexec/proto/ --plugin=protoc-gen-nanopb=/usr/local/Cellar/nanopb-enerator/0.4.8/bin/protoc-gen-nanopb --go_out=pb --go_opt=paths=source_relative --go_opt=Msmartknob.proto=github.com/marcuswu/gosmartknob/pb --go_opt=Mnanopb.proto=github.com/marcuswu/gosmartknob/pb pb/smartknob.proto
```

## Use ##

A new SmartKnob can be initialized with the New method:
```
func New(connection io.ReadWriteCloser, onMessage MessageCallback) *SmartKnob
```

The SmartKnob instance will begin to use the io.ReadWriteCloser for communication with the SmartKnob.
If the port is closed for any reason calling SetReadWriter with a new io.ReadWriteCloser will allow it to continue with the new connection.

```
func (skc *SmartKnob) SetReadWriter(readWriter io.ReadWriteCloser)
```

Sending a message to the SmartKnob is done via the EnqueueMessage method.

```
func (skc *SmartKnob) EnqueueMessage(message *pb.ToSmartknob) error
```