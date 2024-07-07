package supabase_realtime_go

const (
	CHANNEL_EVENT_CLOSE        = "phx_close"
	CHANNEL_EVENT_ERROR        = "phx_error"
	CHANNEL_EVENT_JOIN         = "phx_join"
	CHANNEL_EVENT_REPLY        = "phx_reply"
	CHANNEL_EVENT_LEAVE        = "phx_leave"
	CHANNEL_EVENT_ACCESS_TOKEN = "access_token"

	DEFAULT_TIMEOUT = 10000

	WS_CLOSE_NORMAL = 1000
)

type ChannelState string

const (
	CHANNEL_STATE_CLOSED  ChannelState = "closed"
	CHANNEL_STATE_ERRORED ChannelState = "errored"
	CHANNEL_STATE_JOINED  ChannelState = "joined"
	CHANNEL_STATE_JOINING ChannelState = "joining"
	CHANNEL_STATE_LEAVING ChannelState = "leaving"
)

type ConnectionState string

const (
	CONNECTION_STATE_CONNECTING ConnectionState = "connecting"
	CONNECTION_STATE_OPEN       ConnectionState = "open"
	CONNECTION_STATE_CLOSING    ConnectionState = "closing"
	CONNECTION_STATE_CLOSED     ConnectionState = "closed"
)

type RealtimeListenType string

const (
	REALTIME_LISTEN_TYPE_BROADCAST        RealtimeListenType = "broadcast"
	REALTIME_LISTEN_TYPE_PRESENCE         RealtimeListenType = "presence"
	REALTIME_LISTEN_TYPE_POSTGRES_CHANGES RealtimeListenType = "postgres_changes"
)

type PostgresChangeEvent string

const (
	POSTGRES_CHANGE_EVENT_ALL    PostgresChangeEvent = "*"
	POSTGRES_CHANGE_EVENT_INSERT PostgresChangeEvent = "INSERT"
	POSTGRES_CHANGE_EVENT_UPDATE PostgresChangeEvent = "UPDATE"
	POSTGRES_CHANGE_EVENT_DELETE PostgresChangeEvent = "DELETE"
)

const (
	VSN                        = "1.0.0"
	DEFAULT_HEADERS            = "X-Client-Info"
	POSTGRES_TYPES_JSON        = "json"
	POSTGRES_TYPES_JSONB       = "jsonb"
	POSTGRES_TYPES_TIMESTAMP   = "timestamp"
	POSTGRES_TYPES_TIMESTAMPTZ = "timestamptz"
)

type BroadcastOptions struct {
	Self bool
	Ack  bool
}

type PresenceOptions struct {
	Key string
}

type ConfigOptions struct {
	Broadcast BroadcastOptions
	Presence  PresenceOptions
	Private   bool
}

type RealtimeChannelOptions struct {
	Config ConfigOptions
}

type WebSocketLike interface {
	Connect() error
	Close(code int, reason string) error
	Send(data []byte) error
	IsConnected() bool
	SetHandlers(onOpen func(), onClose func(), onError func(error), onMessage func([]byte))
}
