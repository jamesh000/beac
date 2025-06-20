package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"log"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/multiformats/go-multiaddr"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	port := flag.Int("p", 8000, "Port to listen on")
	dest := flag.String("d", "", "Destination address")
	flag.Parse()

	h, err := makeHost(*port)
	if err != nil {
		panic(err)
	}

	if *dest == "" {
		startPeer(ctx, h, handleStream)
	} else {
		rw, err := startPeerAndConnect(ctx, h, *dest)
		if err != nil {
			log.Println(err)
			return
		}
	}

	fmt.Println(host1)
}

func makeHost(port int) (host.Host, error) {
	// Create key pair
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, rand.Reader)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	nodeAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port))
	if err != nil {
		log.Println(err)
		return nil, err
	}

	return libp2p.New(
		libp2p.ListenAddrs(nodeAddr),
		libp2p.Identity(priv),
	)
}
