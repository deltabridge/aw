package tcp_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"reflect"
	"testing/quick"
	"time"

	"github.com/renproject/aw/dht"
	"github.com/renproject/aw/handshake"
	"github.com/renproject/aw/protocol"
	"github.com/renproject/aw/tcp"
	"github.com/renproject/aw/testutil"
	"github.com/renproject/kv"
	"github.com/sirupsen/logrus"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Tcp", func() {
	initServer := func(ctx context.Context, sender protocol.MessageSender, hs handshake.Handshaker, port int) {
		tcp.NewServer(tcp.ServerOptions{
			Logger:     logrus.StandardLogger(),
			Timeout:    time.Minute,
			Handshaker: hs,
			Port:       port,
		}, sender).Run(ctx)
	}

	initClient := func(ctx context.Context, receiver protocol.MessageReceiver, hs handshake.Handshaker, dht dht.DHT) {
		tcp.NewClient(
			tcp.NewClientConns(tcp.ClientOptions{
				Logger:         logrus.StandardLogger(),
				Timeout:        time.Minute,
				MaxConnections: 10,
				Handshaker:     hs,
			}),
			dht, receiver,
		).Run(ctx)
	}

	Context("when sending a valid message", func() {
		It("should successfully send and receive", func() {
			ctx, cancel := context.WithCancel(context.Background())
			// defer time.Sleep(time.Millisecond)
			defer cancel()

			fromServer := make(chan protocol.MessageOnTheWire, 1000)
			toClient := make(chan protocol.MessageOnTheWire, 1000)

			sender := testutil.NewSimpleTCPPeerAddress("sender", "127.0.0.1", "47325")
			receiver := testutil.NewSimpleTCPPeerAddress("receiver", "127.0.0.1", "47326")
			codec := testutil.NewSimpleTCPPeerAddressCodec()
			dht, err := dht.New(sender, codec, kv.NewTable(kv.NewMemDB(kv.JSONCodec), "dht"), receiver)
			Expect(err).Should(BeNil())

			// start TCP server and client
			go initServer(ctx, fromServer, nil, 47326)
			go initClient(ctx, toClient, nil, dht)

			check := func(x uint, y uint) bool {
				// Set variant and body size to an appropriate range
				v := int((x % 5) + 1)
				numBytes := int(y % 1000000) // 0 bytes to 1 megabyte

				// Generate random variant
				variant := protocol.MessageVariant(v)
				body := make([]byte, numBytes)
				if len(body) > 0 {
					n, err := rand.Read(body)
					Expect(n).To(Equal(numBytes))
					Expect(err).ToNot(HaveOccurred())
				}

				// Send a message to the server using the client and read from
				// message received by the server
				toClient <- protocol.MessageOnTheWire{
					To:      receiver.ID,
					Message: protocol.NewMessage(protocol.V1, variant, body),
				}
				messageOtw := <-fromServer

				// Check that the message sent equals the message received
				return (messageOtw.Message.Length == protocol.MessageLength(len(body)+32) &&
					reflect.DeepEqual(messageOtw.Message.Version, protocol.V1) &&
					reflect.DeepEqual(messageOtw.Message.Variant, variant) &&
					bytes.Compare(messageOtw.Message.Body, body) == 0)
			}

			Expect(quick.Check(check, &quick.Config{
				MaxCount: 100,
			})).Should(BeNil())
		})

		It("should successfully send and receive when both nodes are authenticated", func() {
			ctx, cancel := context.WithCancel(context.Background())
			// defer time.Sleep(time.Millisecond)
			defer cancel()

			fromServer := make(chan protocol.MessageOnTheWire, 1000)
			toClient := make(chan protocol.MessageOnTheWire, 1000)

			clientSignVerifier := testutil.NewMockSignVerifier()
			serverSignVerifier := testutil.NewMockSignVerifier(clientSignVerifier.ID())
			clientSignVerifier.Whitelist(serverSignVerifier.ID())

			sender := testutil.NewSimpleTCPPeerAddress("sender", "127.0.0.1", "47325")
			receiver := testutil.NewSimpleTCPPeerAddress("receiver", "127.0.0.1", "47326")
			codec := testutil.NewSimpleTCPPeerAddressCodec()
			dht, err := dht.New(sender, codec, kv.NewTable(kv.NewMemDB(kv.JSONCodec), "dht"), receiver)
			Expect(err).Should(BeNil())

			// start TCP server and client
			go initServer(ctx, fromServer, handshake.New(serverSignVerifier), 47326)
			go initClient(ctx, toClient, handshake.New(clientSignVerifier), dht)

			check := func(x uint, y uint) bool {
				// Set variant and body size to an appropriate range
				v := int((x % 5) + 1)
				numBytes := int(y % 1000000) // 0 bytes to 1 megabyte

				// Generate random variant
				variant := protocol.MessageVariant(v)
				body := make([]byte, numBytes)
				if len(body) > 0 {
					n, err := rand.Read(body)
					Expect(n).To(Equal(numBytes))
					Expect(err).ToNot(HaveOccurred())
				}

				// Send a message to the server using the client and read from
				// message received by the server
				toClient <- protocol.MessageOnTheWire{
					To:      receiver.ID,
					Message: protocol.NewMessage(protocol.V1, variant, body),
				}
				messageOtw := <-fromServer

				// Check that the message sent equals the message received
				return (messageOtw.Message.Length == protocol.MessageLength(len(body)+32) &&
					reflect.DeepEqual(messageOtw.Message.Version, protocol.V1) &&
					reflect.DeepEqual(messageOtw.Message.Variant, variant) &&
					bytes.Compare(messageOtw.Message.Body, body) == 0)
			}

			Expect(quick.Check(check, &quick.Config{
				MaxCount: 100,
			})).Should(BeNil())
		})

		FIt("should successfully send and receive", func() {
			ctx, cancel := context.WithCancel(context.Background())
			// defer time.Sleep(time.Millisecond)
			defer cancel()

			fromServerChans := make([]chan protocol.MessageOnTheWire, 10)
			receivers := make([]protocol.PeerAddress, 10)
			for i := range receivers {
				receivers[i] = testutil.NewSimpleTCPPeerAddress(fmt.Sprintf("receiver_%d", i), "127.0.0.1", fmt.Sprintf("%d", 47326+i))
				fromServerChans[i] = make(chan protocol.MessageOnTheWire, 100000)
				go initServer(ctx, fromServerChans[i], nil, 47326+i)
			}

			codec := testutil.NewSimpleTCPPeerAddressCodec()
			toClientChans := make([]chan protocol.MessageOnTheWire, 10)
			clients := make([]protocol.PeerAddress, 10)
			for i := range clients {
				sender := testutil.NewSimpleTCPPeerAddress(fmt.Sprintf("sender_%d", i), "127.0.0.1", fmt.Sprintf("%d", 47325-i))
				dht, err := dht.New(sender, codec, kv.NewTable(kv.NewMemDB(kv.JSONCodec), "dht"), receivers...)
				Expect(err).Should(BeNil())
				clients[i] = testutil.NewSimpleTCPPeerAddress(fmt.Sprintf("receiver_%d", i), "127.0.0.1", fmt.Sprintf("%d", 47326+i))
				toClientChans[i] = make(chan protocol.MessageOnTheWire, 100000)
				go initClient(ctx, toClientChans[i], nil, dht)
			}

			check := func(x uint, y uint) bool {
				// Set variant and body size to an appropriate range
				// v := int((x % 5) + 1)
				numBytes := int(y % 1000000) // 0 bytes to 1 megabyte

				// Generate random variant
				// variant := protocol.MessageVariant(v)
				body := make([]byte, numBytes)
				if len(body) > 0 {
					n, err := rand.Read(body)
					Expect(n).To(Equal(numBytes))
					Expect(err).ToNot(HaveOccurred())
				}

				// Send a message to the server using the client and read from
				// message received by the server
				for _, toClient := range toClientChans {
					toClient <- protocol.MessageOnTheWire{
						Message: protocol.NewMessage(protocol.V1, protocol.Multicast, body),
					}
				}
				for range toClientChans {
					for _, fromServer := range fromServerChans {
						messageOtw := <-fromServer
						if !(messageOtw.Message.Length == protocol.MessageLength(len(body)+32) &&
							reflect.DeepEqual(messageOtw.Message.Version, protocol.V1) &&
							reflect.DeepEqual(messageOtw.Message.Variant, protocol.Multicast) &&
							bytes.Compare(messageOtw.Message.Body, body) == 0) {
							return false
						}
					}
				}
				// Check that the message sent equals the message received
				return true
			}

			Expect(quick.Check(check, &quick.Config{
				MaxCount: 100,
			})).Should(BeNil())
		})
	})
})
