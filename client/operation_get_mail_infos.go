package client

import (
	"fmt"
	"time"

	"github.com/ao-data/albiondata-client/log"
)

var MailInfos MailInfosLookup

type MailInfosLookup []MailInfo

func (mi MailInfosLookup) getMailInfo(id int) *MailInfo {
	for i := range mi {
		if mi[i].ID == id {
			return &mi[i]
		}
	}
	return nil
}

type MailInfo struct {
	ID         int    `json:"MailId"`     // mapstructure:"3"
	LocationID string `json:"LocationId"` // mapstructure:"7"
	OrderType  string `json:"OrderType"`  // mapstructure:"11"
	Expires    int64  `json:"Expires"`    // mapstructure:"12"
}

func (m *MailInfo) StringArray() []string {
	return []string{
		fmt.Sprintf("%d", m.ID),
		m.LocationID,
		m.OrderType,
		m.StringExpires(),
	}
}

func (m *MailInfo) StringExpires() string {
	return time.Unix(m.Expires, 0).Format(time.RFC3339)
}

// operationGetMailInfosResponse is the mailbox list: one column per field, one
// entry per mail.
//
// The game moved the city, mail type and time columns along by one (6, 10, 11 ->
// 7, 11, 12; seen 2026-11-10). Reading the old positions found flag columns, the
// whole response failed to decode, and no mail ever had a type - so a finished
// buy order could not be told from a finished sell order. If mails stop being
// classified again, check these positions first (run with -debug and read the
// opGetMailInfos response).
//
// Column 12 looks like the time the mail was received, in .NET ticks, not an
// expiry in Unix seconds. It is carried as before and only reaches the legacy
// market-notification upload.
type operationGetMailInfosResponse struct {
	MailIDs    []int    `mapstructure:"3"`
	Locations  []string `mapstructure:"7"`
	OrderTypes []string `mapstructure:"11"`
	Expires    []int64  `mapstructure:"12"`
}

func (op operationGetMailInfosResponse) Process(state *albionState) {
	log.Debugf("Got response to GetMailInfos operation")

	for i, id := range op.MailIDs {
		mail := MailInfo{ID: id}
		// The columns should all be the same length; never index past a short one.
		if i < len(op.Locations) {
			mail.LocationID = op.Locations[i]
		}
		if i < len(op.OrderTypes) {
			mail.OrderType = op.OrderTypes[i]
		}
		if i < len(op.Expires) {
			mail.Expires = op.Expires[i]
		}
		// The list is re-sent every time the mailbox opens: update, don't pile up.
		if known := MailInfos.getMailInfo(id); known != nil {
			*known = mail
		} else {
			MailInfos = append(MailInfos, mail)
		}
	}

	if len(MailInfos) < 1 {
		log.Info("Mail Infos Response - no mails\n\n")
		return
	}

	log.Infof("Mail Infos - Cached %d mail infos", len(MailInfos))
}
