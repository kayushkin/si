package discord

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	si "github.com/kayushkin/si"
)

// Adapter connects directly to Discord via bot token.
type Adapter struct {
	token     string
	channelID string // default channel to send to
	botID     string
	session   *discordgo.Session
	incoming  chan si.Message
}

// New creates a Discord adapter.
func New(token, channelID string) *Adapter {
	return &Adapter{
		token:     token,
		channelID: channelID,
		incoming:  make(chan si.Message, 64),
	}
}

func (a *Adapter) Name() string { return "discord" }

func (a *Adapter) Start(ctx context.Context) error {
	s, err := discordgo.New("Bot " + a.token)
	if err != nil {
		return err
	}
	a.session = s

	s.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages | discordgo.IntentsMessageContent

	s.AddHandler(func(_ *discordgo.Session, m *discordgo.MessageCreate) {
		// Skip own messages
		if m.Author.ID == a.botID {
			return
		}

		a.incoming <- si.Message{
			ID:        m.ID,
			Text:      m.Content,
			Author:    m.Author.Username,
			Channel:   "discord:" + m.ChannelID,
			Timestamp: time.Now(),
		}
	})

	if err := s.Open(); err != nil {
		return err
	}

	a.botID = s.State.User.ID
	log.Printf("[discord] connected as %s (%s)", s.State.User.Username, a.botID)

	<-ctx.Done()
	s.Close()
	close(a.incoming)
	return nil
}

func (a *Adapter) Send(msg si.Message) error {
	ch := a.channelID // default
	if strings.HasPrefix(msg.Channel, "discord:") {
		ch = strings.TrimPrefix(msg.Channel, "discord:")
	}

	_, err := a.session.ChannelMessageSend(ch, msg.Text)
	if err != nil {
		log.Printf("[discord] send error: %v", err)
	}
	return err
}

func (a *Adapter) Receive() <-chan si.Message {
	return a.incoming
}

// compile-time check
var _ si.Adapter = (*Adapter)(nil)
