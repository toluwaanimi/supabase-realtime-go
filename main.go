package supabase_realtime_go

import (
	"fmt"
	"github.com/toluwaanimi/supabase_realtime_go"
	"log"
	"time"
)

func main() {
	// Initialize the client
	client := supabase_realtime_go.NewClient("ws://localhost:4000/socket",
		func(c *supabase_realtime_go.Client) {
			c.Logger = func(kind, msg string, data interface{}) {
				log.Printf("[%s] %s: %v\n", kind, msg, data)
			}
			c.Timeout = 20000 // 20 seconds timeout
		})

	// Connect to the WebSocket
	client.Connect()

	// Create a channel
	channel := client.Channel("realtime:example", supabase_realtime_go.RealtimeChannelOptions{
		Config: supabase_realtime_go.ConfigOptions{
			Broadcast: supabase_realtime_go.BroadcastOptions{
				Ack:  true,
				Self: true,
			},
			Presence: supabase_realtime_go.PresenceOptions{
				Key: "user123",
			},
			Private: false,
		},
	})

	// Subscribe to the channel
	err := channel.Subscribe(func(status string, err error) {
		if err != nil {
			log.Fatalf("Subscription failed: %v\n", err)
		}
		fmt.Printf("Subscription status: %s\n", status)
	}, time.Duration(client.Timeout)*time.Millisecond)
	if err != nil {
		log.Fatalf("Failed to subscribe: %v\n", err)
	}

	// Handle presence events
	channel.On(supabase_realtime_go.REALTIME_LISTEN_TYPE_PRESENCE, map[string]interface{}{"event": "sync"}, func(payload interface{}) {
		fmt.Println("Presence sync event:", payload)
	})

	channel.On(supabase_realtime_go.REALTIME_LISTEN_TYPE_PRESENCE, map[string]interface{}{"event": "join"}, func(payload interface{}) {
		fmt.Println("Presence join event:", payload)
	})

	channel.On(supabase_realtime_go.REALTIME_LISTEN_TYPE_PRESENCE, map[string]interface{}{"event": "leave"}, func(payload interface{}) {
		fmt.Println("Presence leave event:", payload)
	})

	// Handle broadcast events
	channel.On(supabase_realtime_go.REALTIME_LISTEN_TYPE_BROADCAST, map[string]interface{}{"event": "new_message"}, func(payload interface{}) {
		fmt.Println("Broadcast event - new message:", payload)
	})

	// Send a broadcast message
	_, err = channel.Send(map[string]interface{}{
		"type":    supabase_realtime_go.REALTIME_LISTEN_TYPE_BROADCAST,
		"event":   "new_message",
		"payload": map[string]interface{}{"content": "Hello, world!"},
	}, nil)
	if err != nil {
		log.Fatalf("Failed to send message: %v\n", err)
	}

	// Simulate presence tracking
	err = channel.Presence.Track(map[string]interface{}{
		"user_id": "user123",
		"status":  "online",
	}, nil)
	if err != nil {
		log.Fatalf("Failed to track presence: %v\n", err)
	}

	// Wait to receive messages
	select {}
}
