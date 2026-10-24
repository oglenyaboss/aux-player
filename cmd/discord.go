package cmd

import (
	"github.com/oglenyaboss/aux-player/internal/config"
	"github.com/oglenyaboss/aux-player/internal/discord"
	playerpkg "github.com/oglenyaboss/aux-player/internal/player"
)

func startDiscordRPC(cfg *config.Config, player interface {
	Subscribe() <-chan playerpkg.State
}, log func(string)) func() {
	if noDiscord || !cfg.DiscordRPCEnabled() {
		return func() {}
	}
	clientID := cfg.DiscordClientID
	if clientID == "" {
		clientID = discord.DefaultClientID
	}
	svc := discord.Start(clientID, player, log)
	return func() {
		_ = svc.Close()
	}
}
