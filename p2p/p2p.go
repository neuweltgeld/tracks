package p2p

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	multiaddr "github.com/multiformats/go-multiaddr"
	"os"
	"sync"
)

var connectedPeers = make(map[peer.ID]peer.AddrInfo)
var mutex = &sync.Mutex{} // For synchronizing access to connectedPeers

func startNode(ctx context.Context) (host.Host, error) {
	// Hex-encoded private key
	serializedPrivKey := "08011240ced049e2de9cdd76c281975d59ee35a950a0b0239c603628fda5b7766080310d1a379e48c51ebc92f96351530954cd0a83726bdb5978d7fcc9926a547f903e8d"

	// Decode the hex-encoded private key
	privKeyBytes, err := hex.DecodeString(serializedPrivKey)
	if err != nil {
		panic(fmt.Errorf("failed to decode private key: %w", err))
	}

	// Unmarshal the private key bytes into a libp2p PrivKey object
	privKey, err := crypto.UnmarshalPrivateKey(privKeyBytes)
	if err != nil {
		panic(fmt.Errorf("failed to unmarshal private key: %w", err))
	}

	node, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/2300"), // Listen on all interfaces and a random port
		libp2p.Identity(privKey),                          // Use the private key to identify this node
		libp2p.Ping(false),                                // Disable the built-in ping protocol
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	// Register connection handler to update connectedPeers
	node.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(n network.Network, c network.Conn) {
			mutex.Lock()
			peerInfo := peer.AddrInfo{
				ID:    c.RemotePeer(),
				Addrs: []multiaddr.Multiaddr{c.RemoteMultiaddr()},
			}
			connectedPeers[c.RemotePeer()] = peerInfo
			mutex.Unlock()
			fmt.Printf("Connected to %s\n", c.RemotePeer())
		},
		DisconnectedF: func(n network.Network, c network.Conn) {
			mutex.Lock()
			delete(connectedPeers, c.RemotePeer())
			mutex.Unlock()
			fmt.Printf("Disconnected from %s\n", c.RemotePeer())
		},
	})

	return node, nil
}

func printNodeInfo(node host.Host) {
	fmt.Println("Listen addresses:", node.Addrs())
	peerInfo := peer.AddrInfo{
		ID:    node.ID(),
		Addrs: node.Addrs(),
	}
	addrs, err := peer.AddrInfoToP2pAddrs(&peerInfo)
	if err != nil {
		fmt.Println("Failed to obtain p2p addresses:", err)
		return
	}
	for _, addr := range addrs {
		fmt.Println("libp2p node address:", addr)
	}

	privKey := node.Peerstore().PrivKey(node.ID())
	if privKey == nil {
	}
	fmt.Printf("Node's Private Key: %x\n", privKey)
	// Convert the private key to a bytes representation (for demonstration purposes)
	privBytes, err := crypto.MarshalPrivateKey(privKey)
	if err != nil {
	}

	// Print the private key bytes
	fmt.Printf("Node's Private Key: %x\n", privBytes)

}

func connectToPeer(ctx context.Context, node host.Host, addrStr string) error {
	addr, err := multiaddr.NewMultiaddr(addrStr)
	if err != nil {
		return fmt.Errorf("parsing multiaddr failed: %w", err)
	}
	peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return fmt.Errorf("creating peer info failed: %w", err)
	}
	if err := node.Connect(ctx, *peerInfo); err != nil {
		return fmt.Errorf("connecting to peer failed: %w", err)
	}
	return nil
}

const customProtocolID = "/station/tracks/0.0.1"

func setupStreamHandler(node host.Host) {
	node.SetStreamHandler(protocol.ID(customProtocolID), func(s network.Stream) {
		defer s.Close()
		buf := bufio.NewReader(s)
		str, err := buf.ReadString('\n')
		if err != nil {
			fmt.Println("Failed to read from stream:", err)
			os.Exit(1)
		}

		var receivedNumber int
		_, err = fmt.Sscanf(str, "Random number: %d", &receivedNumber)
		if err != nil {
			fmt.Println("Failed to parse received number:", err)
			os.Exit(1)
		}

		fmt.Printf("Received random number: %d\n", receivedNumber)

	})
}

func sendMessage(ctx context.Context, node host.Host, peerID peer.ID, message string) error {
	s, err := node.NewStream(ctx, peerID, protocol.ID(customProtocolID))
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}
	defer s.Close()

	_, err = s.Write([]byte(message + "\n"))
	if err != nil {
		return fmt.Errorf("failed to write message to stream: %w", err)
	}

	return nil
}

// Function to broadcast a message to all connected peers
func BroadcastMessage(ctx context.Context, host host.Host, message string) {
	mutex.Lock()
	defer mutex.Unlock()
	for peerID, _ := range connectedPeers {
		if peerID == host.ID() {
			continue // Skip sending message to self
		}
		if err := sendMessage(ctx, host, peerID, message); err != nil {
			fmt.Printf("Error broadcasting message to %s: %s\n", peerID, err)
		}
	}
}

func P2PConfiguration() bool {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	node, err := startNode(ctx)
	if err != nil {
		panic(err)
		return false
	}
	defer node.Close()
	printNodeInfo(node)
	setupStreamHandler(node)
	return true
}