package gosmartknob

import (
	"errors"
	"hash/crc32"
	"io"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/marcuswu/gosmartknob/core"
	"github.com/marcuswu/gosmartknob/pb"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/proto"
)

const (
	protobufProtocolVersion = 1
	retryMillis             = 250
	baud                    = 921600
	Ch340VendorId           = 0x1a86
	Ch340ProductId          = 0x7523
	Esp32s3VendorId         = 0x303a
	Esp32s3ProductId        = 0x1001
)

var DefaultDeviceFilters = []core.UsbFilter{
	core.UsbFilter{
		VendorId:  Ch340VendorId,
		ProductId: Ch340ProductId,
	},
	core.UsbFilter{
		VendorId:  Esp32s3VendorId,
		ProductId: Esp32s3ProductId,
	},
}

type MessageCallback func(message *pb.FromSmartKnob)
type SendBytes func(packet []uint8)

type QueueEntry struct {
	nonce          uint32
	encodedPayload []uint8
}

type SmartKnob struct {
	onMessage     MessageCallback
	connection    io.ReadWriteCloser
	outgoingQueue []QueueEntry
	lastNonce     uint32
	readRunning   atomic.Bool
	buffer        []byte
	retry         *time.Timer
}

func New(connection io.ReadWriteCloser, onMessage MessageCallback) *SmartKnob {
	sk := &SmartKnob{
		onMessage:     onMessage,
		outgoingQueue: make([]QueueEntry, 0),
		lastNonce:     uint32(rand.Int31()),
		readRunning:   atomic.Bool{},
		buffer:        make([]byte, 0),
	}

	sk.readRunning.Store(false)
	sk.SetReadWriter(connection)
	return sk
}

func (skc *SmartKnob) sendBytes(data []byte) {
	for data := data; len(data) > 0; {
		written, err := skc.connection.Write(data)
		data = data[written:]
		if err != nil && (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) {
			skc.connection.Close()
			skc.connection = nil
		}
	}
}

func (skc *SmartKnob) SetReadWriter(readWriter io.ReadWriteCloser) {
	if skc.readRunning.Load() {
		log.Debug().Msg("Attempting to change port")
		skc.connection.Close()

		// Wait for read go routine to exit before setting the new one up
		for skc.readRunning.Load() {
		}
	}
	skc.connection = readWriter
	go func() {
		skc.readRunning.Store(true)
		defer skc.readRunning.Store(false)
		for {
			if skc.connection == nil {
				return
			}
			buffer := make([]byte, 255)
			read, err := skc.connection.Read(buffer)
			if read > 0 {
				skc.onReceivedData(buffer)
			}
			if err != nil && (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) {
				skc.connection.Close()
				skc.connection = nil
				return
			}
		}
	}()
}

func (skc *SmartKnob) SendConfig(message *pb.SmartKnobConfig) {
	skc.EnqueueMessage(
		&pb.ToSmartknob{
			Payload: &pb.ToSmartknob_SmartknobConfig{
				SmartknobConfig: message,
			},
		},
	)
}

func (skc *SmartKnob) onReceivedData(data []byte) {
	skc.buffer = append(skc.buffer, data...)

	endOfNextPacket := func(buffer []byte) (int, error) {
		for i := 0; i < len(buffer); i++ {
			if buffer[i] == 0 {
				return i, nil
			}
		}
		return 0, errors.New("not found")
	}

	for i, err := endOfNextPacket(skc.buffer); err == nil; i, err = endOfNextPacket(skc.buffer) {
		rawBuffer := skc.buffer[:i]
		packet := core.CobsDecode(rawBuffer)
		skc.buffer = skc.buffer[i+1:]
		if len(packet) <= 4 {
			log.Debug().Bytes("Packet", skc.buffer[0:i]).Msg("Received short packet")
			continue
		}
		payload := packet[0 : len(packet)-4]

		// Validate CRC32
		crcBuf := packet[len(packet)-4:]
		providedCrc := uint32(crcBuf[0]) | uint32(crcBuf[1]<<8) | uint32(crcBuf[2]<<16) | uint32(crcBuf[3]<<24)
		crc := crc32.ChecksumIEEE(payload)
		if crc != providedCrc {
			log.Debug().Uint32("expected", crc).Uint32("received", providedCrc).
				Bytes("raw buffer", rawBuffer).Msg("Bad CRC")
			continue
		}

		message := &pb.FromSmartKnob{}
		if err := proto.Unmarshal(payload, message); err != nil {
			log.Warn().Bytes("message", payload).Msg("Invalid protobuf message")
			return
		}
		if message.ProtocolVersion != protobufProtocolVersion {
			log.Warn().
				Int("expected", protobufProtocolVersion).
				Int("received", int(message.ProtocolVersion)).
				Msg("Invalid protocol version")
			return
		}

		if message.GetAck() != nil {
			skc.handleAck(message.GetAck().Nonce)
		}
		skc.onMessage(message)
	}
}

func (skc *SmartKnob) EnqueueMessage(message *pb.ToSmartknob) error {
	if skc.connection == nil {
		return errors.New("port is unavailable")
	}
	message.ProtocolVersion = protobufProtocolVersion
	skc.lastNonce = skc.lastNonce + 1
	message.Nonce = skc.lastNonce

	// Encode before enqueueing to ensure messages don't change once they're queued
	payload, err := proto.Marshal(message)
	if err != nil {
		return err
	}
	// payload := PB.ToSmartknob.encode(message).finish()

	if len(skc.outgoingQueue) > 10 {
		log.Warn().Int("pending messages dropped", len(skc.outgoingQueue)).
			Msg("SmartKnob outgoing queue overflowed!")
		skc.outgoingQueue = make([]QueueEntry, 0)
	}
	skc.outgoingQueue = append(skc.outgoingQueue, QueueEntry{
		nonce:          message.Nonce,
		encodedPayload: payload,
	})
	skc.serviceQueue()
	return nil
}

func (skc *SmartKnob) handleAck(nonce uint32) {
	if len(skc.outgoingQueue) < 1 || nonce != skc.outgoingQueue[0].nonce {
		log.Warn().Uint32("nonce", nonce).Msg("Ignoring unexpected ack")
		return
	}

	// Cancel retry if it exists
	if skc.retry != nil {
		skc.retry.Stop()
		skc.retry = nil
	}
	skc.outgoingQueue = skc.outgoingQueue[1:]
	skc.serviceQueue()
}

func (skc *SmartKnob) serviceQueue() {
	if skc.connection == nil {
		return
	}
	if skc.retry == nil {
		// retry is pending; let the pending timer handle the next step
		return
	}
	if len(skc.outgoingQueue) == 0 {
		return
	}

	payload := skc.outgoingQueue[0].encodedPayload

	crc := crc32.ChecksumIEEE(payload)
	crcData := []byte{
		byte(crc & 0xff),
		byte((crc >> 8) & 0xff),
		byte((crc >> 16) & 0xff),
		byte((crc >> 24) & 0xff),
	}
	packet := make([]byte, 0, len(payload)+4)
	packet = append(packet, payload...)
	packet = append(packet, crcData...)

	cobsEncodedPacket := core.CobsEncode(packet)
	encodedDelimitedPacket := make([]byte, 0, len(cobsEncodedPacket)+1)
	encodedDelimitedPacket = append(encodedDelimitedPacket, cobsEncodedPacket...)
	encodedDelimitedPacket = append(encodedDelimitedPacket, 0x00)

	skc.retry = time.AfterFunc(retryMillis, func() {
		skc.retry = nil
		log.Debug().Msg("Retrying ToSmartKnob...")
		skc.serviceQueue()
	})

	log.Debug().
		Int("payload length", len(payload)).
		Int("encoded length", len(cobsEncodedPacket)).
		Bytes("CRC", crcData).
		Msg("Sent payload to SmartKnob")

	skc.sendBytes(encodedDelimitedPacket)
}
