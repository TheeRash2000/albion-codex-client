// +build windows

package notification

import (
	"github.com/ao-data/albiondata-client/log"
	toast "gopkg.in/toast.v1"
)

func Push(msg string) {
	// Title is what the player reads. AppID stays as it was: Windows ties toast
	// delivery and the player's notification settings to it.
	note := toast.Notification{AppID: "Albion Data Client", Title: "Codex Client", Message: msg}

	err := note.Push()
	if err != nil {
		log.Error(err)
	}
}
