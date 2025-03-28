package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/fiatjaf/eventstore"
	"github.com/nbd-wtf/go-nostr"
)

const layout = "2006-01-02"

var (
	ownerImportedNotes  = 0
	taggedImportedNotes = 0
)

func importOwnerNotes() {
	ctx := context.Background()
	wdb := eventstore.RelayWrapper{Store: outboxDB}

	startTime, err := time.Parse(layout, config.ImportStartDate)
	if err != nil {
		fmt.Println("Error parsing start date:", err)
		return
	}
	endTime := startTime.Add(240 * time.Hour)

	for {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		startTimestamp := nostr.Timestamp(startTime.Unix())
		endTimestamp := nostr.Timestamp(endTime.Unix())

		filters := []nostr.Filter{{
			Authors: nPubsToPubkeys(config.OwnerNpub),
			Since:   &startTimestamp,
			Until:   &endTimestamp,
		}}

		for ev := range pool.SubManyEose(ctx, config.ImportSeedRelays, filters) {
			wdb.Publish(ctx, *ev.Event)
			ownerImportedNotes++
		}
		log.Println("📦 imported", ownerImportedNotes, "owner notes")
		time.Sleep(5 * time.Second)

		startTime = startTime.Add(240 * time.Hour)
		endTime = endTime.Add(240 * time.Hour)

		if startTime.After(time.Now()) {
			log.Println("✅ owner note import complete! ")
			break
		}
	}
}

func importTaggedNotes() {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	wdb := eventstore.RelayWrapper{Store: inboxDB}
	filters := []nostr.Filter{{
		Tags: nostr.TagMap{
			"p": nPubsToPubkeys(config.OwnerNpub),
		},
	}}

	log.Println("📦 importing inbox notes, please wait 2 minutes")
	taggedImportedNotes = 0
	for ev := range pool.SubMany(ctx, config.ImportSeedRelays, filters) {
		if !wotMap[ev.Event.PubKey] {
			continue
		}
		for _, tag := range ev.Event.Tags.GetAll([]string{"p"}) {
			if len(tag) < 2 {
				continue
			}
			if contains(nPubsToPubkeys(config.OwnerNpub), tag[1]) {
				wdb.Publish(ctx, *ev.Event)
				taggedImportedNotes++
			}
		}
	}
	log.Println("📦 imported", taggedImportedNotes, "tagged notes")
	log.Println("✅ tagged import complete. please restart the relay")
}

func subscribeInbox() {
	ctx := context.Background()
	wdb := eventstore.RelayWrapper{Store: inboxDB}
	startTime := nostr.Timestamp(time.Now().Add(-time.Minute * 5).Unix())
	filters := []nostr.Filter{{
		Tags: nostr.TagMap{
			"p": nPubsToPubkeys(config.OwnerNpub),
		},
		Since: &startTime,
	}}

	log.Println("📢 subscribing to inbox")
	for ev := range pool.SubMany(ctx, config.ImportSeedRelays, filters) {
		if !wotMap[ev.Event.PubKey] {
			continue
		}
		for _, tag := range ev.Event.Tags.GetAll([]string{"p"}) {
			if len(tag) < 2 {
				continue
			}
			if contains(nPubsToPubkeys(config.OwnerNpub), tag[1]) {
				wdb.Publish(ctx, *ev.Event)
				switch ev.Event.Kind {
				case nostr.KindTextNote:
					log.Println("📰 new note in your inbox")
				case nostr.KindReaction:
					log.Println(ev.Event.Content, "new reaction in your inbox")
				case nostr.KindZap:
					log.Println("⚡️ new zap in your inbox")
				case nostr.KindEncryptedDirectMessage:
					log.Println("🔒 new encrypted message in your inbox")
				case nostr.KindRepost:
					log.Println("🔁 new repost in your inbox")
				case nostr.KindFollowList:
					// do nothing
				default:
					log.Println("📦 new event in your inbox")
				}
				taggedImportedNotes++
			}
		}
	}
}
