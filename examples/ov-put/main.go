package main

import (
	"context"
	"strings"

	ipfslite "github.com/hsanjuan/ipfs-lite"
	leveldb "github.com/ipfs/go-ds-leveldb"
	log "github.com/ipfs/go-log"
	overlay "github.com/lanzafame/go-libp2p-dht-overlay"
)

func init() {
	log.SetLogLevel("ov-put", "DEBUG")
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

	logger.Debug("adding file to overlay dht")
	n, err := lite.AddFile(ctx, strings.NewReader("hello"), nil)
	if err != nil {
		panic(err)
	}

	logger.Debug("printing cid")
	logger.Debug(n.Cid())
	logger.Debug("blocking...")

	select {}
}
