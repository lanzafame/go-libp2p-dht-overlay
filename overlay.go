/*
Package overlay spec outline
 - spin up host/peer that will be a part of the overlay network
 - spin up host/peer that will connect to public dht to perform rendezvous
 - perform rendezvous on the namespace of the overlay dht 'name', i.e. 'cat-pics-0001'
 - if there are any peers in the namespace,
	- send a 'request' to them for their associated peer address, i.e. thethe peer/host connected to the overlay dht
	- hand the responded address to the peer associated with the current peer
	- the associated peer will then perform a bootstrapping to the provided address

 - future optimizations:
	- only require the last n peers to stay in the public dht network providing the rendezvous service
	- if the number of peers in the overlay network drop below n, spin the public peer up and start rendezvous again
*/
package overlay

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/ipfs/go-log"
	libp2p "github.com/libp2p/go-libp2p"
	crypto "github.com/libp2p/go-libp2p-crypto"
	discovery "github.com/libp2p/go-libp2p-discovery"
	host "github.com/libp2p/go-libp2p-host"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	inet "github.com/libp2p/go-libp2p-net"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	protocol "github.com/libp2p/go-libp2p-protocol"
	routedhost "github.com/libp2p/go-libp2p/p2p/host/routed"
	maddr "github.com/multiformats/go-multiaddr"
	logging "github.com/whyrusleeping/go-logging"
)

var logger = log.Logger("overlay-coordinator")

func init() {
	log.SetAllLoggers(logging.WARNING)
	log.SetLogLevel("overlay-coordinator", "DEBUG")
}

// Coordinator represents a Peer that connects to the Public DHT,
// a Peer that connects to an overlay DHT and the communication
// communication channels in between those peers.
type Coordinator struct {
	*Identity
	Config

	PublicPeer, OverlayPeer host.Host

	PublicDHT, OverlayDHT *dht.IpfsDHT

	peerInfoCh chan peerstore.PeerInfo

	overlayBootMu       sync.RWMutex
	overlayBootstrapped bool
}

// NewCoordinator is a constructor for the Coordinator type.
func NewCoordinator(ctx context.Context, config Config) (*Coordinator, error) {
	p := &Coordinator{Config: config}

	iden, err := LoadIdentity("./identity.json")
	if err != nil {
		logger.Error(err)
	}

	if iden == nil {
		iden = &Identity{}

		iden.PublicPrivKey, _, err = crypto.GenerateKeyPair(crypto.RSA, 2048)
		if err != nil {
			return nil, err
		}
		iden.OverlayPrivKey, _, err = crypto.GenerateKeyPair(crypto.RSA, 2048)
		if err != nil {
			return nil, err
		}
	}

	p.Identity = iden

	// libp2p.New constructs a new libp2p Host. Other options can be added
	// here.
	publicPeer, err := libp2p.New(
		ctx,
		libp2p.Identity(p.Identity.PublicPrivKey),
		libp2p.ListenAddrs([]maddr.Multiaddr(p.ListenAddresses)...),
		libp2p.NATPortMap(),
	)
	if err != nil {
		return nil, err
	}

	overlayPeer, err := libp2p.New(
		ctx,
		libp2p.Identity(p.Identity.OverlayPrivKey),
		libp2p.ListenAddrs([]maddr.Multiaddr(p.OverlayListenAddresses)...),
		libp2p.NATPortMap(),
	)
	if err != nil {
		return nil, err
	}

	// Set a function as stream handler. This function is called when a peer
	// initiates a connection and starts a stream with this peer.
	publicPeer.SetStreamHandler(protocol.ID(p.ProtocolID), publicPeerStreamHandlerFunc(p))

	// Start a DHT, for use in peer discovery. We can't just make a new DHT
	// client because we want each peer to maintain its own local copy of the
	// DHT, so that the bootstrapping node of the DHT can go down without
	// inhibiting future peer discovery.
	p.PublicDHT, err = dht.New(ctx, publicPeer)
	if err != nil {
		return nil, err
	}

	// Bootstrap the DHT. In the default configuration, this spawns a Background
	// thread that will refresh the peer table every five minutes.
	// logger.Debug("Bootstrapping the DHT")

	// create OverlayDHT
	p.OverlayDHT, err = dht.New(ctx, overlayPeer)
	if err != nil {
		return nil, err
	}

	// wrap hosts in routedHost
	p.PublicPeer = routedhost.Wrap(publicPeer, p.PublicDHT)
	p.OverlayPeer = routedhost.Wrap(overlayPeer, p.OverlayDHT)

	str := `Coordinator:
	PublicPeer.ID: %v
	PublicPeer.Addrs: %v
	OverlayPeer.ID: %v
	OverlayPeer.Addrs: %v`
	logger.Debugf(
		str,
		p.PublicPeer.ID().Pretty(),
		p.PublicPeer.Addrs(),
		p.OverlayPeer.ID().Pretty(),
		p.OverlayPeer.Addrs(),
	)

	err = p.Identity.Save("./identity.json")
	if err != nil {
		logger.Error(err)
	}

	return p, nil
}

// Bootstrap bootstraps the public peer, connects to rendezvous peers, and then
// bootstraps the overlay peer.
func (p *Coordinator) Bootstrap(ctx context.Context) error {
	err := p.bootstrapPublicIPFSNetwork(ctx, p.Config.BootstrapPeers)
	if err != nil {
		logger.Error(err)
	}

	go p.bootstrapOverlayNetwork(ctx)

	err = p.coordinateOverlayPeers(ctx)
	if err != nil {
		logger.Error(err)
	}

	return nil
}

// bootstrapPublicIPFSNetwork connects the PublicPeer to the configured BootstrapPeers.
func (p *Coordinator) bootstrapPublicIPFSNetwork(ctx context.Context, peers []maddr.Multiaddr) error {
	connected := make(chan struct{})
	// bootstrap public peer
	var wg sync.WaitGroup
	logger.Debugf("bootstrap peers: %d", len(peers))
	for _, peerAddr := range peers {
		peerinfo, _ := peerstore.InfoFromP2pAddr(peerAddr)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := p.PublicPeer.Connect(ctx, *peerinfo); err != nil {
				logger.Error(err)
				return
			}
			logger.Info("connection established with bootstrap node: ", *peerinfo)
			connected <- struct{}{}
		}()
	}
	logger.Debug("waiting for connection go routines to return")

	go func() {
		wg.Wait()
		close(connected)
	}()

	i := 0
	for range connected {
		i++
	}
	if nPeers := len(peers); i < nPeers/2 {
		logger.Warningf("only connected to %d bootstrap peers out of %d", i, nPeers)
	} else {
		logger.Warningf("connected to %d bootstrap peers out of %d", i, nPeers)
	}

	return p.PublicDHT.Bootstrap(ctx)
}

// bootstrapOverlayNetwork connects the PublicPeer to the configured BootstrapPeers.
func (p *Coordinator) bootstrapOverlayNetwork(ctx context.Context) error {
	connected := make(chan struct{})
	// bootstrap public peer
	var wg sync.WaitGroup
	for pi := range p.peerInfoCh {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := p.OverlayPeer.Connect(ctx, *pi); err != nil {
				logger.Error(err)
				return
			}
			logger.Info("connection established with bootstrap node: ", *pi)
			connected <- struct{}{}
		}()
	}
	logger.Debug("waiting for connection go routines to return")

	go func() {
		wg.Wait()
		close(connected)
	}()

	i := 0
	for range connected {
		i++
	}
	logger.Warningf("connected to %d bootstrap peers", i)

	return p.OverlayDHT.Bootstrap(ctx)
}

// coordinateOverlayPeers coordinates between the public peer and the
// overlay peer to gather overlay peer addresses.
func (p *Coordinator) coordinateOverlayPeers(ctx context.Context) error {
	logger.Debug("starting discovery")

	// annouce on the public dht that we are a coordinator node
	routingDiscovery := discovery.NewRoutingDiscovery(p.PublicDHT)
	logger.Debug("advertising peer")
	discovery.Advertise(ctx, routingDiscovery, p.Config.RendezvousString)

	logger.Debug("finding other peers")
	// perform rendezvous to get the addresses of other coordinator public peers
	peerChan, err := routingDiscovery.FindPeers(ctx, p.Config.RendezvousString)
	if err != nil {
		return err
	}

	gotPeerCh := make(chan struct{})
	go func() {
		// process peers performing rendezvous protocol
		for pr := range peerChan {
			if pr.ID == p.PublicPeer.ID() {
				continue
			}

			logger.Info("connecting to peer: ", pr)
			stream, err := p.PublicPeer.NewStream(ctx, pr.ID, protocol.ID(p.Config.ProtocolID))
			if err != nil {
				logger.Error("connection failed: ", err)
				continue
			}
			close(gotPeerCh)
			rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

			go p.readOverlayPeerInfo(rw)
			go p.sendOverlayPeerInfo(rw)
		}
	}()
	<-gotPeerCh

	return nil
}

func publicPeerStreamHandlerFunc(p *Coordinator) func(inet.Stream) {
	return func(stream inet.Stream) {
		logger.Info("Got a new stream!")

		// Create a buffer stream for non blocking read and write.
		rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

		go p.readOverlayPeerInfo(rw)
		go p.sendOverlayPeerInfo(rw)
	}
}

func (p *Coordinator) readOverlayPeerInfo(rw *bufio.ReadWriter) {
	logger.Info("reading overlay peer info")
	str, err := rw.ReadString('\n')
	if err != nil {
		logger.Error("error reading from buffer")
		panic(err)
	}

	mas, err := textUnmarshalMultiaddrSlice(str)
	if err != nil {
		logger.Error(str)
		panic(err)
	}

	logger.Info("got multiaddr, putting on addrCh")
	for _, ma := range mas {
		p.addrsCh <- ma
	}
}

func (p *Coordinator) sendOverlayPeerInfo(rw *bufio.ReadWriter) {
	logger.Info("sending overlay peer info")

	logger.Info(p.OverlayPeer.Addrs())
	mas, err := textMarshalMultiaddrSlice(p.OverlayPeer.Addrs())
	if err != nil {
		logger.Errorf("sendOverlayPeerAddr: %v", err)
		return
	}
	err = writeData(rw, mas)
	if err != nil {
		logger.Errorf("sendOverlayPeerAddr: %v", err)
		return
	}
}

func textMarshalMultiaddrSlice(mas []maddr.Multiaddr) (string, error) {
	var result string
	for _, ma := range mas {
		text, err := ma.MarshalText()
		if err != nil {
			return "", err
		}
		result += string(text) + ","
	}
	strings.TrimSuffix(result, ",")
	return result, nil
}

func textUnmarshalMultiaddrSlice(text string) ([]maddr.Multiaddr, error) {
	mas := make([]maddr.Multiaddr, 1)
	mastrs := strings.Split(text, ",")
	for _, mastr := range mastrs {
		ma, err := maddr.NewMultiaddr("")
		if err != nil {
			logger.Errorf("NewMultiaddr: %v", err)
			return nil, err
		}
		logger.Debug(mastr)
		err = ma.UnmarshalText([]byte(strings.TrimSpace(mastr)))
		if err != nil {
			logger.Errorf("ma.UnmarshalText: %v", err)
			return nil, err
		}
		mas = append(mas, ma)
	}
	return mas, nil
}

func writeData(rw *bufio.ReadWriter, data string) error {
	logger.Debug("writedata: ", data)
	_, err := rw.WriteString(fmt.Sprintf("%s\n", data))
	if err != nil {
		return fmt.Errorf("error writing to buffer: %v", err)
	}
	err = rw.Flush()
	if err != nil {
		return fmt.Errorf("error flushing buffer: %v", err)
	}
	return nil
}
