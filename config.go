package overlay

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	crypto "github.com/libp2p/go-libp2p-crypto"
	maddr "github.com/multiformats/go-multiaddr"
)

// A new type we need for writing a custom flag parser
type addrList []maddr.Multiaddr

func (al *addrList) String() string {
	strs := make([]string, len(*al))
	for i, addr := range *al {
		strs[i] = addr.String()
	}
	return strings.Join(strs, ",")
}

func (al *addrList) Set(value string) error {
	addr, err := maddr.NewMultiaddr(value)
	if err != nil {
		return err
	}
	*al = append(*al, addr)
	return nil
}

// StringsToAddrs ...
func StringsToAddrs(addrStrings []string) (maddrs []maddr.Multiaddr, err error) {
	for _, addrString := range addrStrings {
		addr, err := maddr.NewMultiaddr(addrString)
		if err != nil {
			return maddrs, err
		}
		maddrs = append(maddrs, addr)
	}
	return
}

// Config provides the configuration required to start the overlay nodes
type Config struct {
	RendezvousString       string
	BootstrapPeers         addrList
	ListenAddresses        addrList
	OverlayListenAddresses addrList
	ProtocolID             string
}

// Identity is the private keys of the two peers.
type Identity struct {
	PublicPrivKey  crypto.PrivKey
	OverlayPrivKey crypto.PrivKey
}

type jsonIdentity struct {
	PublicPrivKey, OverlayPrivKey []byte
}

// Save writes config to the provided path.
func (c *Identity) Save(path string) error {
	if c == nil {
		return fmt.Errorf("Identity is nil")
	}
	if c.PublicPrivKey == nil {
		return fmt.Errorf("PublicPrivKey is nil")
	}
	byPubPrivKey, err := crypto.MarshalPrivateKey(c.PublicPrivKey)
	if err != nil {
		return err
	}
	byOverlayPrivKey, err := crypto.MarshalPrivateKey(c.OverlayPrivKey)
	if err != nil {
		return err
	}
	jsonID := &jsonIdentity{PublicPrivKey: byPubPrivKey, OverlayPrivKey: byOverlayPrivKey}
	file, err := json.MarshalIndent(jsonID, "", " ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(path, file, 0644)
}

// LoadIdentity loads a json config from a path.
func LoadIdentity(path string) (*Identity, error) {
	file, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	jsonID := &jsonIdentity{}
	err = json.Unmarshal(file, jsonID)
	if err != nil {
		return nil, err
	}

	c := &Identity{}
	c.PublicPrivKey, err = crypto.UnmarshalPrivateKey(jsonID.PublicPrivKey)
	if err != nil {
		return nil, err
	}
	c.OverlayPrivKey, err = crypto.UnmarshalPrivateKey(jsonID.OverlayPrivKey)
	if err != nil {
		return nil, err
	}

	return c, nil
}
