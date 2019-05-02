package main

import (
	"context"
	"os"

	ipfslite "github.com/hsanjuan/ipfs-lite"
	cid "github.com/ipfs/go-cid"
	leveldb "github.com/ipfs/go-ds-leveldb"
	log "github.com/ipfs/go-log"
	overlay "github.com/lanzafame/go-libp2p-dht-overlay"
)

func init() {
	log.SetLogLevel("ov-get", "DEBUG")
}

func main() {
	config, err := ParseFlags()
	if err != nil {
		panic(err)
	}

	logger.Debug(">>>>>> Listen Address: ", config.ListenAddresses)
	logger.Debug(">>>>>> Overlay Listen Address: ", config.OverlayListenAddresses)

	ctx := context.Background()

	logger.Debug("starting coordinator")
	coord, err := overlay.NewCoordinator(ctx, config)
	if err != nil {
		panic(err)
	}

	logger.Debug("bootstrapping")
	err = coord.Bootstrap(ctx)
	if err != nil {
		panic(err)
	}

	ds, err := leveldb.NewDatastore("", nil)
	if err != nil {
		panic(err)
	}

	logger.Debug("starting ipfs lite")
	lite, err := ipfslite.New(ctx, ds, coord.OverlayPeer, coord.OverlayDHT, &ipfslite.Config{false})
	if err != nil {
		panic(err)
	}

	logger.Debug("decoding cid")
	c, err := cid.Decode("zdj7Wdkjub2uSDUbfmcLN6ngEFWuKhL5NQopLh7BhGUHzDWkT")
	if err != nil {
		panic(err)
	}

	logger.Debug("requesting cid from overlay dht")
	w, err := lite.GetFile(ctx, c)
	if err != nil {
		panic(err)
	}

	logger.Debug("writing contents out")
	_, err = w.WriteTo(os.Stdout)
	if err != nil {
		panic(err)
	}
}
