package client

import (
	"github.com/ao-data/albiondata-client/log"
)

type operationGetGameServerByCluster struct {
	ZoneID string `mapstructure:"0"`
}

func (op operationGetGameServerByCluster) Process(state *albionState) {
	location := normalizeLocationID(op.ZoneID)
	if location == "" {
		log.Debugf("Ignoring implausible GetGameServerByCluster zone value: %q", op.ZoneID)
		return
	}
	if state.LocationId != location {
		log.Infof("(operationGetGameServerByCluster) Updating player location to %v.", location)
		state.LocationId = location
	}

	// A zone change resolves the location; flush any market browse that was
	// buffered while it was unknown. No-op when nothing is buffered.
	flushPendingMarket(state)
}
