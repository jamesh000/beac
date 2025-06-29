package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/libp2p/go-libp2p"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/multiformats/go-multiaddr"
)

const serviceName string = "/testingbeac/1.0"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// arguments
	port := flag.Int("l", 0, "Port to listen on")
	dest := flag.String("d", "", "Destination address")
	mdnsEnabled := flag.Bool("mdns", false, "Enable MDNS")
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
	topic, err := ps.Join(serviceName)
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

	var PeerC chan peer.AddrInfo
	if *mdnsEnabled {
		PeerC, err = initMdns(h, serviceName)
		if err != nil {
			panic(err)
		}
	}

	err = dht.Bootstrap(ctx)
	if err != nil {
		panic(err)
	}

	routingDiscovery := drouting.NewRoutingDiscovery(dht)
	dutil.Advertise(ctx, routingDiscovery, serviceName)

	routingPeerC, err := routingDiscovery.FindPeers(ctx, serviceName)
	if err != nil {
		panic(err)
	}

	for peer := range routingPeerC {
		if peer.ID == h.ID() {
			continue
		}
		err := h.Connect(ctx, peer)
		if err != nil {
			log.Println(err)
			continue
		}
		log.Println("Connected to peer", peer.ID)
	}

	donec := make(chan struct{}, 1)
	go publishLoop(ctx, topic, donec)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT)

	for {
		select {
		case pi := <-PeerC:
			if err := h.Connect(ctx, pi); err != nil {
				log.Println(err)
				continue
			}
			fmt.Println("Connected to", pi.ID)
		case <-donec:
			h.Close()
			os.Exit(0)
		case <-stop:
			h.Close()
		}
	}
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

func publishLoop(ctx context.Context, topic *pubsub.Topic, donec chan struct{}) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		msg := scanner.Text()
		err := topic.Publish(ctx, []byte(msg))
		if err != nil {
			fmt.Println("Failed to publish message on topic", topic.String(), "due to", err)
		}
	}
	donec <- struct{}{}
}

func getHostAddress(ha host.Host) string {
	// Build host multiaddress
	hostAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/p2p/%s", ha.ID()))

	// Now we can build a full multiaddress to reach this host
	// by encapsulating both addresses:
	addr := ha.Addrs()[0]
	return addr.Encapsulate(hostAddr).String()
}

type discoveryNotifee struct {
	PeerC chan peer.AddrInfo
}

func (n *discoveryNotifee) HandlePeerFound(pi peer.AddrInfo) {
	n.PeerC <- pi
}

func initMdns(h host.Host, service string) (chan peer.AddrInfo, error) {
	n := &discoveryNotifee{}
	n.PeerC = make(chan peer.AddrInfo)

	ser := mdns.NewMdnsService(h, service, n)
	if err := ser.Start(); err != nil {
		return nil, err
	}
	return n.PeerC, nil
}
