package main

import (
	"flag"

	"github.com/ipfs/go-log"
	overlay "github.com/lanzafame/go-libp2p-dht-overlay"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	maddr "github.com/multiformats/go-multiaddr"
)

var logger = log.Logger("ov-get")

// ParseFlags parses the cli flags into the configuration values.
func ParseFlags() (overlay.Config, error) {
	config := overlay.Config{}
	flag.StringVar(&config.RendezvousString, "rendezvous", "overlay-1",
		"Unique string to identify group of nodes. Share this with your friends to let them connect with you")
	// flag.Var(&config.BootstrapPeers, "peer", "Adds a peer multiaddress to the bootstrap list")
	flag.Var(&config.ListenAddresses, "listen", "Adds a multiaddress to the listen list")
	flag.Var(&config.OverlayListenAddresses, "overlaylisten", "Adds a multiaddress to the overlay listen list")
	flag.StringVar(&config.ProtocolID, "pid", "/overlay/1.0.0", "Sets a protocol id for stream headers")
	flag.Parse()

	// if len(config.BootstrapPeers) == 0 {
	// 	config.BootstrapPeers = dht.DefaultBootstrapPeers
	// }
	myPeer, err := maddr.NewMultiaddr("/ip4/61.68.63.58/tcp/4001/ipfs/QmV8LYMZ1KLRzJacFCXrUjYJdHN4XcdTvLmswdkzwVjjdV")
	if err != nil {
		panic(err)
	}
	config.BootstrapPeers = append(dht.DefaultBootstrapPeers, myPeer)

	return config, nil
}
