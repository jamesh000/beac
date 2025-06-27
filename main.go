package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/libp2p/go-libp2p"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/multiformats/go-multiaddr"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// arguments
	port := flag.Int("l", 8000, "Port to listen on")
	dest := flag.String("d", "", "Destination address")
	flag.Parse()

	// make host with Kademlia DHT routing
	h, dht, err := makeRoutedHost(ctx, *port)
	if err != nil {
		panic(err)
	}

	// Start pubsub (GossipSub)
	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		panic(err)
	}
	topic, err := ps.Join("testTopic")
	if err != nil {
		panic(err)
	}
	defer topic.Close()
	sub, err := topic.Subscribe()
	if err != nil {
		panic(err)
	}
	go subHandler(ctx, sub)

	fmt.Println(getHostAddress(h))

	if *dest != "" {
		targetAddr, err := multiaddr.NewMultiaddr(*dest)
		if err != nil {
			panic(err)
		}
		targetInfo, err := peer.AddrInfoFromP2pAddr(targetAddr)
		if err != nil {
			panic(err)
		}
		err = h.Connect(ctx, *targetInfo)
		if err != nil {
			panic(err)
		}
		fmt.Println("Connected to", targetInfo.ID)
	}

	err = dht.Bootstrap(ctx)
	if err != nil {
		panic(err)
	}

	publishLoop(ctx, topic)
}

func makeRoutedHost(ctx context.Context, port int) (host.Host, *kaddht.IpfsDHT, error) {
	// Create key pair
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, rand.Reader)
	if err != nil {
		log.Println(err)
		return nil, nil, err
	}

	nodeAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port))
	if err != nil {
		log.Println(err)
		return nil, nil, err
	}

	var dht *kaddht.IpfsDHT
	newDHT := func(h host.Host) (routing.PeerRouting, error) {
		var err error
		dht, err = kaddht.New(ctx, h)
		return dht, err
	}

	h, err := libp2p.New(
		libp2p.ListenAddrs(nodeAddr),
		libp2p.Identity(priv),
		libp2p.DefaultTransports,
		libp2p.DefaultMuxers,
		libp2p.DefaultSecurity,
		libp2p.NATPortMap(),
		libp2p.Routing(newDHT),
	)

	return h, dht, err
}

func subHandler(ctx context.Context, sub *pubsub.Subscription) {
	defer sub.Cancel()
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			fmt.Println("Failed to read next message on topic", sub.Topic(), "due to", err)
			continue
		}

		fmt.Println(msg)
	}
}

func publishLoop(ctx context.Context, topic *pubsub.Topic) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		msg := scanner.Text()
		err := topic.Publish(ctx, []byte(msg))
		if err != nil {
			fmt.Println("Failed to publish message on topic", topic.String(), "due to", err)
		}
	}
}

func getHostAddress(ha host.Host) string {
	// Build host multiaddress
	hostAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/p2p/%s", ha.ID()))

	// Now we can build a full multiaddress to reach this host
	// by encapsulating both addresses:
	addr := ha.Addrs()[0]
	return addr.Encapsulate(hostAddr).String()
}
