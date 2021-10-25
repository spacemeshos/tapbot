package main

import (
	"bot/bot"
	"flag"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/spacemeshos/ed25519"
	"github.com/spacemeshos/go-spacemesh/common/types"
	"github.com/spacemeshos/smrepl/client"
	"github.com/tyler-smith/go-bip39"
	"log"
	"os"
	"os/signal"
	"syscall"
)

const spaceSalt = "Spacemesh blockmesh"

func main() {
	// todo: read args - api address, account privatekey

	cfg, err := bot.LoadConfigFromFile()
	if err != nil {
		fmt.Println("error loading config from file ", err)
		if cfg == nil {
			cfg = bot.DefaultConfig()
		}
		flag.StringVar(&cfg.Server, "server", "", fmt.Sprintf("The Spacemesh api grpc server host and port. Defaults to %s", cfg.Server))
		flag.StringVar(&cfg.PublicKey, "public-key", "", "address of tap bot")
		flag.StringVar(&cfg.PrivateKey, "private-key", "", "private key of tap (not needed if mnemonic provided)")
		flag.StringVar(&cfg.Mnemonic, "mnemonic", "", "Mnemonic to recover keys from")
		flag.Uint64Var(&cfg.TransferAmount, "amount", 10, "set the name of wallet file to open")
		flag.StringVar(&cfg.Mnemonic, "wallet-directory", "", "set default wallet files directory")
		flag.StringVar(&cfg.BotToken, "bot", "", "token for discord bot")
	}



	//apiAddr := "127.0.0.1:9092"
	//accountpk := ed25519.NewKeyFromSeed([]byte("somerandombytes"))

	addr := types.Address{}
	pk := ed25519.PrivateKey{}
	if cfg.Mnemonic != "" {
		seed := bip39.NewSeed(cfg.Mnemonic, "")
		pk = ed25519.NewDerivedKeyFromSeed(seed[:32], 0, []byte(spaceSalt))
		pub := pk.Public().(ed25519.PublicKey)[:]
		addr = types.BytesToAddress(pub)
	} else {
		pk, err = bot.NewPrivateKeyFromBuffer(bot.FromHex(cfg.PrivateKey))
		if err != nil {
			fmt.Println("no address provided")
			return
		}
		pub := pk.Public().(ed25519.PublicKey)[:]
		addr, err = types.StringToAddress(cfg.PublicKey)
		if err != nil {
			fmt.Println("no public key found")
			addr = types.BytesToAddress(pub)
		}
	}

	if cfg.BotToken == "" {
		fmt.Println("No token provided. Please run: airhorn -t <bot token>")
		return
	}

	dg, err := discordgo.New("Bot " + cfg.BotToken)
	if err != nil {
		fmt.Println("Error creating Discord session: ", err)
		return
	}

	be, err := client.OpenConnection(cfg.Server, cfg.SecureConnection,"")
	if err != nil {
		fmt.Println("Error creating wallet backend: ", err)
		return
	}

	bb := bot.NewBot(be, addr, pk, *cfg)

	// Register ready as a callback for the ready events.
	dg.AddHandler(ready)

	// Register messageCreate as a callback for the messageCreate events.
	dg.AddHandler(bb.OnMessage)

	// Open the websocket and begin listening.
	err = dg.Open()
	if err != nil {
		fmt.Println("Error opening Discord session: ", err)
	}

	stop := make(chan struct{})
	exit := make(chan int)
	go handleSignals(exit, stop)
	<-exit


}

func ready(s *discordgo.Session, event *discordgo.Ready) {
	// Set the playing status.
	s.ChannelMessageSend("tap", "faucet bot ready")
}

func  handleSignals(exitCh chan int, stop chan struct{}) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(
		sigCh,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	)

	for {
		select {
		case <-stop:
			log.Println("[INFO] stop handleSignals")
			return
		case s := <-sigCh:
			switch s {
			case syscall.SIGINT: // kill -SIGINT XXXX or Ctrl+c
				log.Println("[SIGNAL] Catch SIGINT")
				exitCh <- 0

			case syscall.SIGTERM: // kill -SIGTERM XXXX
				log.Println("[SIGNAL] Catch SIGTERM")
				exitCh <- 1

			case syscall.SIGQUIT: // kill -SIGQUIT XXXX
				log.Println("[SIGNAL] Catch SIGQUIT")
				exitCh <- 0
			}
		}
	}
}
